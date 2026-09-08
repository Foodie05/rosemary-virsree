package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
		fail(w, r, http.StatusServiceUnavailable, "OIDC is not configured")
		return
	}
	state := secretbox.Random("", 32)
	nonce := secretbox.Random("", 32)
	verifier := secretbox.Random("", 48)
	challenge := sha256.Sum256([]byte(verifier))
	if err := s.svc.DB.SaveOIDCChallenge(r.Context(), secretbox.Hash(state), nonce, verifier, time.Now().Add(10*time.Minute).UTC()); err != nil {
		fail(w, r, http.StatusInternalServerError, "could not start login")
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
	failRedirect := func(zh, en string) {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(localText(r, zh, en)), http.StatusFound)
	}
	if providerErr := r.URL.Query().Get("error"); providerErr != "" {
		failRedirect("身份服务拒绝了登录请求，请重试。", "The identity provider denied the sign-in request. Try again.")
		return
	}
	code, state := r.URL.Query().Get("code"), r.URL.Query().Get("state")
	if code == "" || state == "" {
		failRedirect("登录回调缺少必要参数，请重新登录。", "The sign-in callback is missing required parameters. Sign in again.")
		return
	}
	nonce, verifier, err := s.svc.DB.ConsumeOIDCChallenge(r.Context(), secretbox.Hash(state))
	if err != nil {
		failRedirect("登录请求已过期，请重新登录。", "The sign-in request has expired. Sign in again.")
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
		failRedirect("暂时无法连接身份服务，请稍后重试。", "The identity provider is temporarily unreachable. Try again later.")
		return
	}
	defer tokenResp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if tokenResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(tokenResp.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" {
		failRedirect("身份服务未能完成凭据交换，请重新登录。", "The identity provider could not complete the credential exchange. Sign in again.")
		return
	}
	if token.IDToken == "" {
		failRedirect("身份服务没有返回 ID Token，请联系管理员。", "The identity provider did not return an ID Token. Contact the administrator.")
		return
	}
	idSubject, err := verifyIDToken(r.Context(), client, token.IDToken, s.svc.Config.OIDCIssuer, s.svc.Config.OIDCClientID, nonce)
	if err != nil {
		failRedirect("ID Token 校验失败，请重新登录。", "ID Token validation failed. Sign in again.")
		return
	}
	userinfoReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, s.svc.Config.OIDCIssuer+"/oidc/userinfo", nil)
	userinfoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	userinfoResp, err := client.Do(userinfoReq)
	if err != nil {
		failRedirect("无法读取登录用户资料，请稍后重试。", "The user profile could not be read. Try again later.")
		return
	}
	defer userinfoResp.Body.Close()
	var user struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if userinfoResp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(userinfoResp.Body, 1<<20)).Decode(&user) != nil || user.Subject == "" || user.Subject != idSubject || user.Email == "" {
		failRedirect("身份服务没有返回有效邮箱，请联系管理员。", "The identity provider did not return a valid email address. Contact the administrator.")
		return
	}
	if user.EmailVerified != nil && !*user.EmailVerified {
		failRedirect("该邮箱尚未完成验证。", "This email address has not been verified.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if !containsFold(s.svc.Config.AdminEmails, email) {
		failRedirect("该邮箱不在 VirSree 管理员白名单中。", "This email address is not on the VirSree administrator allowlist.")
		return
	}
	rawSession := secretbox.Random("rvs_session_", 32)
	expires := time.Now().Add(time.Duration(s.svc.Config.SessionTTL) * time.Second).UTC()
	if err = s.svc.DB.CreateAdminSession(r.Context(), secretbox.Hash(rawSession), email, expires); err != nil {
		failRedirect("暂时无法创建管理会话，请稍后重试。", "The administrator session could not be created. Try again later.")
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
		fail(w, r, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"configured": len(sources) > 0, "storage_source_count": len(sources)})
}

func (s *Server) storageSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.svc.Storage.List(r.Context())
	if err != nil {
		fail(w, r, 500, err.Error())
		return
	}
	write(w, 200, sources)
}

func (s *Server) addStorageSource(w http.ResponseWriter, r *http.Request) {
	var in service.StorageSourceInput
	if err := decode(r, &in); err != nil {
		fail(w, r, 400, err.Error())
		return
	}
	source, err := s.svc.Storage.Add(r.Context(), in)
	if err != nil {
		problem := errorFor(err.Error(), http.StatusBadRequest)
		kind := strings.ToLower(strings.TrimSpace(in.Kind))
		if kind != "s3" && kind != "webdav" {
			kind = "invalid"
		}
		if problem.Code == "storage_probe_not_found" && in.PathStyle {
			problem.ZH += " 本次请求开启了 Path-style；公有云 S3 兼容服务通常需要关闭后重试。"
			problem.EN += " Path-style was enabled for this attempt; public S3-compatible services commonly require it to be disabled."
		}
		slog.Warn("storage source rejected",
			"request_id", requestID(r),
			"error_code", problem.Code,
			"storage_kind", kind,
			"endpoint_id", endpointFingerprint(in.Endpoint, s.svc.Config.MasterKey),
			"public_endpoint_id", endpointFingerprint(in.PublicEndpoint, s.svc.Config.MasterKey),
			"cdn_endpoint_id", endpointFingerprint(in.CDNEndpoint, s.svc.Config.MasterKey),
			"path_style", in.PathStyle,
		)
		failProblem(w, r, 400, problem)
		return
	}
	s.svc.DB.Audit(r.Context(), "storage-source.created", source.ID, source.Kind)
	write(w, http.StatusCreated, map[string]any{"source": source, "verified": true})
}

func (s *Server) transferUpload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength < 0 {
		fail(w, r, http.StatusLengthRequired, "Content-Length is required")
		return
	}
	if err := s.svc.RelayUpload(r.Context(), r.PathValue("token"), r.Body, r.ContentLength); err != nil {
		fail(w, r, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transferDownload(w http.ResponseWriter, r *http.Request) {
	body, head, err := s.svc.RelayDownload(r.Context(), r.PathValue("token"))
	if err != nil {
		fail(w, r, http.StatusNotFound, err.Error())
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
