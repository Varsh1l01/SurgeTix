package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d", rec.Code)
	}
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestRegistryExposesRuntimeAndProcess(t *testing.T) {
	out := scrape(t, Handler(NewRegistry()))
	for _, m := range []string{"go_goroutines", "process_cpu_seconds_total", "process_resident_memory_bytes"} {
		if !strings.Contains(out, m) {
			t.Errorf("missing %s", m)
		}
	}
}

func TestWrapRecordsREDByFixedRoute(t *testing.T) {
	reg := NewRegistry()
	h := NewHTTP(reg)
	conflict := h.Wrap("POST /v1/x/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	implicitOK := h.Wrap("GET /v1/y", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	for range 3 {
		conflict.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/x/123", nil))
	}
	implicitOK.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/y", nil))

	out := scrape(t, Handler(reg))
	for _, want := range []string{
		`http_requests_total{code="409",method="POST",route="POST /v1/x/{id}"} 3`,
		`http_requests_total{code="200",method="GET",route="GET /v1/y"} 1`,
		`http_request_duration_seconds_count{method="POST",route="POST /v1/x/{id}"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(out, "/v1/x/123") {
		t.Error("raw path leaked into labels")
	}
}
