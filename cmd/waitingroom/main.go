// Command waitingroom is the SurgeTix Waiting Room service.
//
// Owns (PDF p.15-16): proof-of-work, signed queue tickets, the admission controller,
// access tokens and the second-chance queue. State lives in Redis.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. The queue arrives in phase 5.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/kafka"
	"github.com/Varsh1l01/SurgeTix/internal/platform/redisx"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("waitingroom", "WAITINGROOM", ":18083", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
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
