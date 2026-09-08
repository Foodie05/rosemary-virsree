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
