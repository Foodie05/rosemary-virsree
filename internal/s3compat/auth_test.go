package s3compat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/service"
	"rosemary-virsree/internal/store"
)

type rewriteIdentityEncodingTransport struct {
	next        http.RoundTripper
	replacement string
}

func (t rewriteIdentityEncodingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	if clone.Header.Get("Accept-Encoding") == "identity" {
		if t.replacement == "" {
			clone.Header.Del("Accept-Encoding")
		} else {
			clone.Header.Set("Accept-Encoding", t.replacement)
		}
	}
	return t.next.RoundTrip(clone)
}

func TestPermissionsAreIndependent(t *testing.T) {
	if !permissionOK("read,delete", "read") || !permissionOK("read,delete", "delete") {
		t.Fatal("explicit permissions were not granted")
	}
	if permissionOK("manage", "read") || permissionOK("read", "write") {
		t.Fatal("one permission incorrectly implied another")
	}
}

func TestGoSDKHeaderSignedListObjectsV2(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "sdk.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, err := secretbox.New("test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PublicURL: "https://storage.example", TotalQuota: 1 << 30, MaxObjectsPerBucket: 100, MaxPendingUploads: 10, Backend: config.Backend{Region: "us-east-1"}}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := svc.NewBucket(ctx, "SDK", "sdk-bucket", "private", 1<<20, false)
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := svc.NewKey(ctx, bucket, "sdk", "read")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /s3/{bucket}", New(svc))
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, tc := range []struct {
		name       string
		httpClient aws.HTTPClient
		wantOK     bool
	}{
		{name: "direct", httpClient: http.DefaultClient, wantOK: true},
		{name: "proxy drops signed identity encoding", httpClient: &http.Client{Transport: rewriteIdentityEncodingTransport{next: &http.Transport{DisableCompression: true}}}, wantOK: true},
		{name: "proxy changes signed identity encoding", httpClient: &http.Client{Transport: rewriteIdentityEncodingTransport{next: http.DefaultTransport, replacement: "gzip"}}, wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := awss3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(key.AK, secret, ""), HTTPClient: tc.httpClient}, func(o *awss3.Options) { o.BaseEndpoint = aws.String(server.URL + "/s3"); o.UsePathStyle = true })
			_, listErr := client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(bucket.Slug), Prefix: aws.String("integration-check/")})
			if tc.wantOK && listErr != nil {
				t.Fatalf("Go SDK ListObjectsV2: %v", listErr)
			}
			if !tc.wantOK && listErr == nil {
				t.Fatal("accepted a request after a signed header was changed")
			}
		})
	}
}
