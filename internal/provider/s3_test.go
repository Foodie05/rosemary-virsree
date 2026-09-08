package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"rosemary-virsree/internal/config"
)

func TestPresignPutBindsDeclaredContentLength(t *testing.T) {
	p := New(config.Backend{Endpoint: "https://s3.example.test", PublicEndpoint: "https://s3.example.test", Region: "us-east-1", Bucket: "private-bucket", AccessKey: "synthetic", SecretKey: "synthetic-secret", PathStyle: true})
	raw, err := p.PresignPut(context.Background(), "rosemary-staging/test/file.bin", "application/octet-stream", 1234, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u.Query().Get("X-Amz-SignedHeaders"), "content-length") {
		t.Fatalf("content length is not signed: %q", u.Query().Get("X-Amz-SignedHeaders"))
	}
}

func TestFailedProbeCleansUpThroughUploadEndpoint(t *testing.T) {
	var uploaded, deleted atomic.Bool
	upload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			uploaded.Store(true)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer upload.Close()
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer internal.Close()

	p := New(config.Backend{Endpoint: internal.URL, PublicEndpoint: upload.URL, DownloadEndpoint: upload.URL, Region: "us-east-1", Bucket: "private-bucket", AccessKey: "synthetic", SecretKey: "synthetic-secret", PathStyle: true})
	if err := p.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "metadata probe") {
		t.Fatalf("expected metadata probe failure, got %v", err)
	}
	if !uploaded.Load() || !deleted.Load() {
		t.Fatalf("upload endpoint cleanup was not completed: uploaded=%v deleted=%v", uploaded.Load(), deleted.Load())
	}
}

func TestUploadAndDownloadUseSeparateEndpoints(t *testing.T) {
	p := New(config.Backend{Endpoint: "https://internal.example.test", PublicEndpoint: "https://upload.example.test", DownloadEndpoint: "https://cdn.example.test", Region: "us-east-1", Bucket: "private-bucket", AccessKey: "synthetic", SecretKey: "synthetic-secret", PathStyle: true})
	put, err := p.PresignPut(context.Background(), "object", "text/plain", 5, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	get, err := p.PresignGet(context.Background(), "object", time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	putURL, _ := url.Parse(put)
	getURL, _ := url.Parse(get)
	if putURL.Host != "upload.example.test" || getURL.Host != "cdn.example.test" {
		t.Fatalf("unexpected signer hosts: upload=%q download=%q", putURL.Host, getURL.Host)
	}
}
