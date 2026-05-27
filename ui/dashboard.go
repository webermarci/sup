package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/webermarci/sup/control"
	"github.com/webermarci/sup/ui/frontend"
)

// Dashboard serves the web-based dashboard frontend and mounts the control registry API.
type Dashboard struct {
	registry *control.Registry
}

// NewDashboard creates a new Dashboard instance with the given control registry.
func NewDashboard(registry *control.Registry) *Dashboard {
	if registry == nil {
		panic("sup: nil control registry")
	}
	return &Dashboard{registry: registry}
}

// Handler returns an http.Handler that serves the dashboard API and frontend assets.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/api/", http.StripPrefix("/api", d.registry.Handler()))
	mux.Handle("/", svelteKitHandler(frontend.FileSystem(), "/"))

	return cors()(mux)
}

func cors() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func svelteKitHandler(fileSystem fs.FS, path string) http.Handler {
	httpFileSystem := http.FS(fileSystem)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, path)

		_, err := httpFileSystem.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			path = fmt.Sprintf("%s.html", path)
		}

		r.URL.Path = path
		http.FileServer(httpFileSystem).ServeHTTP(w, r)
	})
}
