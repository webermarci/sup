package ui

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
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

func staticHandler(dashboard *Dashboard, templ *template.Template) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		dashboard.mu.RLock()

		sortedCards := make([]Card, len(dashboard.schema))
		copy(sortedCards, dashboard.schema)

		sort.Slice(sortedCards, func(i, j int) bool {
			return sortedCards[i].Name < sortedCards[j].Name
		})

		values := make(map[string]string)
		for _, card := range sortedCards {
			if val, exists := dashboard.lastValues[card.Name]; exists {
				switch v := val.(type) {
				case string:
					values[card.Name] = v
				case []byte:
					values[card.Name] = string(v)
				default:
					b, _ := json.Marshal(v)
					values[card.Name] = string(b)
				}
			} else {
				values[card.Name] = ""
			}
		}

		data := TemplateData{
			Cards:      sortedCards,
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
