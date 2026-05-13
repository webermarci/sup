package ui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webermarci/sup"
)

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

func TestDashboard_Inspect(t *testing.T) {
	p := sup.NewPushedSignal("pushed", func(ctx context.Context, v int) error { return nil })
	d := NewDashboard("dashboard", WithObserve(p))
	spec := d.Inspect()

	if spec.Kind != "dashboard" {
		t.Fatalf("expected kind dashboard, got %q", spec.Kind)
	}

	if got := spec.Metadata["observed_count"]; got != "1" {
		t.Fatalf("expected observed_count=%q, got %q", "1", got)
	}
}
