package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

func TestBitifulDownloadUsesCDNAuthenticationAndRequestedTTL(t *testing.T) {
	p := New(config.Backend{
		Endpoint: "https://s3.example.test", PublicEndpoint: "https://s3.example.test",
		DownloadEndpoint: "https://cdn.example.test", DownloadMode: "bitiful_token", DownloadAuthKey: "synthetic-cdn-key",
		Region: "us-east-1", Bucket: "private-bucket", AccessKey: "synthetic", SecretKey: "synthetic-secret", PathStyle: true,
	})
	before := time.Now().Unix()
	raw, err := p.PresignGet(context.Background(), "folder/中文 file.txt", 30*24*time.Hour, "ignored.txt")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := strconv.ParseInt(u.Query().Get("_ts"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt < before+30*24*60*60 || expiresAt > time.Now().Unix()+30*24*60*60+1 {
		t.Fatalf("unexpected CDN expiry: %d", expiresAt)
	}
	sum := md5.Sum([]byte("synthetic-cdn-key" + u.EscapedPath() + strconv.FormatInt(expiresAt, 10)))
	if u.Host != "cdn.example.test" || u.EscapedPath() != "/folder/%E4%B8%AD%E6%96%87%20file.txt" || u.Query().Get("_btf_tk") != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected Bitiful URL components: host=%q path=%q query keys=%d", u.Host, u.EscapedPath(), len(u.Query()))
	}
	if len(u.Query()) != 2 || strings.Contains(raw, "synthetic-cdn-key") || strings.Contains(raw, "X-Amz-") {
		t.Fatalf("authentication key leaked or wrong signing protocol used")
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

func TestProbeReadsBackThroughBitifulCDN(t *testing.T) {
	const tokenKey = "synthetic-cdn-key"
	var mu sync.Mutex
	objects := map[string][]byte{}
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/private-bucket/")
		mu.Lock()
		defer mu.Unlock()
		if source := r.Header.Get("x-amz-copy-source"); source != "" {
			from, _ := url.PathUnescape(strings.TrimPrefix(source, "/"))
			from = strings.TrimPrefix(from, "private-bucket/")
			body, ok := objects[from]
			if !ok {
				http.NotFound(w, r)
				return
			}
			objects[key] = append([]byte(nil), body...)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<CopyObjectResult><ETag>&quot;probe&quot;</ETag><LastModified>2026-09-09T00:00:00Z</LastModified></CopyObjectResult>`))
			return
		}
		switch r.Method {
		case http.MethodPut:
			objects[key], _ = io.ReadAll(r.Body)
		case http.MethodHead:
			body, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("ETag", `"probe"`)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer storage.Close()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expiresAt, err := strconv.ParseInt(r.URL.Query().Get("_ts"), 10, 64)
		sum := md5.Sum([]byte(tokenKey + r.URL.EscapedPath() + strconv.FormatInt(expiresAt, 10)))
		if err != nil || expiresAt <= time.Now().Unix() || r.URL.Query().Get("_btf_tk") != hex.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		mu.Lock()
		body, ok := objects[strings.TrimPrefix(r.URL.Path, "/")]
		mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	defer cdn.Close()

	p := New(config.Backend{
		Endpoint: storage.URL, PublicEndpoint: storage.URL, DownloadEndpoint: cdn.URL,
		DownloadMode: "bitiful_token", DownloadAuthKey: tokenKey, Region: "us-east-1",
		Bucket: "private-bucket", AccessKey: "synthetic", SecretKey: "synthetic-secret", PathStyle: true,
	})
	if err := p.Probe(context.Background()); err != nil {
		t.Fatal(err)
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
