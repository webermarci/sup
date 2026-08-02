package hub

import (
	_ "embed"
	"net/http"
)

//go:embed debug.html
var debugPageHTML []byte

func serveDebugPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(debugPageHTML)
}
