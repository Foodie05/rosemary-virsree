package httpapi

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rosemary-virsree/internal/buildinfo"
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
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			write(w, 200, map[string]string{"issuer": issuer.URL, "jwks_uri": issuer.URL + "/oidc/jwks"})
		case "/oidc/jwks":
			write(w, 200, map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()), "e": "AQAB"}}})
		case "/oidc/token":
			var body map[string]string
			if r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&body) != nil || body["code_verifier"] == "" {
				t.Error("token request did not use JSON with PKCE verifier")
			}
			idToken := signedTestIDToken(t, privateKey, issuer.URL, "client", "user-1", nonce)
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

func signedTestIDToken(t *testing.T, key *rsa.PrivateKey, issuer, audience, subject, nonce string) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{"iss": issuer, "aud": audience, "sub": subject, "nonce": nonce, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
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

func TestErrorsFollowRequestLanguageAndIncludeTraceID(t *testing.T) {
	h := testServer(t)
	for _, tc := range []struct {
		language string
		field    string
	}{
		{"zh-CN,zh;q=0.9,en;q=0.8", "message_zh"},
		{"en-US,en;q=0.9", "message_en"},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
		r.Header.Set("Accept-Language", tc.language)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		got := decodeMap(t, w)
		if w.Code != http.StatusUnauthorized || got["code"] != "admin_auth_required" {
			t.Fatalf("unexpected error response: %d %#v", w.Code, got)
		}
		if got["error"] != got[tc.field] || got["message_zh"] == got["message_en"] {
			t.Fatalf("language %q was not selected: %#v", tc.language, got)
		}
		if trace, _ := got["trace_id"].(string); !strings.HasPrefix(trace, "req_") || w.Header().Get("X-Request-ID") != trace {
			t.Fatalf("missing or mismatched trace ID: %#v, header %q", got, w.Header().Get("X-Request-ID"))
		}
	}
}

func TestStorageProviderDetailsAreNotReturned(t *testing.T) {
	raw := "storage verification failed: metadata probe: operation error S3: HeadObject, StatusCode: 404, RequestID: secret-request, HostID: secret-host, NotFound"
	problem := errorFor(raw, http.StatusBadRequest)
	if problem.Code != "storage_probe_not_found" {
		t.Fatalf("code = %q", problem.Code)
	}
	if strings.Contains(problem.ZH, "secret-") || strings.Contains(problem.EN, "secret-") || strings.Contains(problem.EN, "RequestID") {
		t.Fatalf("provider details leaked in localized error: %#v", problem)
	}
}

func TestStorageProbeErrorsPreferTheFailedDataPath(t *testing.T) {
	tests := []struct {
		raw, code string
	}{
		{"storage verification failed: direct/CDN download probe: Get https://redacted: context deadline exceeded", "storage_cdn_unreachable"},
		{"storage verification failed: direct/CDN download probe: 403 Forbidden", "storage_cdn_access_denied"},
		{"storage verification failed: direct upload probe: context deadline exceeded", "storage_upload_failed"},
	}
	for _, tc := range tests {
		if got := errorFor(tc.raw, http.StatusBadRequest); got.Code != tc.code {
			t.Errorf("errorFor(%q) code = %q; want %q", tc.raw, got.Code, tc.code)
		}
	}
}

func TestStorageProbeLogMetadataIsAllowlisted(t *testing.T) {
	if got := safeCDNMode(" bitiful_token "); got != "bitiful_token" {
		t.Fatalf("safe CDN mode = %q", got)
	}
	if got := safeCDNMode("secret-mode"); got != "invalid" {
		t.Fatalf("unrecognized CDN mode leaked: %q", got)
	}
	if got := storageProbeStage(errors.New("storage verification failed: direct/CDN download probe: 403 Forbidden")); got != "direct_cdn_download" {
		t.Fatalf("probe stage = %q", got)
	}
}

func TestEndpointFingerprintIsKeyedAndRedacted(t *testing.T) {
	a := endpointFingerprint(".s3.example.com", "first-key")
	b := endpointFingerprint("https://s3.example.com", "first-key")
	c := endpointFingerprint("s3.example.com", "second-key")
	if a != b || a == c || strings.Contains(a, "s3.example.com") || !strings.HasPrefix(a, "hmac-sha256:") {
		t.Fatalf("unexpected endpoint fingerprints: %q %q %q", a, b, c)
	}
}

func TestRequestLogRedactsPathAndQueryValues(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := testServer(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/buckets/secret-bucket/objects?prefix=secret-object", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	output := logs.String()
	if strings.Contains(output, "secret-bucket") || strings.Contains(output, "secret-object") {
		t.Fatalf("request values leaked into logs: %s", output)
	}
	if !strings.Contains(output, "GET /api/v1/admin/buckets/{bucket}/objects") || !strings.Contains(output, "request_id=req_") || !strings.Contains(output, "error_code=admin_auth_required") {
		t.Fatalf("missing structured request fields: %s", output)
	}
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
	version := request(t, h, http.MethodGet, "/api/v1/version", "", nil)
	if version.Code != http.StatusOK || decodeMap(t, version)["version"] != buildinfo.NormalizedVersion() || !strings.Contains(version.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("version metadata or cache policy: %d %s %#v", version.Code, version.Body.String(), version.Header())
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

func TestConsoleCachePolicyKeepsEntryFreshAndAssetsImmutable(t *testing.T) {
	webDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>VirSree</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "index-hash.js"), []byte("console.log('VirSree')"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("test-master-key")
	cfg := config.Config{AdminToken: "admin", MasterKey: "test-master-key", PublicURL: "https://gateway.test", WebDir: webDir, TotalQuota: 1000, Backend: config.Backend{Region: "us-east-1"}}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := New(svc).Handler()

	for _, path := range []string{"/", "/console/route?__virsree_version=0.2.6"} {
		w := request(t, h, http.MethodGet, path, "", nil)
		if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || w.Header().Get("Pragma") != "no-cache" {
			t.Fatalf("entry cache policy for %s: %d %#v", path, w.Code, w.Header())
		}
	}
	asset := request(t, h, http.MethodGet, "/assets/index-hash.js", "", nil)
	if asset.Code != http.StatusOK || asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache policy: %d %#v", asset.Code, asset.Header())
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
