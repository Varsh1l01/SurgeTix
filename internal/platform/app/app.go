// Package app is the common entry point for every SurgeTix binary: load
// configuration, build the logger, create the service's own dependency
// clients, serve health endpoints and shut down cleanly on SIGINT/SIGTERM.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/logging"
	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// Setup reads the service's own settings from env and creates its dependency
// clients. Clients must connect lazily so a missing dependency shows up in
// /readyz instead of preventing start-up. closers run in reverse order on exit,
// even when Setup returns an error.
type Setup func(ctx context.Context, env *config.Env) (checks []service.Check, closers []func(), err error)

// Runtime gives a service's setup the places to attach application routes
// and its own metrics. Every process serves GET /metrics.
type Runtime struct {
	Mux     *http.ServeMux
	Metrics *prometheus.Registry
	routes  bool
}

// Handle registers an application route on the service's listener.
func (rt *Runtime) Handle(pattern string, h http.Handler) {
	rt.Mux.Handle(pattern, h)
	rt.routes = true
}

// SetupWithRuntime is Setup with access to the Runtime.
type SetupWithRuntime func(ctx context.Context, env *config.Env, rt *Runtime) (checks []service.Check, closers []func(), err error)

// Main runs a service and exits the process with its status.
// envPrefix scopes the listen address variable, e.g. "BOOKING" → BOOKING_HTTP_ADDR.
func Main(name, envPrefix, defaultAddr string, setup Setup) {
	MainWithRuntime(name, envPrefix, defaultAddr, withoutRuntime(setup))
}

func withoutRuntime(setup Setup) SetupWithRuntime {
	return func(ctx context.Context, env *config.Env, _ *Runtime) ([]service.Check, []func(), error) {
		return setup(ctx, env)
	}
}

// MainWithRuntime is Main for services that serve application routes.
func MainWithRuntime(name, envPrefix, defaultAddr string, setup SetupWithRuntime) {
	os.Exit(run(name, envPrefix, defaultAddr, setup))
}

func run(name, envPrefix, defaultAddr string, setup SetupWithRuntime) int {
	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "%s: loading .env: %v\n", name, err)
		return 1
	}
	env := config.FromOS()
	common := config.LoadCommon(env, name, envPrefix, defaultAddr)
	level, levelErr := logging.ParseLevel(common.LogLevel)
	log := logging.New(os.Stdout, name, level)
	slog.SetDefault(log) // library log output (e.g. go-redis) goes through the same JSON handler

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt := &Runtime{Mux: http.NewServeMux(), Metrics: metrics.NewRegistry()}
	checks, closers, setupErr := setup(ctx, env, rt)
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}()
	if err := errors.Join(levelErr, env.Err(), setupErr); err != nil {
		log.Error("invalid configuration", "err", err)
		return 1
	}

	err := service.Run(ctx, service.Options{
		Name:            name,
		Addr:            common.HTTPAddr,
		ShutdownTimeout: common.ShutdownTimeout,
		Logger:          log,
		Checks:          checks,
		Metrics:         metrics.Handler(rt.Metrics),
		Routes:          rt.routesHandler(),
	})
	if err != nil {
		log.Error("service failed", "err", err)
		return 1
	}
	return 0
}

func (rt *Runtime) routesHandler() http.Handler {
	if !rt.routes {
		return nil
	}
	return rt.Mux
}
