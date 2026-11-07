// Package migrations holds the Booking service's real schema (booking_db,
// schema public) as goose SQL migrations, applied under the session-level
// advisory lock of internal/platform/migrate (ADR-036, AMB-51).
//
//	00001  seats, ga_buckets, holds, hold_seats, hold_ga_allocations   (PDF p.17-18)
//	00002  bookings, tickets, one_live_ticket_per_seat, ticket_transfers (PDF p.18-19)
//	00003  idempotency_keys, outbox, processed_events                   (PDF p.21)
//	00004  idempotency_keys.claim_generation, claimed_at                (approved deviation, ADR-036)
//
// Version history lives in goose's default table, goose_db_version. The
// Phase 2 baseline uses its own schema ("baseline") and its own table
// (baseline_goose_db_version), so the two never interfere.
package migrations

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/platform/migrate"
)

//go:embed *.sql
var files embed.FS

// FS returns the migration files (root contains NNNNN_name.sql).
func FS() embed.FS { return files }

// LockID is Booking's migration lock. Every Booking replica uses this value,
// so only one migrates at a time. Advisory locks are per database, so other
// services' ids cannot collide with it.
const LockID int64 = 0x53_54_42_4B_4D_49_47 // "STBKMIG": SurgeTix BooKing MIGrations

// Up applies all pending Booking migrations.
func Up(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) ([]migrate.Result, error) {
	return migrate.Up(ctx, pool, files, migrate.Options{LockID: LockID, Logger: logger})
}
