package ui

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboard_StaticHandler(t *testing.T) {
	dashboard := &Dashboard{
		schema: []Card{
			{Name: "sensor-1", Type: "boolean"},
			{Name: "sensor-2", Type: "number"},
		},
		lastValues: map[string]any{
			"sensor-1": true,
			"sensor-2": 42.5,
		},
	}

	templ := template.Must(template.ParseFS(staticFS, "static/index.html"))

	handler := staticHandler(dashboard, templ)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %v", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %v", contentType)
	}

	bodyBytes, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("could not read response body: %v", err)
	}
	body := string(bodyBytes)

	if !strings.Contains(body, "sensor-1") || !strings.Contains(body, "sensor-2") {
		t.Errorf("expected HTML to contain sensor names, got:\n%s", body)
	}

	if !strings.Contains(body, "true") {
		t.Errorf("expected HTML to contain boolean value 'true', got:\n%s", body)
	}

	if !strings.Contains(body, "42.5") {
		t.Errorf("expected HTML to contain number value '42.5', got:\n%s", body)
	}
}

func TestDashboard_ServesCSS(t *testing.T) {
	d := NewDashboard(t.Name())

	handler := d.Handler()

	req := httptest.NewRequest(http.MethodGet, "/static/theme.css", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %v", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/css") {
		t.Errorf("expected Content-Type to contain text/css, got %q", contentType)
	}

	bodyBytes, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("could not read response body: %v", err)
	}
	body := string(bodyBytes)

	if len(body) == 0 {
		t.Error("expected CSS body, got empty response")
	}
	if !strings.Contains(body, "var(--bg-page)") {
		t.Errorf("expected CSS body to contain our theme variables, got:\n%s", body)
	}
}
