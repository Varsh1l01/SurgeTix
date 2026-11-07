// Package migrate applies a service's goose SQL migrations under a
// session-level PostgreSQL advisory lock (ADR-036, AMB-51).
//
// Why a session lock, and why it must stay on one session: Booking runs with
// at least two replicas (PDF p.8). If two replicas start together, only one
// may migrate. A session-level advisory lock excludes other migrators only if
// the lock and every migration statement run on the SAME database session.
// goose's Provider takes one *sql.Conn for the whole run, locks it through the
// SessionLocker, and runs every SQL migration (transactional or not) and every
// version-table write on that connection. The integration tests prove this by
// recording pg_backend_pid() inside each migration.
//
// The locker here BLOCKS in Postgres (pg_advisory_lock) instead of polling
// pg_try_advisory_lock the way goose's built-in locker does. A second
// migrator therefore waits inside Postgres, visible in pg_locks as an
// ungranted advisory lock, until the first one finishes. The wait is bounded
// by the caller's context, never by a retry count.
//
// Only SQL migrations are supported. goose runs non-transactional Go
// migrations on the shared pool rather than the locked connection, so this
// package never registers Go migrations.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Options configures Up.
type Options struct {
	// LockID is the advisory lock key. Advisory locks are scoped to the
	// current database. Use one fixed id per service schema so that every
	// replica of the service contends on the same lock.
	LockID int64
	// VersionTable is goose's history table. Empty means goose's default,
	// "goose_db_version".
	VersionTable string
	// Logger receives goose's progress messages. Nil means slog.Default().
	Logger *slog.Logger
}

// Result describes one applied migration.
type Result struct {
	Version int64
	Path    string
}

// Up applies all pending migrations found in fsys (the root of fsys holds
// the NNNNN_name.sql files). It returns the migrations applied by this call,
// which is empty when another migrator already applied everything.
func Up(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, opts Options) ([]Result, error) {
	if pool == nil || fsys == nil {
		return nil, errors.New("migrate: pool and fsys are required")
	}
	if opts.LockID == 0 {
		return nil, errors.New("migrate: LockID is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	popts := []goose.ProviderOption{
		goose.WithSessionLocker(NewSessionLocker(opts.LockID)),
		goose.WithDisableGlobalRegistry(true), // SQL only: no Go migrations can sneak in
		goose.WithSlog(logger),
	}
	if opts.VersionTable != "" {
		popts = append(popts, goose.WithTableName(opts.VersionTable))
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, popts...)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, src := range p.ListSources() {
		if src.Type != goose.TypeSQL {
			return nil, fmt.Errorf("migrate: %s is not an SQL migration; only SQL migrations run on the locked session", src.Path)
		}
	}

	results, err := p.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	out := make([]Result, 0, len(results))
	for _, r := range results {
		out = append(out, Result{Version: r.Source.Version, Path: r.Source.Path})
	}
	return out, nil
}

// NewSessionLocker returns a goose SessionLocker that takes the session-level
// advisory lock with the blocking pg_advisory_lock, on the exact connection
// goose then uses for the migrations.
func NewSessionLocker(lockID int64) lock.SessionLocker {
	return &blockingSessionLocker{lockID: lockID}
}

type blockingSessionLocker struct{ lockID int64 }

// SessionLock waits in Postgres until the lock is free or ctx ends.
// Cancelling ctx cancels the waiting statement, so a stopped migrator never
// ends up holding the lock.
func (l *blockingSessionLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", l.lockID); err != nil {
		return fmt.Errorf("acquire migration lock %d: %w", l.lockID, err)
	}
	return nil
}

// SessionUnlock releases the lock on the same session. If the session is
// gone, Postgres has already released it, because session-level advisory
// locks end with the session.
func (l *blockingSessionLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	var released bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", l.lockID).Scan(&released); err != nil {
		return fmt.Errorf("release migration lock %d: %w", l.lockID, err)
	}
	if !released {
		return fmt.Errorf("release migration lock %d: this session did not hold it", l.lockID)
	}
	return nil
}
