package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (h *Hub) handleActors(w http.ResponseWriter, _ *http.Request) {
	actors := h.actorsSnapshot()
	response := make([]hubActor, 0, len(actors))

	for _, actor := range actors {
		response = append(response, describeActor(actor))
	}

	writeJSON(w, response)
}

func (h *Hub) handleActor(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actors[r.PathValue("actorID")]
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	writeJSON(w, describeActor(actor))
}

func (h *Hub) handleSignals(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.signalsSnapshot())
}

func (h *Hub) handleSignal(w http.ResponseWriter, r *http.Request) {
	signal, ok := h.signals[r.PathValue("signalID")]
	if !ok {
		http.Error(w, "signal not found", http.StatusNotFound)
		return
	}

	writeJSON(w, signal.describe())
}

func (h *Hub) handleSetSignal(w http.ResponseWriter, r *http.Request) {
	signal, ok := h.signals[r.PathValue("signalID")]
	if !ok {
		http.Error(w, "signal not found", http.StatusNotFound)
		return
	}

	var request struct {
		Value json.RawMessage `json:"value"`
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(raw, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Value) == 0 {
		http.Error(w, "value is required", http.StatusBadRequest)
		return
	}

	if err := signal.set(request.Value); err != nil {
		if errors.Is(err, errSignalReadOnly) {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, err.Error(), http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, signal.describe())
}

func (h *Hub) handleGraph(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.graphSnapshot())
}

func (h *Hub) handleEvents(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Event-History-Limit", strconv.Itoa(h.eventHistoryLimit))
	writeJSON(w, h.eventsSnapshot())
}

func (h *Hub) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events := make(chan []byte, 16)

	h.mu.Lock()
	h.clients[events] = struct{}{}
	h.mu.Unlock()
	flusher.Flush()

	defer func() {
		h.mu.Lock()
		delete(h.clients, events)
		h.mu.Unlock()
		close(events)
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case event := <-events:
			if _, err := w.Write(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
