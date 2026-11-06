// Command identity is the SurgeTix Identity service.
//
// Owns (PDF p.15-16): users, credentials, email verification, Google OAuth (PKCE),
// JWT signing keys and JWKS. Database: identity_db.
//
// Phase 1 provides only configuration, logging, dependency connectivity,
// health endpoints and graceful shutdown. Authentication arrives in phase 4.
package main

import (
	"context"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/postgres"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func main() {
	app.Main("identity", "IDENTITY", ":18081", func(ctx context.Context, env *config.Env) ([]service.Check, []func(), error) {
		var checks []service.Check
		var closers []func()
		pool, err := postgres.Open(ctx, env.Required("IDENTITY_DATABASE_URL"))
		if err != nil {
			return nil, closers, err
		}
		closers = append(closers, pool.Close)
		checks = append(checks, postgres.Check(pool))
		return checks, closers, nil
	})
}
