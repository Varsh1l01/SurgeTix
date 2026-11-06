// Package app is the common entry point for every SurgeTix binary: load
// configuration, build the logger, create the service's own dependency
// clients, serve health endpoints and shut down cleanly on SIGINT/SIGTERM.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/logging"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// Setup reads the service's own settings from env and creates its dependency
// clients. Clients must connect lazily so a missing dependency shows up in
// /readyz instead of preventing start-up. closers run in reverse order on exit,
// even when Setup returns an error.
type Setup func(ctx context.Context, env *config.Env) (checks []service.Check, closers []func(), err error)

// Main runs a service and exits the process with its status.
// envPrefix scopes the listen address variable, e.g. "BOOKING" → BOOKING_HTTP_ADDR.
func Main(name, envPrefix, defaultAddr string, setup Setup) {
	os.Exit(run(name, envPrefix, defaultAddr, setup))
}

func run(name, envPrefix, defaultAddr string, setup Setup) int {
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

	checks, closers, setupErr := setup(ctx, env)
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
	})
	if err != nil {
		log.Error("service failed", "err", err)
		return 1
	}
	return 0
}
