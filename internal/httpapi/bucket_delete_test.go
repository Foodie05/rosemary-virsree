package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestBucketDeleteAPIRequiresAdminExactNameAndServerWait(t *testing.T) {
	h := testServer(t)
	created := request(t, h, "POST", "/api/v1/buckets", "admin", map[string]any{"name": "Display name", "slug": "delete-bucket", "visibility": "private", "quota_bytes": 100})
	if created.Code != 201 {
		t.Fatalf("create: %d", created.Code)
	}
	path := "/api/v1/admin/buckets/delete-bucket"
	if got := request(t, h, "POST", path+"/deletion-confirmation", "", map[string]any{}); got.Code != http.StatusUnauthorized {
		t.Fatalf("non-admin challenge: %d", got.Code)
	}
	challenge := request(t, h, "POST", path+"/deletion-confirmation", "admin", map[string]any{})
	data := decodeMap(t, challenge)
	token := data["confirmation_token"].(string)
	for _, name := range []string{"Display name", "DELETE-BUCKET", "delete-bucket "} {
		got := request(t, h, "DELETE", path, "admin", map[string]any{"confirm_name": name, "confirmation_token": token})
		if got.Code != 409 || decodeMap(t, got)["code"] != "bucket_name_mismatch" {
			t.Fatalf("accepted wrong name: %d %s", got.Code, got.Body.String())
		}
	}
	body := map[string]any{"confirm_name": "delete-bucket", "confirmation_token": token}
	if got := request(t, h, "DELETE", path, "admin", body); got.Code != 409 || decodeMap(t, got)["code"] != "bucket_delete_wait" {
		t.Fatalf("wait bypass: %d %s", got.Code, got.Body.String())
	}
	if got := request(t, h, "DELETE", path, "", body); got.Code != 401 {
		t.Fatalf("non-admin delete: %d", got.Code)
	}
	ready, err := time.Parse(time.RFC3339Nano, data["ready_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(ready) + 10*time.Millisecond)
	got := request(t, h, "DELETE", path, "admin", body)
	if got.Code != 202 || decodeMap(t, got)["status"] != "deleting" {
		t.Fatalf("delete not queued: %d %s", got.Code, got.Body.String())
	}
	if got = request(t, h, "DELETE", path, "admin", body); got.Code != 202 {
		t.Fatalf("retry not idempotent: %d", got.Code)
	}
	if got = request(t, h, "PUT", path, "admin", map[string]any{"name": "changed", "visibility": "private", "quota_bytes": 100}); decodeMap(t, got)["code"] != "bucket_deleting" {
		t.Fatalf("delete/edit race allowed: %d %s", got.Code, got.Body.String())
	}
}
