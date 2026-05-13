package ui

import (
	"html/template"
	"net/http"
	"sort"
)

func dashboardPage(dashboard *Dashboard, tmpl *template.Template) http.HandlerFunc {
	type DashboardPageData struct {
		Nodes []Node
	}

	return func(w http.ResponseWriter, r *http.Request) {
		dashboard.mu.RLock()
		nodes := make([]Node, len(dashboard.nodes))
		copy(nodes, dashboard.nodes)
		dashboard.mu.RUnlock()

		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		})

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, DashboardPageData{
			Nodes: nodes,
		})
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
