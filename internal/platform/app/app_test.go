package app

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func TestRunExitsOnMissingConfigAndRunsClosers(t *testing.T) {
	closed := false
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", withoutRuntime(func(_ context.Context, env *config.Env) ([]service.Check, []func(), error) {
		env.Required("SURGETIX_APPTEST_DEFINITELY_UNSET")
		return nil, []func(){func() { closed = true }}, nil
	}))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !closed {
		t.Error("closers must run even when configuration is invalid")
	}
}

func TestRunExitsOnSetupError(t *testing.T) {
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", withoutRuntime(func(context.Context, *config.Env) ([]service.Check, []func(), error) {
		return nil, nil, errors.New("boom")
	}))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunExitsOnInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "loud")
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", withoutRuntime(func(context.Context, *config.Env) ([]service.Check, []func(), error) {
		return nil, nil, nil
	}))
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRuntimeRoutesOnlyWhenRegistered(t *testing.T) {
	rt := &Runtime{Mux: http.NewServeMux()}
	if rt.routesHandler() != nil {
		t.Fatal("no routes registered: handler must be nil (Phase 1 behaviour)")
	}
	rt.Handle("GET /v1/x", http.NotFoundHandler())
	if rt.routesHandler() == nil {
		t.Fatal("routes registered: handler must be set")
	}
}
