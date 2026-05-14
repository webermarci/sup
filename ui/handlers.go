package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
)

func cors() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
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

func getNodes(dashboard *Dashboard) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		dashboard.mu.RLock()
		nodes := make([]Node, len(dashboard.nodes))
		copy(nodes, dashboard.nodes)
		dashboard.mu.RUnlock()

		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		})

		json.NewEncoder(w).Encode(nodes)
	}
}

func getEvents(dashboard *Dashboard) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		clientCh := make(chan []byte, 16)
		dashboard.mu.Lock()
		dashboard.clients[clientCh] = struct{}{}
		dashboard.mu.Unlock()

		defer func() {
			dashboard.mu.Lock()
			delete(dashboard.clients, clientCh)
			dashboard.mu.Unlock()
			close(clientCh)
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-clientCh:
				w.Write(msg)
				flusher.Flush()
			}
		}
	}
}
