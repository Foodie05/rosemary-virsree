package s3compat

import (
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rosemary-virsree/internal/service"
)

type Gateway struct{ svc *service.Service }

func New(s *service.Service) *Gateway { return &Gateway{svc: s} }

type listResult struct {
	XMLName      xml.Name `xml:"ListBucketResult"`
	Xmlns        string   `xml:"xmlns,attr"`
	Name, Prefix string
	KeyCount     int       `xml:"KeyCount"`
	MaxKeys      int       `xml:"MaxKeys"`
	IsTruncated  bool      `xml:"IsTruncated"`
	Contents     []content `xml:"Contents"`
}
type content struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	perm := "read"
	if r.Method == "DELETE" {
		perm = "delete"
	}
	c, expires, e := g.authenticate(r, perm, bucket)
	if e != nil {
		s3error(w, r, 403, "AccessDenied", "当前虚拟凭据无权执行此操作。", "The virtual credentials do not permit this operation.")
		return
	}
	if key == "" && r.Method == "GET" {
		g.list(w, r, c)
		return
	}
	if key == "" {
		s3error(w, r, 405, "MethodNotAllowed", "VirSree 不支持这个桶级操作。", "VirSree does not support this bucket-level operation.")
		return
	}
	switch r.Method {
	case "GET":
		if expires < 1 {
			s3error(w, r, 400, "InvalidRequest", "应用必须通过预签名请求、rvs-expires 或 X-RVS-Expires-In 指定有效期。", "The application must choose an expiry through a presigned request, rvs-expires, or X-RVS-Expires-In.")
			return
		}
		u, e := g.svc.DownloadURL(r.Context(), c, key, expires, "")
		if e != nil {
			s3error(w, r, 404, "NoSuchKey", "对象不存在或已经失效。", "The object does not exist or is no longer available.")
			return
		}
		w.Header().Set("Location", u)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusTemporaryRedirect)
	case "HEAD":
		o, e := g.svc.DB.GetObject(r.Context(), c.Bucket.ID, key)
		if e != nil || o.Status != "ready" {
			s3error(w, r, 404, "NoSuchKey", "对象不存在或尚未就绪。", "The object does not exist or is not ready.")
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
		w.Header().Set("Content-Type", o.ContentType)
		w.Header().Set("ETag", o.ETag)
		w.WriteHeader(200)
	case "DELETE":
		if e := g.svc.Delete(r.Context(), c, key); e != nil {
			s3error(w, r, 404, "NoSuchKey", "对象不存在或无法删除。", "The object does not exist or could not be deleted.")
			return
		}
		w.WriteHeader(204)
	default:
		s3error(w, r, 405, "MethodNotAllowed", "VirSree 不支持这个 S3 操作。", "VirSree does not support this S3 operation.")
	}
}
func (g *Gateway) list(w http.ResponseWriter, r *http.Request, c service.Credential) {
	prefix := r.URL.Query().Get("prefix")
	objs, e := g.svc.DB.ListObjects(r.Context(), c.Bucket.ID, prefix)
	if e != nil {
		s3error(w, r, 500, "InternalError", "VirSree 暂时无法列出对象，请使用请求追踪编号排查。", "VirSree could not list objects. Use the request trace ID to investigate.")
		return
	}
	out := listResult{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/", Name: c.Bucket.Slug, Prefix: prefix, KeyCount: len(objs), MaxKeys: 1000}
	for _, o := range objs {
		out.Contents = append(out.Contents, content{Key: o.LogicalKey, LastModified: o.UpdatedAt.UTC().Format(time.RFC3339), ETag: o.ETag, Size: o.Size, StorageClass: "STANDARD"})
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(out)
}
func s3error(w http.ResponseWriter, r *http.Request, status int, code, zh, en string) {
	message := en
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept-Language"))), "zh") {
		message = zh
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("X-VirSree-Error-Code", code)
	w.WriteHeader(status)
	_ = xml.NewEncoder(w).Encode(struct {
		XMLName                  xml.Name `xml:"Error"`
		Code, Message, RequestID string
	}{Code: code, Message: message, RequestID: w.Header().Get("X-Request-ID")})
}
