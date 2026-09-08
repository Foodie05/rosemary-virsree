package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/service"
)

const adminCookie = "rvs_admin_session"

func (s *Server) sessionEmail(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(adminCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	email, err := s.svc.DB.AdminSession(r.Context(), secretbox.Hash(cookie.Value))
	return email, err == nil
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	email, authenticated := s.sessionEmail(r)
	if !authenticated && r.Header.Get("Authorization") == "Bearer "+s.svc.Config.AdminToken {
		authenticated = true
		email = "automation-token"
	}
	write(w, http.StatusOK, map[string]any{
		"authenticated":   authenticated,
		"email":           email,
		"oidc_configured": s.svc.Config.OIDCReady(),
		"setup_required":  authenticated && !s.svc.DB.StorageConfigured(r.Context()),
	})
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if !s.svc.Config.OIDCReady() {
		fail(w, http.StatusServiceUnavailable, "OIDC is not configured")
		return
	}
	state := secretbox.Random("", 32)
	nonce := secretbox.Random("", 32)
	verifier := secretbox.Random("", 48)
	challenge := sha256.Sum256([]byte(verifier))
	if err := s.svc.DB.SaveOIDCChallenge(r.Context(), secretbox.Hash(state), nonce, verifier, time.Now().Add(10*time.Minute).UTC()); err != nil {
		fail(w, http.StatusInternalServerError, "could not start login")
		return
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {s.svc.Config.OIDCClientID},
		"redirect_uri":          {s.svc.Config.OIDCRedirectURL},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, s.svc.Config.OIDCIssuer+"/oidc/authorize?"+q.Encode(), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	failRedirect := func(message string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(message), http.StatusFound)
	}
	if providerErr := r.URL.Query().Get("error"); providerErr != "" {
		failRedirect("登录被拒绝：" + providerErr)
		return
	}
	code, state := r.URL.Query().Get("code"), r.URL.Query().Get("state")
	if code == "" || state == "" {
		failRedirect("登录回调缺少 code 或 state")
		return
	}
	nonce, verifier, err := s.svc.DB.ConsumeOIDCChallenge(r.Context(), secretbox.Hash(state))
	if err != nil {
		failRedirect("登录请求已过期，请重新登录")
		return
	}
	payload := map[string]string{
		"grant_type": "authorization_code", "code": code,
		"client_id": s.svc.Config.OIDCClientID, "client_secret": s.svc.Config.OIDCClientSecret,
		"redirect_uri": s.svc.Config.OIDCRedirectURL, "code_verifier": verifier,
	}
	raw, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 15 * time.Second}
	tokenReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, s.svc.Config.OIDCIssuer+"/oidc/token", bytes.NewReader(raw))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		failRedirect("无法连接身份服务")
		return
	}
	defer tokenResp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if tokenResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(tokenResp.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" {
		failRedirect("身份服务未能交换登录凭据")
		return
	}
	if token.IDToken == "" {
		failRedirect("身份服务未返回 ID Token")
		return
	}
	idSubject, err := verifyIDToken(r.Context(), client, token.IDToken, s.svc.Config.OIDCIssuer, s.svc.Config.OIDCClientID, nonce)
	if err != nil {
		failRedirect("ID Token 校验失败")
		return
	}
	userinfoReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, s.svc.Config.OIDCIssuer+"/oidc/userinfo", nil)
	userinfoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	userinfoResp, err := client.Do(userinfoReq)
	if err != nil {
		failRedirect("无法读取登录用户资料")
		return
	}
	defer userinfoResp.Body.Close()
	var user struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if userinfoResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(userinfoResp.Body, 1<<20)).Decode(&user) != nil || user.Subject == "" || user.Subject != idSubject || user.Email == "" {
		failRedirect("身份服务未返回有效邮箱")
		return
	}
	if user.EmailVerified != nil && !*user.EmailVerified {
		failRedirect("邮箱尚未验证")
		return
	}
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if !containsFold(s.svc.Config.AdminEmails, email) {
		failRedirect("该邮箱不在管理白名单")
		return
	}
	rawSession := secretbox.Random("rvs_session_", 32)
	expires := time.Now().Add(time.Duration(s.svc.Config.SessionTTL) * time.Second).UTC()
	if err = s.svc.DB.CreateAdminSession(r.Context(), secretbox.Hash(rawSession), email, expires); err != nil {
		failRedirect("无法创建管理会话")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: rawSession, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.svc.Config.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(s.svc.Config.SessionTTL)})
	http.Redirect(w, r, "/", http.StatusFound)
}

func containsFold(items []string, wanted string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), wanted) {
			return true
		}
	}
	return false
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookie); err == nil {
		s.svc.DB.DeleteAdminSession(r.Context(), secretbox.Hash(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.svc.Config.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	sources, err := s.svc.Storage.List(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"configured": len(sources) > 0, "storage_source_count": len(sources)})
}

func (s *Server) storageSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.svc.Storage.List(r.Context())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	write(w, 200, sources)
}

func (s *Server) addStorageSource(w http.ResponseWriter, r *http.Request) {
	var in service.StorageSourceInput
	if err := decode(r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	source, err := s.svc.Storage.Add(r.Context(), in)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	s.svc.DB.Audit(r.Context(), "storage-source.created", source.ID, source.Kind)
	write(w, http.StatusCreated, map[string]any{"source": source, "verified": true})
}

func (s *Server) transferUpload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength < 0 {
		fail(w, http.StatusLengthRequired, "Content-Length is required")
		return
	}
	if err := s.svc.RelayUpload(r.Context(), r.PathValue("token"), r.Body, r.ContentLength); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transferDownload(w http.ResponseWriter, r *http.Request) {
	body, head, err := s.svc.RelayDownload(r.Context(), r.PathValue("token"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	defer body.Close()
	if head.ContentType != "" {
		w.Header().Set("Content-Type", head.ContentType)
	}
	if head.Size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprint(head.Size))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, body)
}
