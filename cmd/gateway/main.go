// Command gateway is the SurgeTix Gateway service.
//
// Owns (PDF p.15-16): routing, JWT and access-token verification, per-user and
// per-IP rate limits, request IDs, the cacheable queue-status route.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Routing, auth and rate limiting arrive in phase 4.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/redisx"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("gateway", "GATEWAY", ":18080", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		rdb := redisx.New(env.Required("REDIS_ADDR"), env.String("REDIS_PASSWORD", ""))
		closers = append(closers, func() { _ = rdb.Close() })
		checks = append(checks, redisx.Check(rdb))
		return checks, closers, nil
	})
}
