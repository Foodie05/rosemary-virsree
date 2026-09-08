package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	svc := service.New(db, provider.New(cfg.Backend), box, cfg)
	return New(svc).Handler()
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
