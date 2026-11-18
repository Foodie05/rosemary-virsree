package httpapi

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The same canonical document URL serves a browser reader or machine-readable
// Markdown. Cache negotiation must include Accept to keep CDN variants separate.
func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/docs/")
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(name, ".md") {
		http.NotFound(w, r)
		return
	}
	doc := filepath.Join(s.svc.Config.DocsDir, name)
	if info, err := os.Stat(doc); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Add("Vary", "Accept")
	noStore(w)
	if r.URL.Query().Get("raw") != "1" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		index := filepath.Join(s.svc.Config.WebDir, "index.html")
		if _, err := os.Stat(index); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, index)
			return
		}
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeFile(w, r, doc)
}
