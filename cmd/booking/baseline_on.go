//go:build unsafe_baseline

package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/booking/baseline"
	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

// setupBaseline serves the unsafe baseline only when the runtime flag is set.
// This file exists only in binaries built with -tags unsafe_baseline.
func setupBaseline(ctx context.Context, env *config.Env, rt *app.Runtime, pool *pgxpool.Pool) error {
	on, err := unsafebaseline.Enabled(env)
	if err != nil {
		return err
	}
	if !on {
		slog.Info("unsafe baseline compiled in but disabled", "flag", unsafebaseline.EnvFlag)
		return nil
	}
	slog.Warn("UNSAFE NAIVE BASELINE ENABLED: this route deliberately oversells; testing and benchmarking only (ADR-034)",
		"route", baseline.Route, "pool_max_conns", pool.Config().MaxConns)
	return baseline.Mount(ctx, pool, rt)
}
