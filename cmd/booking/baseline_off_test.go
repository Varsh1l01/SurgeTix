//go:build !unsafe_baseline

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

const baselinePath = "/v1/baseline/events/5e1f0000-0000-4000-8000-000000000001/holds"

// Normal build: one key (the runtime flag) is never enough.
func TestNormalBuildNeverServesBaseline(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		rt := &app.Runtime{Mux: http.NewServeMux()}
		env := config.FromMap(map[string]string{unsafebaseline.EnvFlag: flag})
		if err := setupBaseline(context.Background(), env, rt, nil); err != nil {
			t.Fatalf("flag %q: %v", flag, err)
		}
		rec := httptest.NewRecorder()
		rt.Mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, baselinePath, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("flag %q: baseline route answered %d, want 404", flag, rec.Code)
		}
	}

	rt := &app.Runtime{Mux: http.NewServeMux()}
	env := config.FromMap(map[string]string{unsafebaseline.EnvFlag: "true"})
	if err := setupBaseline(context.Background(), env, rt, nil); !errors.Is(err, unsafebaseline.ErrNotCompiled) {
		t.Fatalf("flag true on a normal build: err = %v, want ErrNotCompiled (refuse to start)", err)
	}
}
