// Command checkout is the SurgeTix Checkout service.
//
// Owns (PDF p.15-16): orders, checkout and refund sagas, cancellations, payments,
// FakePay webhooks and the nightly audit. Database: checkout_db.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Payments arrive in phase 6.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/kafka"
	"github.com/Varsh1l01/SurgeTix/internal/platform/postgres"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("checkout", "CHECKOUT", ":18085", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		pool, err := postgres.Open(ctx, env.Required("CHECKOUT_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, pool.Close)
		checks = append(checks, postgres.Check(pool))
		kc, err := kafka.NewClient(env.List("KAFKA_BROKERS"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, kc.Close)
		checks = append(checks, kafka.Check(kc))
		return checks, closers, nil
	})
}
