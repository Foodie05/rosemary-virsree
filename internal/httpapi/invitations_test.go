package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/service"
	"rosemary-virsree/internal/store"
)

func TestInvitationAPIKeepsSecretsOutOfListsAndHasNoOtherPermissions(t *testing.T) {
	h := testServer(t)
	if w := request(t, h, "POST", "/api/v1/buckets", "admin", map[string]any{"name": "Bucket", "slug": "bucket-test", "visibility": "public", "quota_bytes": 100}); w.Code != 201 {
		t.Fatal(w.Code)
	}
	path := "/api/v1/admin/buckets/bucket-test/upload-invitations"
	if w := request(t, h, "GET", path, "", nil); w.Code != 401 {
		t.Fatal("unauthenticated list allowed")
	}
	w := request(t, h, "POST", path, "admin", map[string]any{"name": "Test invitation", "max_files": 3, "expires_in": 3600})
	if w.Code != 201 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("create: %d", w.Code)
	}
	data := decodeMap(t, w)
	token := data["token"].(string)
	id := data["invitation"].(map[string]any)["id"].(string)
	w = request(t, h, "GET", path, "admin", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatal("list exposed invitation secret")
	}
	w = request(t, h, "GET", "/api/v1/agent/uploads", token, nil)
	if w.Code != 200 {
		t.Fatalf("invitation inspection: %d", w.Code)
	}
	for _, p := range []string{"/api/v1/buckets", "/api/v1/access-keys", path} {
		if w = request(t, h, "GET", p, token, nil); w.Code != 401 {
			t.Fatalf("token has unrelated permission: %s %d", p, w.Code)
		}
	}
	if w = request(t, h, "POST", "/api/v1/agent/uploads", token, map[string]any{"key": "file"}); w.Code != 400 {
		t.Fatal("missing size accepted")
	}
	if w = request(t, h, "POST", "/api/v1/agent/uploads/commit", token, map[string]any{"upload_id": "foreign", "key": "file"}); w.Code != 403 || decodeMap(t, w)["code"] != "upload_invitation_ownership" {
		t.Fatal("foreign commit allowed")
	}
	if w = request(t, h, "DELETE", path+"/"+id, "admin", nil); w.Code != 204 {
		t.Fatal("revoke failed")
	}
	if w = request(t, h, "GET", "/api/v1/agent/uploads", token, nil); w.Code != 403 || decodeMap(t, w)["code"] != "upload_invitation_inactive" {
		t.Fatal("revoked token accepted")
	}
}
func TestDocumentContentNegotiationAndTraversal(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	web := filepath.Join(dir, "web")
	os.Mkdir(docs, 0700)
	os.Mkdir(web, 0700)
	os.WriteFile(filepath.Join(docs, "api.md"), []byte("# API"), 0600)
	os.WriteFile(filepath.Join(web, "index.html"), []byte("<!doctype html><title>Reader</title>"), 0600)
	db, err := store.Open(filepath.Join(dir, "docs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("test-key")
	cfg := config.Config{DocsDir: docs, WebDir: web}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := New(svc).Handler()
	for _, tt := range []struct{ path, accept, contains, typ string }{{"/docs/api.md", "text/html", "Reader", "text/html"}, {"/docs/api.md", "text/markdown", "# API", "text/markdown"}, {"/docs/api.md?raw=1", "text/html", "# API", "text/markdown"}, {"/docs/api.md?raw=1&download=1", "text/html", "# API", "text/markdown"}} {
		r := httptest.NewRequest("GET", tt.path, nil)
		r.Header.Set("Accept", tt.accept)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tt.contains) || !strings.Contains(w.Header().Get("Content-Type"), tt.typ) || w.Header().Get("Vary") != "Accept" || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("document negotiation %s: %d %v", tt.path, w.Code, w.Header())
		}
		if strings.Contains(tt.path, "download") && !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
			t.Fatal("missing download header")
		}
	}
	r := httptest.NewRequest("GET", "/docs/missing.md", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("missing doc not rejected")
	}
}
