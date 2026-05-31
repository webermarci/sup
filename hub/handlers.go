package hub

import (
	"io"
	"net/http"
	"time"

	"github.com/webermarci/sup"
)

func (h *Hub) handleActors(w http.ResponseWriter, r *http.Request) {
	actors := h.actorsSnapshot()
	res := make([]hubActor, 0, len(actors))

	for _, actor := range actors {
		res = append(res, describeActor(actor))
	}

	writeJSON(w, res)
}

func (h *Hub) handleActor(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r.PathValue("actorID"))
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	writeJSON(w, describeActor(actor))
}

func (h *Hub) handleControls(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r.PathValue("actorID"))
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	writeJSON(w, describeControls(actor))
}

func (h *Hub) handleSignals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.signalsSnapshot())
}

func (h *Hub) handleSignal(w http.ResponseWriter, r *http.Request) {
	signal, ok := h.signal(r.PathValue("signalID"))
	if !ok {
		http.Error(w, "signal not found", http.StatusNotFound)
		return
	}

	writeJSON(w, signal)
}

func (h *Hub) handleEvents(w http.ResponseWriter, r *http.Request) {
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

	ch := make(chan []byte, 16)

	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
		close(ch)
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

		case msg := <-ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Hub) handleControl(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r.PathValue("actorID"))
	if !ok {
		http.Error(w, "actor not found", http.StatusNotFound)
		return
	}

	control, ok := findControl(actor, r.PathValue("controlName"))
	if !ok {
		http.Error(w, "control not found", http.StatusNotFound)
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch control := control.(type) {
	case sup.CastControl:
		if err := control.Dispatch(r.Context(), raw); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	case sup.CallControl:
		res, err := control.Dispatch(r.Context(), raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, res)

	default:
		http.Error(w, "unsupported control", http.StatusInternalServerError)
	}
}

func (h *Hub) serveDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, err := w.Write(debugIndex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
