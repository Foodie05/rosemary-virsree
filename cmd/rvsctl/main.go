package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type claim struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Visibility string `json:"visibility"`
	Quota      int64  `json:"quota_bytes"`
}
type result struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:8080", "central Rosemary VirSree gateway")
	token := flag.String("token", "", "one-time bootstrap token")
	name := flag.String("name", "Application bucket", "display name")
	slug := flag.String("bucket", "", "virtual bucket name")
	quota := flag.Int64("quota", 1<<30, "quota in bytes")
	visibility := flag.String("visibility", "private", "private or public")
	out := flag.String("env-file", ".rosemary.env", "credential destination (0600)")
	flag.Parse()
	if *token == "" || *slug == "" {
		fmt.Fprintln(os.Stderr, "token and bucket are required")
		os.Exit(2)
	}
	if e := os.MkdirAll(filepath.Dir(*out), 0700); e != nil {
		fatal(e)
	}
	f, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		fatal(fmt.Errorf("credential destination must not already exist: %w", e))
	}
	body, _ := json.Marshal(claim{Token: *token, Name: *name, Slug: *slug, Visibility: *visibility, Quota: *quota})
	client := &http.Client{Timeout: 30 * time.Second}
	resp, e := client.Post(strings.TrimRight(*endpoint, "/")+"/api/v1/agent/claim", "application/json", bytes.NewReader(body))
	if e != nil {
		installFatal(f, *out, e)
	}
	defer resp.Body.Close()
	var r result
	if e = json.NewDecoder(resp.Body).Decode(&r); e != nil {
		installFatal(f, *out, e)
	}
	if resp.StatusCode >= 300 {
		installFatal(f, *out, fmt.Errorf("claim rejected (%s)", resp.Status))
	}
	content := fmt.Sprintf("AWS_ENDPOINT_URL=%s\nAWS_REGION=%s\nAWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nAWS_S3_FORCE_PATH_STYLE=true\nRVS_BUCKET=%s\n", r.Endpoint, r.Region, r.AccessKey, r.SecretKey, r.Bucket)
	if _, e = f.WriteString(content); e != nil {
		installFatal(f, *out, e)
	}
	if e = f.Close(); e != nil {
		_ = os.Remove(*out)
		fatal(e)
	}
	fmt.Printf("Credential installed at %s (secret was not printed).\n", *out)
}
func installFatal(f *os.File, path string, e error) {
	_ = f.Close()
	_ = os.Remove(path)
	fatal(e)
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
