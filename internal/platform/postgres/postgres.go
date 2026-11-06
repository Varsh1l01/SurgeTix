// Package postgres opens a connection pool to the calling service's own database.
//
// Each service owns exactly one database and connects with its own role
// (ADR-008: no service reads another's tables). The DSN therefore always comes
// from a service-scoped variable such as BOOKING_DATABASE_URL, never a shared one.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// Open parses dsn and returns a pool. It does not connect eagerly: a service
// starts even if Postgres is not up yet, and /readyz reports the outage.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx errors can echo the DSN; never log credentials.
		return nil, fmt.Errorf("postgres: invalid connection string")
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Check returns a readiness probe that pings the pool.
func Check(pool *pgxpool.Pool) service.Check {
	return service.Check{Name: "postgres", Fn: pool.Ping}
}
