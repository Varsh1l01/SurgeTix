//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/booking/holds"
	"github.com/Varsh1l01/SurgeTix/internal/booking/inventory"
	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const testEvent = "5a4b3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d"

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// readyz serves /readyz with the given checks, as the Booking binary does.
func readyz(t *testing.T, checks ...service.Check) int {
	t.Helper()
	srv := service.New(service.Options{Name: "booking", Logger: quietLog(), Checks: checks})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return rec.Code
}

// TestStartCoreMigratesThenRunsWorker: on a fresh booking_db, start-up
// applies every migration, reports ready, and runs the expiry worker,
// which expires a due hold on its own. stop() ends the worker promptly.
func TestStartCoreMigratesThenRunsWorker(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	want, err := latestMigration()
	if err != nil || want != 4 {
		t.Fatalf("latest embedded migration %d %v, want 00004", want, err)
	}
	if code := readyz(t, schemaCheck(pool, want)); code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz before migrations: %d, want 503", code)
	}

	reg := prometheus.NewRegistry()
	check, stop, err := startCore(ctx, pool, runtime(t, pool), reg, quietLog(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()
	if code := readyz(t, check); code != http.StatusOK {
		t.Fatalf("/readyz after migrations: %d", code)
	}

	// A due hold, created the production way, is expired by the running
	// worker within a few of its 2 s ticks.
	inv, err := inventory.Generate(inventory.Layout{EventID: testEvent, Seats: []inventory.SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 5, PricePaise: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Load(ctx, pool, inv); err != nil {
		t.Fatal(err)
	}
	r, err := holds.NewService(pool).Hold(ctx, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "k1", testEvent, []byte(`{"seat_ids":["A-R01-01"]}`))
	if err != nil || r.Status != http.StatusCreated {
		t.Fatalf("hold: %v %d %s", err, r.Status, r.Body)
	}
	var c holds.Created
	_ = json.Unmarshal(r.Body, &c)
	if _, err := pool.Exec(ctx, `UPDATE holds SET expires_at = now() - interval '1 minute' WHERE hold_id = $1`, c.HoldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE seats s SET hold_expires_at = h.expires_at FROM holds h WHERE h.hold_id = $1 AND s.hold_id = h.hold_id`, c.HoldID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		var st string
		_ = pool.QueryRow(ctx, `SELECT status FROM holds WHERE hold_id = $1`, c.HoldID).Scan(&st)
		if st == "EXPIRED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the started worker did not expire the due hold")
		}
		time.Sleep(100 * time.Millisecond)
	}

	mfs, _ := reg.Gather()
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	if !names["booking_expiry_batches_total"] || !names["booking_expiry_lag_seconds"] {
		t.Fatalf("worker metrics not on the service registry: %v", names)
	}

	start := time.Now()
	stop()
	stopped = true
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("stop took %v", d)
	}
	rep, err := inventory.Audit(ctx, pool)
	if err != nil || !rep.Clean() {
		t.Fatalf("audit: %v\n%s", err, rep)
	}
}

// TestStartCoreWaitsForAnotherMigrator: while another replica holds the
// migration lock, start-up blocks (so the server never listens and /readyz
// cannot answer ready); once the lock is released it completes and is ready.
func TestStartCoreWaitsForAnotherMigrator(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	other, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	conn, err := other.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrations.LockID); err != nil {
		t.Fatal(err)
	}

	type result struct {
		check service.Check
		stop  func()
		err   error
	}
	out := make(chan result, 1)
	rt := runtime(t, pool)
	go func() {
		c, s, err := startCore(ctx, pool, rt, prometheus.NewRegistry(), quietLog(), 5*time.Second)
		out <- result{c, s, err}
	}()
	select {
	case r := <-out:
		t.Fatalf("start-up finished while another migrator held the lock: %v", r.err)
	case <-time.After(1500 * time.Millisecond):
	}
	want, _ := latestMigration()
	if code := readyz(t, schemaCheck(pool, want)); code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz while waiting: %d", code)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrations.LockID); err != nil {
		t.Fatal(err)
	}
	conn.Release()
	select {
	case r := <-out:
		if r.err != nil {
			t.Fatal(r.err)
		}
		defer r.stop()
		if code := readyz(t, r.check); code != http.StatusOK {
			t.Fatalf("/readyz after start-up: %d", code)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("start-up did not finish after the lock was released")
	}
}

// TestSchemaCheckReportsWrongVersion: a schema behind the binary is unready.
func TestSchemaCheckReportsWrongVersion(t *testing.T) {
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(context.Background(), pool, nil); err != nil {
		t.Fatal(err)
	}
	err := schemaCheck(pool, 5).Fn(context.Background())
	if err == nil || !strings.Contains(err.Error(), "want 5") {
		t.Fatalf("got %v", err)
	}
	if err := schemaCheck(pool, 4).Fn(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// runtime opens the Booking runtime pool (5 s idle-in-transaction timeout)
// on the same database as pool, exactly as main does.
func runtime(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	rp, err := openRuntimePool(context.Background(), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rp.Close)
	return rp
}

func showIdleTimeout(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var v string
	if err := p.QueryRow(context.Background(), `SHOW idle_in_transaction_session_timeout`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestIdleInTransactionTimeoutScope: the 5 s timeout is on runtime-pool
// sessions only. The plain pool (migrations, readiness, metrics, baseline)
// and the role keep the server default (0 = off).
func TestIdleInTransactionTimeoutScope(t *testing.T) {
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	rp := runtime(t, pool)
	if got := showIdleTimeout(t, rp); got != "5s" {
		t.Fatalf("runtime pool: %s, want 5s", got)
	}
	if got := showIdleTimeout(t, pool); got != "0" {
		t.Fatalf("plain pool: %s, want 0 (unchanged)", got)
	}
	var roleSetting int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_db_role_setting s
		JOIN pg_roles r ON r.oid = s.setrole WHERE r.rolname = current_user
		AND array_to_string(s.setconfig, ',') LIKE '%idle_in_transaction%'`).Scan(&roleSetting); err != nil {
		t.Fatal(err)
	}
	if roleSetting != 0 {
		t.Fatal("the timeout must not be a role setting (the migration runner shares the role)")
	}
	// The lock and statement settings of the core are unchanged.
	if lt := showSetting(t, rp, "lock_timeout"); lt != "0" {
		t.Fatalf("runtime pool lock_timeout %s: it is set per hold transaction (500 ms), not per session", lt)
	}
}

func showSetting(t *testing.T, p *pgxpool.Pool, name string) string {
	t.Helper()
	var v string
	if err := p.QueryRow(context.Background(), `SELECT current_setting($1)`, name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestAbandonedRuntimeTransactionIsTerminated: a runtime session left idle
// inside an open transaction, holding a row lock, is terminated by Postgres
// after about 5 s and its lock is released, so a waiter proceeds. The same
// idle transaction on the plain pool is not terminated.
func TestAbandonedRuntimeTransactionIsTerminated(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(ctx, pool, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE idle_probe (id int PRIMARY KEY); INSERT INTO idle_probe VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	rp := runtime(t, pool)

	// Abandoned: lock a row, then go idle without COMMIT/ROLLBACK.
	tx, err := rp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// If an assertion fails, end the transaction before the pools close
	// (cleanups run LIFO), so the waiter below is never left blocked.
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(ctx, `SELECT id FROM idle_probe WHERE id = 1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	// A waiter on the plain pool blocks on that row until the runtime
	// session is terminated.
	waited := make(chan time.Duration, 1)
	go func() {
		wtx, err := pool.Begin(ctx)
		if err != nil {
			waited <- -1
			return
		}
		defer wtx.Rollback(ctx)
		if _, err := wtx.Exec(ctx, `SELECT id FROM idle_probe WHERE id = 1 FOR UPDATE`); err != nil {
			waited <- -1
			return
		}
		waited <- time.Since(start)
	}()
	select {
	case d := <-waited:
		if d < 4*time.Second || d > 15*time.Second {
			t.Fatalf("waiter got the lock after %v, want about 5 s (the timeout)", d)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the abandoned runtime transaction was never terminated")
	}
	if _, err := tx.Exec(ctx, `SELECT 1`); err == nil {
		t.Fatal("the terminated runtime session still accepted a statement")
	}
	_ = tx.Rollback(ctx)
	if err := rp.Ping(ctx); err != nil {
		t.Fatalf("runtime pool unusable after a terminated session: %v", err)
	}

	// The plain pool is not affected: idle 6 s inside a transaction, alive.
	ptx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ptx.Rollback(ctx)
	if _, err := ptx.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(IdleInTransactionTimeout + time.Second)
	if _, err := ptx.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatalf("plain-pool transaction was terminated: %v", err)
	}
}
