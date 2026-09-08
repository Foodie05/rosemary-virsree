package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestWebDAVProbeExercisesDataPlane(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		case "COPY":
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			destination, _ := url.Parse(r.Header.Get("Destination"))
			objects[destination.Path] = append([]byte(nil), body...)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	backend := NewWebDAV(WebDAVConfig{Endpoint: server.URL + "/root", Username: "user", Password: "password"})
	if err := backend.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := backend.objectURL("folder/name?literal.txt"); !strings.Contains(got, "name%3Fliteral.txt") {
		t.Fatalf("object key was not URL escaped: %s", got)
	}
}
