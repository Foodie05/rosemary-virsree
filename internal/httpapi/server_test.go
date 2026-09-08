package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/service"
	"rosemary-virsree/internal/store"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := secretbox.New("test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminToken: "admin", MasterKey: "test-master-key", PublicURL: "https://gateway.test", WebDir: filepath.Join(t.TempDir(), "missing"), TotalQuota: 1000, Backend: config.Backend{Region: "us-east-1"}}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return New(svc).Handler()
}

func TestOIDCLoginCreatesAllowlistedSessionAndStartsOOBE(t *testing.T) {
	var nonce string
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oidc/token":
			var body map[string]string
			if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || body["code_verifier"] == "" {
				t.Error("token request did not use JSON with PKCE verifier")
			}
			claims, _ := json.Marshal(map[string]string{"nonce": nonce})
			idToken := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
			write(w, 200, map[string]string{"access_token": "access", "id_token": idToken})
		case "/oidc/userinfo":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("missing userinfo bearer token")
			}
			write(w, 200, map[string]any{"sub": "user-1", "email": "Admin@Example.com", "email_verified": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "oidc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("test-master-key")
	cfg := config.Config{AdminToken: "admin", MasterKey: "test-master-key", PublicURL: "https://storage.example", WebDir: filepath.Join(t.TempDir(), "missing"), TotalQuota: 1000, MaxObjectsPerBucket: 10, MaxPendingUploads: 10, OIDCIssuer: issuer.URL, OIDCClientID: "client", OIDCClientSecret: "secret", OIDCRedirectURL: "https://storage.example/auth/callback", AdminEmails: []string{"admin@example.com"}, SessionTTL: 3600, Backend: config.Backend{Region: "us-east-1"}}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := New(svc).Handler()

	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginW := httptest.NewRecorder()
	h.ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusFound {
		t.Fatalf("login status = %d", loginW.Code)
	}
	location, err := url.Parse(loginW.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	nonce = location.Query().Get("nonce")
	state := location.Query().Get("state")
	if nonce == "" || state == "" || location.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("incomplete authorize URL: %s", location)
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback?code=ok&state="+url.QueryEscape(state), nil)
	callbackW := httptest.NewRecorder()
	h.ServeHTTP(callbackW, callbackReq)
	if callbackW.Code != http.StatusFound {
		t.Fatalf("callback status = %d: %s", callbackW.Code, callbackW.Body.String())
	}
	cookies := callbackW.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe session cookie: %#v", cookies)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionW := httptest.NewRecorder()
	h.ServeHTTP(sessionW, sessionReq)
	got := decodeMap(t, sessionW)
	if got["authenticated"] != true || got["email"] != "admin@example.com" || got["setup_required"] != true {
		t.Fatalf("unexpected session: %#v", got)
	}
}

func request(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

func TestAdminAndOneTimeOnboarding(t *testing.T) {
	h := testServer(t)

	if got := request(t, h, http.MethodGet, "/health", "", nil).Code; got != http.StatusOK {
		t.Fatalf("health status = %d", got)
	}
	meta := request(t, h, http.MethodGet, "/api/v1/meta", "", nil)
	if meta.Code != http.StatusOK || decodeMap(t, meta)["gateway_url"] != "https://gateway.test" {
		t.Fatalf("public metadata: %d %s", meta.Code, meta.Body.String())
	}
	if got := request(t, h, http.MethodGet, "/api/v1/overview", "wrong", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got)
	}

	created := request(t, h, http.MethodPost, "/api/v1/buckets", "admin", map[string]any{"name": "Media", "slug": "media-assets", "visibility": "private", "quota_bytes": 400})
	if created.Code != http.StatusCreated {
		t.Fatalf("create bucket: %d %s", created.Code, created.Body.String())
	}
	if got := decodeMap(t, created)["secret_key"]; got == "" {
		t.Fatal("owner secret was not returned once")
	}

	boot := request(t, h, http.MethodPost, "/api/v1/bootstrap-tokens", "admin", map[string]any{"name": "worker", "max_quota_bytes": 300, "expires_in": 120})
	if boot.Code != http.StatusCreated {
		t.Fatalf("bootstrap: %d %s", boot.Code, boot.Body.String())
	}
	token, _ := decodeMap(t, boot)["token"].(string)
	claimBody := map[string]any{"token": token, "name": "Worker", "slug": "worker-data", "visibility": "private", "quota_bytes": 300}
	claim := request(t, h, http.MethodPost, "/api/v1/agent/claim", "", claimBody)
	if claim.Code != http.StatusCreated {
		t.Fatalf("claim: %d %s", claim.Code, claim.Body.String())
	}
	result := decodeMap(t, claim)
	if result["endpoint"] != "https://gateway.test/s3" || result["access_key"] == "" || result["secret_key"] == "" {
		t.Fatalf("unexpected claim result: %#v", result)
	}
	if got := request(t, h, http.MethodPost, "/api/v1/agent/claim", "", claimBody).Code; got != http.StatusBadRequest {
		t.Fatalf("reused token status = %d", got)
	}

	over := request(t, h, http.MethodGet, "/api/v1/overview", "admin", nil)
	values := decodeMap(t, over)
	if values["bucket_count"] != float64(2) || values["allocated_quota"] != float64(700) {
		t.Fatalf("unexpected overview: %#v", values)
	}
}

func TestTotalQuotaIsEnforced(t *testing.T) {
	h := testServer(t)
	for _, tc := range []struct {
		slug  string
		quota int
		want  int
	}{{"first-bucket", 800, 201}, {"second-bucket", 300, 400}} {
		w := request(t, h, http.MethodPost, "/api/v1/buckets", "admin", map[string]any{"name": tc.slug, "slug": tc.slug, "visibility": "private", "quota_bytes": tc.quota})
		if w.Code != tc.want {
			t.Fatalf("%s status = %d, want %d: %s", tc.slug, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestPutObjectToGatewayIsDisabled(t *testing.T) {
	h := testServer(t)
	w := request(t, h, http.MethodPut, "/s3/media-assets/file.bin", "", []byte("must not reach gateway"))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PutObject status = %d, want 405: %s", w.Code, w.Body.String())
	}
}
