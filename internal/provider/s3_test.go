package provider

import (
	"context"
	"net/url"
	"strings"
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
