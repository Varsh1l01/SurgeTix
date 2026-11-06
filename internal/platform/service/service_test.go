package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func get(t *testing.T, h http.Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: invalid JSON: %v", path, err)
	}
	return rec.Code, body
}

func TestHealthzAlwaysOK(t *testing.T) {
	s := New(Options{Name: "svc", Logger: quietLogger(), Checks: []Check{
		{Name: "dep", Fn: func(context.Context) error { return errors.New("down") }},
	}})
	code, body := get(t, s.Handler(), "/healthz")
	if code != http.StatusOK || body["status"] != "ok" {
		t.Errorf("healthz = %d %v", code, body)
	}
}

func TestReadyz(t *testing.T) {
	ok := Check{Name: "postgres", Fn: func(context.Context) error { return nil }}
	bad := Check{Name: "redis", Fn: func(context.Context) error { return errors.New("connection refused") }}
	slow := Check{Name: "kafka", Fn: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}

	tests := []struct {
		name   string
		checks []Check
		code   int
		failed string
	}{
		{"no checks", nil, http.StatusOK, ""},
		{"all pass", []Check{ok}, http.StatusOK, ""},
		{"one fails", []Check{ok, bad}, http.StatusServiceUnavailable, "redis"},
		{"times out", []Check{slow}, http.StatusServiceUnavailable, "kafka"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Options{Name: "svc", Logger: quietLogger(), CheckTimeout: 50 * time.Millisecond, Checks: tt.checks})
			code, body := get(t, s.Handler(), "/readyz")
			if code != tt.code {
				t.Fatalf("code = %d, want %d (%v)", code, tt.code, body)
			}
			if tt.failed != "" {
				checks := body["checks"].(map[string]any)
				if checks[tt.failed].(map[string]any)["status"] != "fail" {
					t.Errorf("check %s not reported as failed: %v", tt.failed, checks)
				}
			}
		})
	}
}

func TestServeShutsDownGracefully(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{Name: "svc", Logger: quietLogger(), ShutdownTimeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	url := "http://" + ln.Addr().String() + "/healthz"
	var resp *http.Response
	for i := 0; i < 50; i++ {
		if resp, err = http.Get(url); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server not reachable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if !s.draining.Load() {
		t.Error("server should be draining after shutdown")
	}
	if code, body := get(t, s.Handler(), "/readyz"); code != http.StatusServiceUnavailable || body["status"] != "draining" {
		t.Errorf("readyz while draining = %d %v", code, body)
	}
}

func TestRunFailsOnBadAddress(t *testing.T) {
	if err := Run(context.Background(), Options{Name: "svc", Addr: "not-an-address", Logger: quietLogger()}); err == nil {
		t.Fatal("expected listen error")
	}
}
