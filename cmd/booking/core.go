package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/booking/expiry"
	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// startCore brings up Booking's Phase 3 core (plan M1-T12):
//
//  1. Apply the schema migrations under the session advisory lock (T1, AMB-51).
//     This blocks until done, and setup runs before the HTTP server starts
//     listening, so /readyz can only ever answer after the migrations have
//     completed. A replica that waits behind another's migration stays
//     unready meanwhile.
//  2. Start the expiry worker (T11) on its own goroutine.
//
// Migrations run on migratePool (the plain pool, no idle-in-transaction
// timeout). The worker runs on runtimePool (see IdleInTransactionTimeout).
//
// It returns a readiness check that the schema is at the embedded version,
// and stop, which ends the worker and waits for it (an in-flight batch gets
// up to shutdownTimeout to commit). No public routes: they arrive in M2.
func startCore(ctx context.Context, migratePool, runtimePool *pgxpool.Pool, reg prometheus.Registerer, log *slog.Logger, shutdownTimeout time.Duration) (service.Check, func(), error) {
	want, err := latestMigration()
	if err != nil {
		return service.Check{}, nil, err
	}
	applied, err := migrations.Up(ctx, migratePool, log)
	if err != nil {
		return service.Check{}, nil, fmt.Errorf("booking: migrations: %w", err)
	}
	log.Info("booking schema ready", "version", want, "applied_now", len(applied))

	w, err := expiry.New(runtimePool, reg, expiry.Config{ShutdownTimeout: shutdownTimeout, Logger: log})
	if err != nil {
		return service.Check{}, nil, err
	}
	wctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := w.Run(wctx); err != nil {
			log.Error("expiry worker stopped", "err", err)
		}
	}()
	stop := func() {
		cancel()
		<-done
	}
	return schemaCheck(runtimePool, want), stop, nil
}

// schemaCheck is ready only when the schema is at the embedded version.
func schemaCheck(pool *pgxpool.Pool, want int64) service.Check {
	return service.Check{Name: "booking_schema", Fn: func(ctx context.Context) error {
		var got int64
		err := pool.QueryRow(ctx, `SELECT COALESCE(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&got)
		if err != nil {
			return fmt.Errorf("schema version: %w", err)
		}
		if got != want {
			return fmt.Errorf("schema at version %d, want %d", got, want)
		}
		return nil
	}}
}

// latestMigration is the highest NNNNN_ prefix among the embedded files.
func latestMigration() (int64, error) {
	entries, err := fs.ReadDir(migrations.FS(), ".")
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("booking: migration file %s: %w", e.Name(), err)
		}
		latest = max(latest, v)
	}
	if latest == 0 {
		return 0, fmt.Errorf("booking: no embedded migrations")
	}
	return latest, nil
}
