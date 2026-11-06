// Command notifier is the SurgeTix Notifier service.
//
// Owns (PDF p.15-16): ticket and refund emails, SSE seat-map deltas and
// second-chance offers (a worker consuming Kafka).
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Consumers arrive in phase 6.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/kafka"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("notifier", "NOTIFIER", ":18086", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		kc, err := kafka.NewClient(env.List("KAFKA_BROKERS"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, kc.Close)
		checks = append(checks, kafka.Check(kc))
		return checks, closers, nil
	})
}
