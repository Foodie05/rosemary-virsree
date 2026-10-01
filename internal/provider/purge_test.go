package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rosemary-virsree/internal/config"
)

func TestS3PurgeVersionsAndScopedPagination(t *testing.T) {
	versions := []string{"v1", "v2"}
	marker := true
	objects := map[string]bool{"rosemary/bucket/g1/a": true, "rosemary/other/g1/keep": true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		prefix := q.Get("prefix")
		if r.Method == "GET" && prefix != "rosemary/bucket/" {
			t.Errorf("incorrect cleanup prefix %q", prefix)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == "GET" && q.Has("versions"):
			fmt.Fprint(w, `<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>false</IsTruncated>`)
			for _, v := range versions {
				fmt.Fprintf(w, `<Version><Key>rosemary/bucket/g1/a</Key><VersionId>%s</VersionId></Version>`, v)
			}
			if marker {
				fmt.Fprint(w, `<DeleteMarker><Key>rosemary/bucket/deleted</Key><VersionId>marker</VersionId></DeleteMarker>`)
			}
			fmt.Fprint(w, `</ListVersionsResult>`)
		case r.Method == "GET":
			fmt.Fprint(w, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>false</IsTruncated>`)
			for k := range objects {
				if strings.HasPrefix(k, prefix) {
					fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>5</Size></Contents>`, k)
				}
			}
			fmt.Fprint(w, `</ListBucketResult>`)
		case r.Method == "DELETE":
			key := strings.TrimPrefix(r.URL.Path, "/private/")
			if !strings.HasPrefix(key, "rosemary/bucket/") {
				t.Errorf("deleted outside prefix: %s", key)
			}
			if v := q.Get("versionId"); v != "" {
				if v == "marker" {
					marker = false
				} else {
					for i, x := range versions {
						if x == v {
							versions = append(versions[:i], versions[i+1:]...)
							break
						}
					}
				}
			} else {
				delete(objects, key)
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	p := New(config.Backend{Endpoint: server.URL, Region: "us-east-1", Bucket: "private", AccessKey: "test", SecretKey: "test", PathStyle: true})
	if err := p.PurgePrefix(context.Background(), "rosemary/bucket/"); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 || marker || len(objects) != 1 || !objects["rosemary/other/g1/keep"] {
		t.Fatalf("cleanup missed or affected data: versions=%v marker=%v objects=%v", versions, marker, objects)
	}
	for _, prefix := range []string{"", "rosemary/", "rosemary/bucket", "virsree-system/v1/", "rosemary/../"} {
		if err := p.PurgePrefix(context.Background(), prefix); err == nil {
			t.Fatalf("unsafe prefix allowed %q", prefix)
		}
	}
}

func TestPurgeDoesNotHideVersionPermissionsOrWebDAVPartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(207)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(403)
		fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>Denied</Message></Error>`)
	}))
	defer server.Close()
	p := New(config.Backend{Endpoint: server.URL, Region: "us-east-1", Bucket: "private", AccessKey: "test", SecretKey: "test", PathStyle: true})
	if err := p.PurgePrefix(context.Background(), "rosemary/bucket/"); err == nil {
		t.Fatal("version access denied was treated as success")
	}
	w := NewWebDAV(WebDAVConfig{Endpoint: server.URL})
	if err := w.PurgePrefix(context.Background(), "rosemary/bucket/"); err == nil {
		t.Fatal("WebDAV partial cleanup was treated as success")
	}
}
