//go:build unsafe_baseline

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

const baselinePath = "/v1/baseline/events/5e1f0000-0000-4000-8000-000000000001/holds"

// Tagged build without the runtime flag: still nothing is served, and nothing
// touches the database (the pool is nil here and must not be used).
func TestTaggedBuildNeedsRuntimeFlag(t *testing.T) {
	for _, flag := range []string{"", "false", "0"} {
		rt := &app.Runtime{Mux: http.NewServeMux()}
		env := config.FromMap(map[string]string{unsafebaseline.EnvFlag: flag})
		if err := setupBaseline(env, rt); err != nil {
			t.Fatalf("flag %q: %v", flag, err)
		}
		rec := httptest.NewRecorder()
		rt.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, baselinePath, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("flag %q: baseline route answered %d, want 404", flag, rec.Code)
		}
	}
}

func TestTaggedBuildWithFlagNeedsBookingURL(t *testing.T) {
	env := config.FromMap(map[string]string{unsafebaseline.EnvFlag: "true"})
	rt := &app.Runtime{Mux: http.NewServeMux()}
	if err := setupBaseline(env, rt); err == nil {
		t.Fatal("expected an error without GATEWAY_BOOKING_URL")
	}

	env = config.FromMap(map[string]string{unsafebaseline.EnvFlag: "true", "GATEWAY_BOOKING_URL": "http://127.0.0.1:1"})
	rt = &app.Runtime{Mux: http.NewServeMux(), Metrics: metrics.NewRegistry()}
	if err := setupBaseline(env, rt); err != nil || env.Err() != nil {
		t.Fatalf("setup: %v %v", err, env.Err())
	}
	rec := httptest.NewRecorder()
	rt.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, baselinePath, nil))
	if rec.Code != http.StatusBadGateway { // route exists; upstream is down
		t.Fatalf("with both keys the route must exist: got %d, want 502", rec.Code)
	}
}
