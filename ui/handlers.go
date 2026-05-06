package ui

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
)

func staticHandler(dashboard *Dashboard, templ *template.Template) func(w http.ResponseWriter, r *http.Request) {
	type templateData struct {
		Rows       []Row
		LastValues map[string]string
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		dashboard.mu.RLock()

		sortedRows := make([]Row, len(dashboard.schema))
		copy(sortedRows, dashboard.schema)

		sort.Slice(sortedRows, func(i, j int) bool {
			return sortedRows[i].Name < sortedRows[j].Name
		})

		values := make(map[string]string)
		for _, row := range sortedRows {
			if val, exists := dashboard.lastValues[row.Name]; exists {
				switch v := val.(type) {
				case string:
					values[row.Name] = v
				case []byte:
					values[row.Name] = string(v)
				default:
					b, _ := json.Marshal(v)
					values[row.Name] = string(b)
				}
			} else {
				values[row.Name] = ""
			}
		}

		data := templateData{
			Rows:       sortedRows,
			LastValues: values,
		}
		dashboard.mu.RUnlock()

		if err := templ.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
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
