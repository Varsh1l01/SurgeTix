// Command booking is the SurgeTix Booking service.
//
// Owns (PDF p.15-16): seat and GA inventory, holds, expiry, tickets, rotating QR
// secrets, transfers and gate check-in. Database: booking_db.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Holds arrive in phase 3.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/kafka"
	"github.com/Varsh1l01/SurgeTix/internal/platform/postgres"
	"github.com/Varsh1l01/SurgeTix/internal/platform/redisx"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("booking", "BOOKING", ":18084", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		pool, err := postgres.Open(ctx, env.Required("BOOKING_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, pool.Close)
		checks = append(checks, postgres.Check(pool))
		rdb := redisx.New(env.Required("REDIS_ADDR"), env.String("REDIS_PASSWORD", ""))
		closers = append(closers, func() { _ = rdb.Close() })
		checks = append(checks, redisx.Check(rdb))
		kc, err := kafka.NewClient(env.List("KAFKA_BROKERS"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, kc.Close)
		checks = append(checks, kafka.Check(kc))
		return checks, closers, nil
	})
}
