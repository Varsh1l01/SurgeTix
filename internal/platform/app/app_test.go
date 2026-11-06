package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func TestRunExitsOnMissingConfigAndRunsClosers(t *testing.T) {
	closed := false
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", func(_ context.Context, env *config.Env) ([]service.Check, []func(), error) {
		env.Required("SURGETIX_APPTEST_DEFINITELY_UNSET")
		return nil, []func(){func() { closed = true }}, nil
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !closed {
		t.Error("closers must run even when configuration is invalid")
	}
}

func TestRunExitsOnSetupError(t *testing.T) {
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", func(context.Context, *config.Env) ([]service.Check, []func(), error) {
		return nil, nil, errors.New("boom")
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunExitsOnInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "loud")
	code := run("svc", "SURGETIX_APPTEST", "127.0.0.1:0", func(context.Context, *config.Env) ([]service.Check, []func(), error) {
		return nil, nil, nil
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
