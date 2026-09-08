package s3compat

import (
	"encoding/xml"
	"net/http"
	"strconv"
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
		s3error(w, 403, "AccessDenied", e.Error())
		return
	}
	if key == "" && r.Method == "GET" {
		g.list(w, r, c)
		return
	}
	if key == "" {
		s3error(w, 405, "MethodNotAllowed", "bucket operation not supported")
		return
	}
	switch r.Method {
	case "GET":
		if expires < 1 {
			s3error(w, 400, "InvalidRequest", "application must choose expiry with a presigned request or rvs-expires")
			return
		}
		u, e := g.svc.DownloadURL(r.Context(), c, key, expires, "")
		if e != nil {
			s3error(w, 404, "NoSuchKey", e.Error())
			return
		}
		w.Header().Set("Location", u)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusTemporaryRedirect)
	case "HEAD":
		o, e := g.svc.DB.GetObject(r.Context(), c.Bucket.ID, key)
		if e != nil || o.Status != "ready" {
			s3error(w, 404, "NoSuchKey", "object not found")
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
		w.Header().Set("Content-Type", o.ContentType)
		w.Header().Set("ETag", o.ETag)
		w.WriteHeader(200)
	case "DELETE":
		if e := g.svc.Delete(r.Context(), c, key); e != nil {
			s3error(w, 404, "NoSuchKey", e.Error())
			return
		}
		w.WriteHeader(204)
	default:
		s3error(w, 405, "MethodNotAllowed", "operation not supported")
	}
}
func (g *Gateway) list(w http.ResponseWriter, r *http.Request, c service.Credential) {
	prefix := r.URL.Query().Get("prefix")
	objs, e := g.svc.DB.ListObjects(r.Context(), c.Bucket.ID, prefix)
	if e != nil {
		s3error(w, 500, "InternalError", e.Error())
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
func s3error(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_ = xml.NewEncoder(w).Encode(struct {
		XMLName       xml.Name `xml:"Error"`
		Code, Message string
	}{Code: code, Message: msg})
}
