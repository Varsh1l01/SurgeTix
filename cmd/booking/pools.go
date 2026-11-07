package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IdleInTransactionTimeout is set on Booking's runtime pool only (owner
// decision 2026-11-07, plan review item 5). Postgres terminates a runtime
// session that sits idle inside an open transaction for longer than this,
// releasing every row lock it holds.
//
// Why 5 s: every legitimate Booking transaction finishes far inside the 1 s
// HoldSeats deadline (lock waits are capped by the 500 ms lock_timeout), and
// an expiry batch runs its statements back to back, so no healthy session is
// ever idle in a transaction for seconds. 5 s is also far below the 30 s
// idempotency lease: a stuck or abandoned session gives up its claim-row
// lock (rank 0) and seat locks long before a retry would take its claim
// over (plan §7, §10.4).
//
// Scope: a per-connection runtime parameter of the runtime pool, not a role
// or database setting. The migration runner, the readiness ping, the pool
// metrics and the frozen Phase 2 baseline keep using the plain pool, whose
// configuration is unchanged.
const IdleInTransactionTimeout = 5 * time.Second

const idleInTransactionParam = "idle_in_transaction_session_timeout"

// runtimePoolConfig parses dsn and adds the idle-in-transaction timeout.
func runtimePoolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx errors can echo the DSN; never log credentials.
		return nil, fmt.Errorf("booking: invalid connection string")
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams[idleInTransactionParam] = fmt.Sprintf("%dms", IdleInTransactionTimeout.Milliseconds())
	return cfg, nil
}

// openRuntimePool opens the pool the Phase 3 core runs on (the expiry
// worker now; the hold service from M2). Like postgres.Open, it does not
// connect eagerly.
func openRuntimePool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := runtimePoolConfig(dsn)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
