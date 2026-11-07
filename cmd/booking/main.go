// Command booking is the SurgeTix Booking service.
//
// Owns (PDF p.15-16): seat and GA inventory, holds, expiry, tickets, rotating QR
// secrets, transfers and gate check-in. Database: booking_db.
//
// Phase 1 provides configuration, logging, dependency connectivity, health
// endpoints and graceful shutdown. Phase 3 (M1-T12) adds the booking core:
// schema migrations at start-up (/readyz only after they complete) and the
// expiry worker. Its HTTP routes arrive in M2.
//
// Phase 2 adds the UNSAFE NAIVE BASELINE (ADR-034), present only in binaries
// built with -tags unsafe_baseline and served only when
// SURGETIX_ENABLE_UNSAFE_BASELINE=true. See baseline_on.go / baseline_off.go.
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/kafka"
	"github.com/Varsh1l01/SurgeTix/internal/platform/postgres"
	"github.com/Varsh1l01/SurgeTix/internal/platform/redisx"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.MainWithRuntime("booking", "BOOKING", ":18084", func(ctx context.Context, env *config.Env, rt *app.Runtime) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		pool, err := postgres.Open(ctx, env.Required("BOOKING_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, pool.Close)
		checks = append(checks, postgres.Check(pool))
		postgres.RegisterPoolMetrics(rt.Metrics, pool)
		rdb := redisx.New(env.Required("REDIS_ADDR"), env.String("REDIS_PASSWORD", ""))
		closers = append(closers, func() { _ = rdb.Close() })
		checks = append(checks, redisx.Check(rdb))
		kc, err := kafka.NewClient(env.List("KAFKA_BROKERS"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, kc.Close)
		checks = append(checks, kafka.Check(kc))
		// Phase 3 core: migrations on the plain pool (blocking, before the
		// server listens), then the expiry worker on the runtime pool, which
		// alone carries the 5 s idle-in-transaction timeout (pools.go). The
		// plain pool's configuration, metrics and baseline use are unchanged.
		runtimePool, err := openRuntimePool(ctx, env.Required("BOOKING_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, runtimePool.Close)
		checks = append(checks, service.Check{Name: "postgres_runtime", Fn: runtimePool.Ping})
		// stopCore runs before both pools close (closers run in reverse order).
		schema, stopCore, err := startCore(ctx, pool, runtimePool, rt.Metrics, slog.Default(),
			env.Duration("SHUTDOWN_TIMEOUT", 10*time.Second))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, stopCore)
		checks = append(checks, schema)
		if err := setupBaseline(ctx, env, rt, pool); err != nil {
			return nil, closers, err
		}
		return checks, closers, nil
	})
}
