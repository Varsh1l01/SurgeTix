// Command catalog is the SurgeTix Catalog service.
//
// Owns (PDF p.15-16): events, venues, seat layouts, sale and refund policy config,
// seat-map snapshots in Redis. Database: catalog_db.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Events and seat maps arrive in phase 4.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/postgres"
	"github.com/Varsh1l01/SurgeTix/internal/platform/redisx"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("catalog", "CATALOG", ":18082", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		pool, err := postgres.Open(ctx, env.Required("CATALOG_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, pool.Close)
		checks = append(checks, postgres.Check(pool))
		rdb := redisx.New(env.Required("REDIS_ADDR"), env.String("REDIS_PASSWORD", ""))
		closers = append(closers, func() { _ = rdb.Close() })
		checks = append(checks, redisx.Check(rdb))
		return checks, closers, nil
	})
}
