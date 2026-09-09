package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

func TestNormalizeEndpointAcceptsCommonHumanInput(t *testing.T) {
	tests := map[string]string{
		".s3.bitiful.net":         "https://s3.bitiful.net",
		"s3.bitiful.net/":         "https://s3.bitiful.net",
		"//s3.bitiful.net":        "https://s3.bitiful.net",
		"https//s3.bitiful.net":   "https://s3.bitiful.net",
		"https:/s3.bitiful.net/":  "https://s3.bitiful.net",
		"https://.s3.bitiful.net": "https://s3.bitiful.net",
		" http://localhost:9000 ": "http://localhost:9000",
	}
	for input, want := range tests {
		got, err := normalizeEndpoint(input, false)
		if err != nil || got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestNormalizeEndpointRejectsUnsafeOrAmbiguousInput(t *testing.T) {
	for _, input := range []string{"", "ftp://example.com", "https://user:pass@example.com", "https://example.com?secret=value"} {
		if _, err := normalizeEndpoint(input, false); err == nil {
			t.Errorf("normalizeEndpoint(%q) unexpectedly succeeded", input)
		}
	}
	if got, err := normalizeEndpoint("", true); err != nil || got != "" {
		t.Fatalf("optional empty endpoint = %q, %v", got, err)
	}
}

func TestNormalizeS3ServiceEndpointRemovesDuplicateBucketHost(t *testing.T) {
	if got := normalizeS3ServiceEndpoint("https://ustore.s3.bitiful.net", "ustore"); got != "https://s3.bitiful.net" {
		t.Fatalf("bucket-scoped endpoint = %q", got)
	}
	if got := normalizeS3ServiceEndpoint("https://cdn.example.com", "cdn"); got != "https://cdn.example.com" {
		t.Fatalf("custom endpoint was unexpectedly changed: %q", got)
	}
}

func TestStorageSourceUpdateValidatesBeforeReplacingAndKeepsCredentials(t *testing.T) {
	var mu sync.Mutex
	var reject atomic.Bool
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "virsree-user" || password != "stored-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if reject.Load() && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		case "COPY":
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			destination, _ := url.Parse(r.Header.Get("Destination"))
			objects[destination.Path] = append([]byte(nil), body...)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	db, err := store.Open(t.TempDir() + "/virsree.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, err := secretbox.New("test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	manager := NewStorageManager(db, box, "https://storage.example.com", nil)
	created, err := manager.Add(context.Background(), StorageSourceInput{
		Name: "Primary DAV", Kind: "webdav", Priority: 10, CapacityBytes: 20 << 30,
		Endpoint: server.URL + "/root", WebDAVUsername: "virsree-user", WebDAVPassword: "stored-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	original, err := manager.Detail(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !original.WebDAVUsernameConfigured || !original.WebDAVPasswordConfigured {
		t.Fatal("credential presence was not reported")
	}

	reject.Store(true)
	_, _, err = manager.Update(context.Background(), created.ID, StorageSourceInput{
		Name: "Broken edit", Kind: "webdav", Priority: 1, CapacityBytes: 30 << 30,
		Endpoint: server.URL + "/root",
	})
	if err == nil {
		t.Fatal("failed verification unexpectedly replaced the source")
	}
	afterFailure, err := manager.Detail(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Name != original.Name || afterFailure.Priority != original.Priority || afterFailure.CapacityBytes != original.CapacityBytes {
		t.Fatalf("failed edit changed source metadata: %#v", afterFailure.StorageSource)
	}

	reject.Store(false)
	updated, _, err := manager.Update(context.Background(), created.ID, StorageSourceInput{
		Name: "Verified edit", Kind: "webdav", Priority: 2, CapacityBytes: 30 << 30,
		Endpoint: server.URL + "/root",
	})
	if err != nil {
		t.Fatalf("blank credential fields did not preserve encrypted credentials: %v", err)
	}
	if updated.Name != "Verified edit" || updated.Priority != 2 || updated.CapacityBytes != 30<<30 {
		t.Fatalf("verified edit was not persisted: %#v", updated)
	}
}

func TestStorageSourceBucketChangeRequiresAcknowledgementBeforeProbe(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/virsree.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, err := secretbox.New("test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	configCipher, err := (&StorageManager{box: box}).sealConfig(storedSourceConfig{
		Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "old-private-bucket", AccessKey: "ak", SecretKey: "sk",
	})
	if err != nil {
		t.Fatal(err)
	}
	source := model.StorageSource{ID: "src_test", Name: "S3", Kind: "s3", Priority: 1, CapacityBytes: 10 << 30, Enabled: true, Direct: true, ConfigCipher: configCipher, CreatedAt: time.Now().UTC()}
	if err = db.CreateStorageSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	manager := NewStorageManager(db, box, "https://storage.example.com", nil)
	_, changed, err := manager.Update(context.Background(), source.ID, StorageSourceInput{
		Name: source.Name, Kind: source.Kind, Priority: source.Priority, CapacityBytes: source.CapacityBytes,
		Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "new-private-bucket",
	})
	if err == nil || !changed {
		t.Fatalf("bucket change was not stopped for acknowledgement: changed=%t err=%v", changed, err)
	}
	persisted, err := manager.Detail(context.Background(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Bucket != "old-private-bucket" {
		t.Fatalf("unacknowledged bucket change was persisted: %q", persisted.Bucket)
	}
}
