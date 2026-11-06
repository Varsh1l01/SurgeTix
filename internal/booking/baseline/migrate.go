//go:build unsafe_baseline

// Package baseline is the Phase 2 UNSAFE NAIVE BASELINE for the Booking
// service (ADR-034). It deliberately oversells: a plain availability check,
// then an unconditional booking, with no lock, no conditional update, no
// uniqueness and no stricter isolation in between.
//
// It exists only to measure what SurgeTix's protections buy (T6, PDF p.34-36).
// It is compiled only with -tags unsafe_baseline and served only when
// SURGETIX_ENABLE_UNSAFE_BASELINE=true (internal/platform/unsafebaseline).
// It reads and writes only the "baseline" schema in booking_db, never the
// real Booking tables, and publishes nothing to Kafka.
package baseline

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

//go:embed sql/reset.sql
var resetSQL string

//go:embed sql/audit.sql
var auditSQL string

// VersionTable keeps the baseline's goose history apart from the real Booking
// migrations (phase 3), which use goose's default table.
const VersionTable = "baseline_goose_db_version"

// Migrate applies the baseline migrations (schema and deterministic seed).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub,
		goose.WithTableName(VersionTable),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("baseline migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("baseline migrations: %w", err)
	}
	return nil
}
