package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rosemary-virsree/internal/s3compat"
	"rosemary-virsree/internal/service"
)

type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

func New(svc *service.Service) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.log(s.mux)) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"status": "ok", "backend_ready": s.svc.S3.Ready()})
	})
	s.mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"gateway_url": s.svc.Config.PublicURL, "project_url": s.svc.Config.ProjectURL, "release_url": s.svc.Config.ReleaseURL, "docs_url": s.svc.Config.PublicURL + "/docs/integration.md"})
	})
	s.mux.HandleFunc("GET /api/v1/overview", s.admin(s.overview))
	s.mux.HandleFunc("GET /api/v1/buckets", s.admin(s.buckets))
	s.mux.HandleFunc("GET /api/v1/access-keys", s.admin(s.keys))
	s.mux.HandleFunc("POST /api/v1/bootstrap-tokens", s.admin(s.bootstrap))
	s.mux.HandleFunc("POST /api/v1/agent/claim", s.claim)
	s.mux.HandleFunc("POST /api/v1/buckets", s.admin(s.createBucket))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/access-keys", s.admin(s.createKey))
	s.mux.HandleFunc("DELETE /api/v1/access-keys/{id}", s.admin(s.revokeKey))
	s.mux.HandleFunc("GET /api/v1/admin/buckets/{bucket}/objects", s.admin(s.adminObjects))
	s.mux.HandleFunc("POST /api/v1/admin/buckets/{bucket}/objects/download", s.admin(s.adminDownload))
	s.mux.HandleFunc("POST /api/v1/admin/buckets/{bucket}/objects/invalidate-links", s.admin(s.adminInvalidateLinks))
	s.mux.HandleFunc("DELETE /api/v1/admin/buckets/{bucket}/objects/{key...}", s.admin(s.adminDeleteObject))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/upload", s.virtual("write", s.beginUpload))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/commit", s.virtual("write", s.commit))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/download", s.virtual("read", s.download))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/public-link", s.virtual("manage", s.publicLink))
	s.mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/public-links/{slug}", s.virtual("manage", s.revokePublicLink))
	s.mux.HandleFunc("DELETE /api/v1/buckets/{bucket}/objects/{key...}", s.virtual("delete", s.deleteObject))
	s.mux.HandleFunc("GET /p/{slug}", s.publicRedirect)
	s.mux.HandleFunc("GET /downloads/rvsctl/{os}/{arch}", s.downloadCLI)
	s.mux.HandleFunc("GET /downloads/rvsctl/SHA256SUMS", s.downloadChecksums)
	s.mux.Handle("GET /docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir(s.svc.Config.DocsDir))))
	s.mux.HandleFunc("POST /api/v1/buckets/{bucket}/objects/invalidate-links", s.virtual("manage", s.invalidateLinks))
	gw := s3compat.New(s.svc)
	for _, method := range []string{"GET", "HEAD", "DELETE"} {
		s.mux.Handle(method+" /s3/{bucket}", gw)
		s.mux.Handle(method+" /s3/{bucket}/{key...}", gw)
	}
	if st, e := os.Stat(s.svc.Config.WebDir); e == nil && st.IsDir() {
		fs := http.FileServer(http.Dir(s.svc.Config.WebDir))
		s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			p := filepath.Join(s.svc.Config.WebDir, filepath.Clean(r.URL.Path))
			if info, e := os.Stat(p); e == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(s.svc.Config.WebDir, "index.html"))
		})
	}
}
func (s *Server) downloadChecksums(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.svc.Config.DownloadDir, "rvsctl", "SHA256SUMS"))
}
func (s *Server) downloadCLI(w http.ResponseWriter, r *http.Request) {
	osName, arch := r.PathValue("os"), r.PathValue("arch")
	if (osName != "darwin" && osName != "linux" && osName != "windows") || (arch != "amd64" && arch != "arm64") {
		fail(w, 404, "unsupported rvsctl platform")
		return
	}
	name := "rvsctl"
	if osName == "windows" {
		name += ".exe"
	}
	path := filepath.Join(s.svc.Config.DownloadDir, "rvsctl", osName, arch, name)
	info, e := os.Stat(path)
	if e != nil || info.IsDir() {
		fail(w, 404, "rvsctl build is not available for this platform")
		return
	}
	f, e := os.Open(path)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s-%s%s"`, "rvsctl", osName, arch, map[bool]string{true: ".exe"}[osName == "windows"]))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, name, info.ModTime(), f)
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.svc.Config.AdminToken {
			fail(w, 401, "admin authentication required")
			return
		}
		next(w, r)
	}
}

type virtualHandler func(http.ResponseWriter, *http.Request, service.Credential)

func (s *Server) virtual(permission string, next virtualHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, e := s.svc.Authenticate(r.Context(), r.Header.Get("X-RVS-Access-Key"), r.Header.Get("X-RVS-Secret-Key"), permission)
		if e != nil {
			fail(w, 403, e.Error())
			return
		}
		if c.Bucket.Slug != r.PathValue("bucket") {
			fail(w, 403, "credential does not control this bucket")
			return
		}
		next(w, r, c)
	}
}
func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]any{"error": msg})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	v, e := s.svc.DB.Overview(r.Context())
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	v["total_quota"] = s.svc.Config.TotalQuota
	v["backend_ready"] = s.svc.S3.Ready()
	write(w, 200, v)
}
func (s *Server) buckets(w http.ResponseWriter, r *http.Request) {
	v, e := s.svc.DB.ListBuckets(r.Context())
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	write(w, 200, v)
}
func (s *Server) createBucket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		Visibility string `json:"visibility"`
		Quota      int64  `json:"quota_bytes"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	b, e := s.svc.NewBucket(r.Context(), in.Name, in.Slug, in.Visibility, in.Quota)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	k, secret, e := s.svc.NewKey(r.Context(), b, "owner", "read,write,delete,manage")
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	write(w, 201, map[string]any{"bucket": b, "access_key": k.AK, "secret_key": secret, "shown_once": true})
}
func (s *Server) keys(w http.ResponseWriter, r *http.Request) {
	v, e := s.svc.DB.ListKeys(r.Context())
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	write(w, 200, v)
}
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	bucket, e := s.svc.DB.GetBucket(r.Context(), r.PathValue("bucket"))
	if e != nil {
		fail(w, 404, "virtual bucket not found")
		return
	}
	var in struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if e = decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	key, secret, e := s.svc.NewKey(r.Context(), bucket, in.Name, strings.Join(in.Permissions, ","))
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 201, map[string]any{"id": key.ID, "bucket": bucket.Slug, "name": key.Name, "access_key": key.AK, "secret_key": secret, "permissions": strings.Split(key.Permissions, ","), "shown_once": true})
}
func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	if e := s.svc.DB.RevokeKey(r.Context(), r.PathValue("id")); e != nil {
		fail(w, 500, e.Error())
		return
	}
	s.svc.DB.Audit(r.Context(), "access-key.revoked", r.PathValue("id"), "admin")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) adminCredential(r *http.Request) (service.Credential, error) {
	bucket, e := s.svc.DB.GetBucket(r.Context(), r.PathValue("bucket"))
	return service.Credential{Bucket: bucket}, e
}
func (s *Server) adminObjects(w http.ResponseWriter, r *http.Request) {
	c, e := s.adminCredential(r)
	if e != nil {
		fail(w, 404, "virtual bucket not found")
		return
	}
	objects, e := s.svc.DB.ListObjects(r.Context(), c.Bucket.ID, r.URL.Query().Get("prefix"))
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	write(w, 200, objects)
}
func (s *Server) adminDownload(w http.ResponseWriter, r *http.Request) {
	c, e := s.adminCredential(r)
	if e != nil {
		fail(w, 404, "virtual bucket not found")
		return
	}
	var in struct {
		Key       string `json:"key"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if e = decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	u, e := s.svc.DownloadURL(r.Context(), c, in.Key, in.ExpiresIn, "")
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 200, map[string]any{"url": u, "expires_in": in.ExpiresIn, "direct": true})
}
func (s *Server) adminInvalidateLinks(w http.ResponseWriter, r *http.Request) {
	c, e := s.adminCredential(r)
	if e != nil {
		fail(w, 404, "virtual bucket not found")
		return
	}
	var in struct {
		Key string `json:"key"`
	}
	if e = decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if e = s.svc.RotateObjectKey(r.Context(), c, in.Key); e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 200, map[string]any{"status": "invalidated"})
}
func (s *Server) adminDeleteObject(w http.ResponseWriter, r *http.Request) {
	c, e := s.adminCredential(r)
	if e != nil {
		fail(w, 404, "virtual bucket not found")
		return
	}
	if e = s.svc.Delete(r.Context(), c, r.PathValue("key")); e != nil {
		fail(w, 400, e.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		MaxQuota  int64  `json:"max_quota_bytes"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	raw, t, e := s.svc.CreateBootstrap(r.Context(), in.Name, in.MaxQuota, time.Duration(in.ExpiresIn)*time.Second)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 201, map[string]any{"token": raw, "expires_at": t.ExpiresAt, "max_quota_bytes": t.MaxQuota, "shown_once": true, "project_url": s.svc.Config.ProjectURL, "release_url": s.svc.Config.ReleaseURL})
}
func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token      string `json:"token"`
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		Visibility string `json:"visibility"`
		Quota      int64  `json:"quota_bytes"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	b, k, secret, e := s.svc.Claim(r.Context(), in.Token, in.Name, in.Slug, in.Visibility, in.Quota)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 201, map[string]any{"bucket": b.Slug, "endpoint": s.svc.Config.PublicURL + "/s3", "region": s.svc.Config.Backend.Region, "access_key": k.AK, "secret_key": secret, "shown_once": true, "permissions": strings.Split(k.Permissions, ",")})
}
func (s *Server) beginUpload(w http.ResponseWriter, r *http.Request, c service.Credential) {
	var in struct {
		Key         string `json:"key"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	o, u, e := s.svc.BeginUpload(r.Context(), c, in.Key, in.ContentType, in.Size, in.ExpiresIn)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 201, map[string]any{"upload_id": o.ID, "method": "PUT", "url": u, "expires_in": in.ExpiresIn, "expected_size": in.Size, "required_headers": map[string]string{"Content-Type": in.ContentType}, "commit_url": fmt.Sprintf("%s/api/v1/buckets/%s/objects/commit", s.svc.Config.PublicURL, c.Bucket.Slug)})
}
func (s *Server) commit(w http.ResponseWriter, r *http.Request, c service.Credential) {
	var in struct {
		UploadID string `json:"upload_id"`
		Key      string `json:"key"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	o, e := s.svc.CommitUpload(r.Context(), c, in.UploadID, in.Key)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 200, o)
}
func (s *Server) download(w http.ResponseWriter, r *http.Request, c service.Credential) {
	var in struct {
		Key       string `json:"key"`
		Filename  string `json:"filename"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	u, e := s.svc.DownloadURL(r.Context(), c, in.Key, in.ExpiresIn, in.Filename)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 200, map[string]any{"url": u, "expires_in": in.ExpiresIn, "direct": true})
}
func (s *Server) publicLink(w http.ResponseWriter, r *http.Request, c service.Credential) {
	var in struct {
		Key           string `json:"key"`
		SignExpiresIn int64  `json:"sign_expires_in"`
		LinkExpiresIn int64  `json:"link_expires_in"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	slug, stable, direct, e := s.svc.NewPublicLink(r.Context(), c, in.Key, in.SignExpiresIn, in.LinkExpiresIn)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 201, map[string]any{"slug": slug, "public_url": stable, "direct_url": direct, "sign_expires_in": in.SignExpiresIn, "link_expires_in": in.LinkExpiresIn})
}
func (s *Server) revokePublicLink(w http.ResponseWriter, r *http.Request, c service.Credential) {
	if e := s.svc.RevokePublicLink(r.Context(), c, r.PathValue("slug")); e != nil {
		if service.IsNotFound(e) {
			fail(w, 404, "public link not found")
			return
		}
		fail(w, 400, e.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) invalidateLinks(w http.ResponseWriter, r *http.Request, c service.Credential) {
	var in struct {
		Key string `json:"key"`
	}
	if e := decode(r, &in); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if e := s.svc.RotateObjectKey(r.Context(), c, in.Key); e != nil {
		fail(w, 400, e.Error())
		return
	}
	write(w, 200, map[string]any{"status": "invalidated", "note": "previous direct URLs target a deleted physical key"})
}
func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, c service.Credential) {
	if e := s.svc.Delete(r.Context(), c, r.PathValue("key")); e != nil {
		fail(w, 400, e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) publicRedirect(w http.ResponseWriter, r *http.Request) {
	u, e := s.svc.ResolvePublic(r.Context(), r.PathValue("slug"))
	if e != nil {
		fail(w, 404, "link unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, u, http.StatusTemporaryRedirect)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if o := r.Header.Get("Origin"); o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-RVS-Access-Key,X-RVS-Secret-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start), "request_id", strconv.FormatInt(time.Now().UnixNano(), 36))
	})
}
