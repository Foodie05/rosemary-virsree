package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

type StorageSourceInput struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Priority       int    `json:"priority"`
	CapacityBytes  int64  `json:"capacity_bytes"`
	Endpoint       string `json:"endpoint"`
	PublicEndpoint string `json:"public_endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	PathStyle      bool   `json:"path_style"`
	CDNEndpoint    string `json:"cdn_endpoint"`
	WebDAVUsername string `json:"webdav_username"`
	WebDAVPassword string `json:"webdav_password"`
}
type storedSourceConfig struct {
	Endpoint, PublicEndpoint, Region, Bucket, AccessKey, SecretKey string
	PathStyle                                                      bool
	CDNEndpoint, Username, Password                                string
}
type sourceRuntime struct {
	meta    model.StorageSource
	backend provider.Backend
}
type StorageManager struct {
	mu        sync.RWMutex
	db        *store.Store
	box       *secretbox.Box
	publicURL string
	sources   map[string]sourceRuntime
	legacy    provider.Backend
}

func NewStorageManager(db *store.Store, box *secretbox.Box, publicURL string, legacy provider.Backend) *StorageManager {
	return &StorageManager{db: db, box: box, publicURL: publicURL, sources: map[string]sourceRuntime{}, legacy: legacy}
}
func (m *StorageManager) Load(ctx context.Context) error {
	vs, e := m.db.ListStorageSources(ctx)
	if e != nil {
		return e
	}
	for _, v := range vs {
		plain, e := m.box.Open(v.ConfigCipher)
		if e != nil {
			return fmt.Errorf("open storage source %s: %w", v.ID, e)
		}
		var c storedSourceConfig
		if e = json.Unmarshal([]byte(plain), &c); e != nil {
			return e
		}
		b, e := backendFor(v.Kind, c)
		if e != nil {
			return e
		}
		m.sources[v.ID] = sourceRuntime{v, b}
	}
	return nil
}
func backendFor(kind string, c storedSourceConfig) (provider.Backend, error) {
	switch kind {
	case "s3":
		public := c.PublicEndpoint
		if public == "" {
			public = c.Endpoint
		}
		download := public
		if c.CDNEndpoint != "" {
			download = c.CDNEndpoint
		}
		return provider.New(config.Backend{Endpoint: c.Endpoint, PublicEndpoint: public, DownloadEndpoint: download, Region: c.Region, Bucket: c.Bucket, AccessKey: c.AccessKey, SecretKey: c.SecretKey, PathStyle: c.PathStyle}), nil
	case "webdav":
		return provider.NewWebDAV(provider.WebDAVConfig{Endpoint: c.Endpoint, Username: c.Username, Password: c.Password}), nil
	default:
		return nil, errors.New("storage kind must be s3 or webdav")
	}
}
func (m *StorageManager) Add(ctx context.Context, in StorageSourceInput) (model.StorageSource, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Bucket = strings.TrimSpace(in.Bucket)
	var e error
	if in.Endpoint, e = normalizeEndpoint(in.Endpoint, in.Kind == "s3"); e != nil {
		return model.StorageSource{}, e
	}
	if in.PublicEndpoint, e = normalizeEndpoint(in.PublicEndpoint, true); e != nil {
		return model.StorageSource{}, fmt.Errorf("public endpoint: %w", e)
	}
	if in.CDNEndpoint, e = normalizeEndpoint(in.CDNEndpoint, true); e != nil {
		return model.StorageSource{}, fmt.Errorf("CDN endpoint: %w", e)
	}
	if in.Kind == "s3" {
		in.Endpoint = normalizeS3ServiceEndpoint(in.Endpoint, in.Bucket)
		in.PublicEndpoint = normalizeS3ServiceEndpoint(in.PublicEndpoint, in.Bucket)
	}
	if in.Name == "" || in.CapacityBytes <= 0 {
		return model.StorageSource{}, errors.New("name and positive capacity_bytes are required")
	}
	if in.Kind == "s3" && (in.Region == "" || in.Bucket == "" || in.AccessKey == "" || in.SecretKey == "") {
		return model.StorageSource{}, errors.New("region, bucket, access_key and secret_key are required for S3")
	}
	if in.Kind == "webdav" && (in.Endpoint == "" || in.WebDAVUsername == "" || in.WebDAVPassword == "") {
		return model.StorageSource{}, errors.New("endpoint, webdav_username and webdav_password are required for WebDAV")
	}
	c := storedSourceConfig{Endpoint: in.Endpoint, PublicEndpoint: in.PublicEndpoint, Region: in.Region, Bucket: in.Bucket, AccessKey: in.AccessKey, SecretKey: in.SecretKey, PathStyle: in.PathStyle, CDNEndpoint: in.CDNEndpoint, Username: in.WebDAVUsername, Password: in.WebDAVPassword}
	b, e := backendFor(in.Kind, c)
	if e != nil {
		return model.StorageSource{}, e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if e = b.Probe(probeCtx); e != nil {
		return model.StorageSource{}, fmt.Errorf("storage verification failed: %w", e)
	}
	raw, _ := json.Marshal(c)
	cipher, e := m.box.Seal(string(raw))
	if e != nil {
		return model.StorageSource{}, e
	}
	v := model.StorageSource{ID: secretbox.Random("src_", 10), Name: in.Name, Kind: in.Kind, Priority: in.Priority, CapacityBytes: in.CapacityBytes, Enabled: true, Direct: in.Kind == "s3", CDNEnabled: in.CDNEndpoint != "", ConfigCipher: cipher, CreatedAt: time.Now().UTC()}
	if e = m.db.CreateStorageSource(ctx, v); e != nil {
		return v, e
	}
	m.mu.Lock()
	m.sources[v.ID] = sourceRuntime{v, b}
	m.mu.Unlock()
	return v, nil
}

// normalizeS3ServiceEndpoint repairs a common console-copy mistake. S3 SDK base
// endpoints describe the service; the SDK adds Bucket itself. Passing a
// bucket-scoped host such as bucket.s3.example.com would otherwise add it twice.
func normalizeS3ServiceEndpoint(endpoint, bucket string) string {
	if endpoint == "" || bucket == "" {
		return endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	host := u.Hostname()
	port := u.Port()
	prefix := strings.ToLower(bucket) + "."
	if !strings.HasPrefix(strings.ToLower(host), prefix) {
		return endpoint
	}
	serviceHost := host[len(prefix):]
	if !strings.HasPrefix(strings.ToLower(serviceHost), "s3.") {
		return endpoint
	}
	u.Host = serviceHost
	if port != "" {
		u.Host += ":" + port
	}
	return u.String()
}

func normalizeEndpoint(raw string, optional bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		if optional {
			return "", nil
		}
		return "", errors.New("endpoint is required")
	}
	value = strings.TrimLeft(value, ".")
	switch {
	case strings.HasPrefix(value, "https//"):
		value = "https://" + strings.TrimPrefix(value, "https//")
	case strings.HasPrefix(value, "http//"):
		value = "http://" + strings.TrimPrefix(value, "http//")
	case strings.HasPrefix(value, "https:/") && !strings.HasPrefix(value, "https://"):
		value = "https://" + strings.TrimPrefix(value, "https:/")
	case strings.HasPrefix(value, "http:/") && !strings.HasPrefix(value, "http://"):
		value = "http://" + strings.TrimPrefix(value, "http:/")
	case strings.HasPrefix(value, "//"):
		value = "https:" + value
	case !strings.Contains(value, "://"):
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("endpoint format is invalid; enter a domain such as s3.example.com or a complete HTTP(S) URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.TrimLeft(u.Host, ".")
	if u.Host == "" {
		return "", errors.New("endpoint format is invalid; enter a domain such as s3.example.com or a complete HTTP(S) URL")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
func (m *StorageManager) Ready() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sources {
		if s.meta.Enabled && s.backend.Ready() {
			return true
		}
	}
	return m.legacy != nil && m.legacy.Ready()
}
func (m *StorageManager) List(ctx context.Context) ([]model.StorageSource, error) {
	return m.db.ListStorageSources(ctx)
}
func (m *StorageManager) candidates() []sourceRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []sourceRuntime
	for _, s := range m.sources {
		if s.meta.Enabled && s.backend.Ready() {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].meta.Priority == out[j].meta.Priority {
			return out[i].meta.CreatedAt.Before(out[j].meta.CreatedAt)
		}
		return out[i].meta.Priority < out[j].meta.Priority
	})
	if len(out) == 0 && m.legacy != nil && m.legacy.Ready() {
		out = append(out, sourceRuntime{meta: model.StorageSource{ID: "", Name: "Legacy S3", Kind: "s3", Priority: 0, CapacityBytes: 1 << 62, Enabled: true, Direct: true}, backend: m.legacy})
	}
	return out
}
func (m *StorageManager) backend(id string) (provider.Backend, error) {
	if id == "" && m.legacy != nil {
		return m.legacy, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sources[id]
	if !ok || !s.meta.Enabled {
		return nil, errors.New("storage source unavailable")
	}
	return s.backend, nil
}

func (m *StorageManager) Direct(id string) bool {
	if id == "" {
		return m.legacy != nil && m.legacy.Ready()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sources[id]
	return ok && s.meta.Direct
}
func (m *StorageManager) relayUpload(ctx context.Context, token string, body io.Reader, contentLength int64) error {
	o, exp, e := m.db.UploadByTransferHash(ctx, secretbox.Hash(token))
	if e != nil {
		return errors.New("upload capability unavailable")
	}
	if time.Now().After(exp) || contentLength != o.Size {
		return errors.New("upload capability expired or size mismatch")
	}
	b, e := m.backend(o.SourceID)
	if e != nil {
		return e
	}
	return b.Put(ctx, o.PhysicalKey, io.LimitReader(body, o.Size), o.Size, o.ContentType)
}
func (m *StorageManager) relayDownload(ctx context.Context, token string) (io.ReadCloser, provider.Head, error) {
	o, exp, e := m.db.DownloadByTransferHash(ctx, secretbox.Hash(token))
	if e != nil || time.Now().After(exp) {
		return nil, provider.Head{}, errors.New("download capability unavailable")
	}
	b, e := m.backend(o.SourceID)
	if e != nil {
		return nil, provider.Head{}, e
	}
	return b.Get(ctx, o.PhysicalKey)
}
