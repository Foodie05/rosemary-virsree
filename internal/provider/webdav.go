package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type WebDAVConfig struct{ Endpoint, Username, Password string }
type WebDAV struct {
	endpoint, username, password string
	client                       *http.Client
	ready                        bool
}

func NewWebDAV(c WebDAVConfig) *WebDAV {
	u, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	return &WebDAV{endpoint: strings.TrimRight(c.Endpoint, "/"), username: c.Username, password: c.Password, client: &http.Client{Timeout: 30 * time.Second}, ready: err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""}
}
func (w *WebDAV) Kind() string { return "webdav" }
func (w *WebDAV) Ready() bool  { return w.ready }
func (w *WebDAV) objectURL(key string) string {
	u, _ := url.Parse(w.endpoint)
	for _, segment := range strings.Split(strings.TrimLeft(key, "/"), "/") {
		u.Path = strings.TrimRight(u.Path, "/") + "/" + segment
	}
	return u.String()
}
func (w *WebDAV) request(ctx context.Context, method, key string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, w.objectURL(key), body)
	if err != nil {
		return nil, err
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return w.client.Do(req)
}
func (w *WebDAV) ensureParents(ctx context.Context, key string) error {
	dir := path.Dir(strings.TrimLeft(key, "/"))
	if dir == "." {
		return nil
	}
	parts := strings.Split(dir, "/")
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		cur = path.Join(cur, p)
		r, e := w.request(ctx, "MKCOL", cur, nil, nil)
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode >= 300 && r.StatusCode != 405 {
			return fmt.Errorf("MKCOL %s: %s", cur, r.Status)
		}
	}
	return nil
}
func (w *WebDAV) Probe(ctx context.Context) error {
	if !w.ready {
		return errors.New("invalid WebDAV endpoint")
	}
	key := fmt.Sprintf("rosemary-system/probes/%d", time.Now().UnixNano())
	copyKey := key + "-copy"
	if e := w.Put(ctx, key, strings.NewReader("probe"), 5, "text/plain"); e != nil {
		return fmt.Errorf("write probe: %w", e)
	}
	defer w.Delete(context.Background(), key)
	defer w.Delete(context.Background(), copyKey)
	if _, e := w.Head(ctx, key); e != nil {
		return fmt.Errorf("read probe: %w", e)
	}
	body, _, e := w.Get(ctx, key)
	if e != nil {
		return fmt.Errorf("download probe: %w", e)
	}
	content, readErr := io.ReadAll(io.LimitReader(body, 16))
	body.Close()
	if readErr != nil || string(content) != "probe" {
		return errors.New("download probe returned unexpected content")
	}
	if e := w.Copy(ctx, key, copyKey); e != nil {
		return fmt.Errorf("copy probe: %w", e)
	}
	if _, e := w.Head(ctx, copyKey); e != nil {
		return fmt.Errorf("copied object probe: %w", e)
	}
	if e := w.Delete(ctx, copyKey); e != nil {
		return fmt.Errorf("delete probe: %w", e)
	}
	if e := w.Delete(ctx, key); e != nil {
		return fmt.Errorf("delete probe: %w", e)
	}
	if _, e := w.Head(ctx, key); e == nil {
		return errors.New("delete probe: object still exists")
	}
	return nil
}
func (w *WebDAV) PresignPut(context.Context, string, string, int64, time.Duration) (string, error) {
	return "", errors.New("WebDAV does not support presigned PUT; VirSree relay is required")
}
func (w *WebDAV) PresignGet(context.Context, string, time.Duration, string) (string, error) {
	return "", errors.New("WebDAV does not support presigned GET; VirSree relay is required")
}
func (w *WebDAV) Head(ctx context.Context, key string) (Head, error) {
	r, e := w.request(ctx, "HEAD", key, nil, nil)
	if e != nil {
		return Head{}, e
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 {
		return Head{}, fmt.Errorf("WebDAV HEAD: %s", r.Status)
	}
	n, _ := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	return Head{Size: n, ETag: r.Header.Get("ETag"), ContentType: r.Header.Get("Content-Type")}, nil
}
func (w *WebDAV) Delete(ctx context.Context, key string) error {
	r, e := w.request(ctx, "DELETE", key, nil, nil)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 && r.StatusCode != 404 {
		return fmt.Errorf("WebDAV DELETE: %s", r.Status)
	}
	return nil
}
func (w *WebDAV) Copy(ctx context.Context, from, to string) error {
	if e := w.ensureParents(ctx, to); e != nil {
		return e
	}
	r, e := w.request(ctx, "COPY", from, nil, map[string]string{"Destination": w.objectURL(to), "Overwrite": "T"})
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 {
		return fmt.Errorf("WebDAV COPY: %s", r.Status)
	}
	return nil
}
func (w *WebDAV) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if e := w.ensureParents(ctx, key); e != nil {
		return e
	}
	h := map[string]string{"Content-Type": contentType, "Content-Length": strconv.FormatInt(size, 10)}
	r, e := w.request(ctx, "PUT", key, body, h)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 {
		return fmt.Errorf("WebDAV PUT: %s", r.Status)
	}
	return nil
}
func (w *WebDAV) Get(ctx context.Context, key string) (io.ReadCloser, Head, error) {
	r, e := w.request(ctx, "GET", key, nil, nil)
	if e != nil {
		return nil, Head{}, e
	}
	if r.StatusCode >= 300 {
		r.Body.Close()
		return nil, Head{}, fmt.Errorf("WebDAV GET: %s", r.Status)
	}
	n, _ := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	return r.Body, Head{Size: n, ETag: r.Header.Get("ETag"), ContentType: r.Header.Get("Content-Type")}, nil
}

var _ Backend = (*WebDAV)(nil)
