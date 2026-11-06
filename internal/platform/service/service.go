// Package service runs a SurgeTix process: an HTTP listener with liveness
// and readiness endpoints, and graceful shutdown when the context ends.
//
// /healthz reports that the process is alive. /readyz runs the dependency
// checks the service declares and returns 503 if any fails, or while the
// process is shutting down, so a load balancer or Kubernetes probe stops
// sending traffic before the listener closes (NFR-OPS, PDF p.8).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Check is a named dependency probe, e.g. a Postgres ping.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// Options configures Run.
type Options struct {
	Name            string
	Addr            string
	ShutdownTimeout time.Duration
	CheckTimeout    time.Duration // per readiness check; default 2s
	Logger          *slog.Logger
	Checks          []Check
}

// Server is the HTTP side of a service. It is separate from Run so tests can
// drive the handler directly.
type Server struct {
	opts     Options
	draining atomic.Bool
}

// New returns a Server for opts.
func New(opts Options) *Server {
	if opts.CheckTimeout <= 0 {
		opts.CheckTimeout = 2 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{opts: opts}
}

// Handler serves /healthz and /readyz.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": s.opts.Name})
	})
	mux.HandleFunc("GET /readyz", s.ready)
	return mux
}

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "draining", "service": s.opts.Name})
		return
	}
	results := make(map[string]checkResult, len(s.opts.Checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range s.opts.Checks {
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), s.opts.CheckTimeout)
			defer cancel()
			res := checkResult{Status: "ok"}
			if err := c.Fn(ctx); err != nil {
				res = checkResult{Status: "fail", Error: err.Error()}
			}
			mu.Lock()
			results[c.Name] = res
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	status, code := "ok", http.StatusOK
	for name, res := range results {
		if res.Status != "ok" {
			status, code = "unavailable", http.StatusServiceUnavailable
			s.opts.Logger.Warn("readiness check failed", "check", name, "err", res.Error)
		}
	}
	writeJSON(w, code, map[string]any{"status": status, "service": s.opts.Name, "checks": results})
}

// Serve listens on ln until ctx is done, then drains and shuts down.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}
	log := s.opts.Logger
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("service started", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	s.draining.Store(true)
	log.Info("shutting down", "timeout", s.opts.ShutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("service stopped")
	return nil
}

// Run listens on opts.Addr and serves until ctx is done.
func Run(ctx context.Context, opts Options) error {
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return err
	}
	return New(opts).Serve(ctx, ln)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
