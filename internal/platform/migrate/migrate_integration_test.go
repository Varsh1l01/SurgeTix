//go:build integration

package migrate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const testLockID int64 = 0x5375_7267_6554_0001 // arbitrary, test-only

var (
	ctr *tcpostgres.PostgresContainer
	dsn string
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	var err error
	ctr, err = tcpostgres.Run(ctx, "postgres:17.11",
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("it-admin"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err == nil {
		dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
	}
	cancel()
	code := 1
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
	} else {
		code = m.Run()
	}
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	os.Exit(code)
}

// freshDB creates an empty database so tests never share migration state.
func freshDB(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	// pgx's ConnConfig.ConnString() returns the original string, so switch
	// the database in the URL itself.
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// poolNamed opens a pool whose sessions are identifiable in pg_stat_activity.
func poolNamed(t *testing.T, connStr, appName string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = appName
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// waitFor polls an observed database condition. Reaching the deadline is a
// test FAILURE, not a pass: the tests never rely on sleeping for correctness.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// lockKeyPredicate matches our bigint advisory lock in pg_locks
// (classid = high 32 bits, objid = low 32 bits, objsubid = 1 for bigint keys).
const lockKeyPredicate = `locktype = 'advisory' AND objsubid = 1 AND ((classid::bigint << 32) | objid::bigint) = %d`

func lockHolder(t *testing.T, p *pgxpool.Pool) (pid int32, ok bool) {
	t.Helper()
	err := p.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT pid FROM pg_locks WHERE `+lockKeyPredicate+` AND granted`, testLockID)).Scan(&pid)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return pid, true
}

func pidOf(t *testing.T, p *pgxpool.Pool, appName string) (int32, bool) {
	t.Helper()
	var pid int32
	err := p.QueryRow(context.Background(),
		`SELECT pid FROM pg_stat_activity WHERE application_name = $1 AND datname = current_database() LIMIT 1`, appName).Scan(&pid)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return pid, true
}

// probeMigration records, from inside the migration, the session running it
// and the session holding the advisory lock at that moment.
func probeMigration(version int) string {
	return fmt.Sprintf(`INSERT INTO probe (version, statement_pid, lock_holder_pid)
SELECT %d, pg_backend_pid(), (SELECT pid FROM pg_locks WHERE `+lockKeyPredicate+` AND granted);`, version, testLockID)
}

func TestEveryMigrationRunsOnTheLockHoldingSession(t *testing.T) {
	db := freshDB(t)
	admin := poolNamed(t, db, "t1-admin")
	fsys := fstest.MapFS{
		"00001_probe.sql": {Data: []byte("-- +goose Up\nCREATE TABLE probe (version INT PRIMARY KEY, statement_pid INT NOT NULL, lock_holder_pid INT);\n" +
			probeMigration(1) + "\n-- +goose Down\nDROP TABLE probe;\n")},
		"00002_tx.sql": {Data: []byte("-- +goose Up\n" + probeMigration(2) + "\n-- +goose Down\nDELETE FROM probe WHERE version = 2;\n")},
		// goose runs NO TRANSACTION migrations outside a transaction; they must still use the locked session.
		"00003_notx.sql": {Data: []byte("-- +goose NO TRANSACTION\n-- +goose Up\n" + probeMigration(3) + "\n-- +goose Down\nDELETE FROM probe WHERE version = 3;\n")},
	}

	res, err := Up(context.Background(), poolNamed(t, db, "t1-migrator"), fsys, Options{LockID: testLockID})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("applied %d migrations, want 3: %+v", len(res), res)
	}

	rows, err := admin.Query(context.Background(), `SELECT version, statement_pid, lock_holder_pid FROM probe ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	var firstPid int32
	for rows.Next() {
		var v int
		var stmt int32
		var holder *int32
		if err := rows.Scan(&v, &stmt, &holder); err != nil {
			t.Fatal(err)
		}
		seen++
		if holder == nil {
			t.Fatalf("migration %d ran while NO session held the migration lock", v)
		}
		if *holder != stmt {
			t.Fatalf("migration %d ran on session %d but the lock was held by session %d", v, stmt, *holder)
		}
		if firstPid == 0 {
			firstPid = stmt
		} else if stmt != firstPid {
			t.Fatalf("migration %d ran on session %d, earlier ones on %d: not one session", v, stmt, firstPid)
		}
		t.Logf("migration %d: statement session %d, lock holder %d", v, stmt, *holder)
	}
	if seen != 3 {
		t.Fatalf("probe rows = %d, want 3", seen)
	}
	if _, held := lockHolder(t, admin); held {
		t.Fatal("migration lock still held after Up returned")
	}
}

func TestSecondMigratorWaitsForTheFirst(t *testing.T) {
	db := freshDB(t)
	ctx := context.Background()
	admin := poolNamed(t, db, "t1-admin")
	if _, err := admin.Exec(ctx, `CREATE TABLE gate (x INT)`); err != nil {
		t.Fatal(err)
	}
	// The test holds a lock that migration 1 needs, freezing migrator A
	// mid-migration while it holds the advisory lock.
	gate, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gate.Rollback(ctx) }()
	if _, err := gate.Exec(ctx, `LOCK TABLE gate IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}

	fsys := fstest.MapFS{
		"00001_wait_on_gate.sql": {Data: []byte("-- +goose Up\nCREATE TABLE applied (version INT PRIMARY KEY, pid INT NOT NULL);\n" +
			"INSERT INTO applied SELECT 1, pg_backend_pid() FROM (SELECT count(*) FROM gate) g;\n-- +goose Down\nDROP TABLE applied;\n")},
		"00002_second.sql": {Data: []byte("-- +goose Up\nINSERT INTO applied VALUES (2, pg_backend_pid());\n-- +goose Down\nDELETE FROM applied WHERE version = 2;\n")},
	}
	type outcome struct {
		res []Result
		err error
	}
	aDone, bDone := make(chan outcome, 1), make(chan outcome, 1)
	// Pools are created here, not inside the goroutines: poolNamed may call
	// t.Fatal, which is only allowed on the test goroutine.
	poolA, poolB := poolNamed(t, db, "migrator-a"), poolNamed(t, db, "migrator-b")
	go func() {
		r, err := Up(ctx, poolA, fsys, Options{LockID: testLockID})
		aDone <- outcome{r, err}
	}()

	// A holds the advisory lock and is blocked on the gate, on the same session.
	var aPid int32
	waitFor(t, "migrator A blocked on the gate while holding the migration lock", func() bool {
		var pid int32
		err := admin.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE application_name = 'migrator-a'
			AND wait_event_type = 'Lock' AND query ILIKE '%gate%'`).Scan(&pid)
		if err != nil {
			return false
		}
		holder, ok := lockHolder(t, admin)
		aPid = pid
		return ok && holder == pid
	})

	go func() {
		r, err := Up(ctx, poolB, fsys, Options{LockID: testLockID})
		bDone <- outcome{r, err}
	}()

	// B is waiting inside Postgres for the same advisory lock (ungranted).
	var bPid int32
	waitFor(t, "migrator B waiting on the migration lock", func() bool {
		pid, ok := pidOf(t, admin, "migrator-b")
		if !ok {
			return false
		}
		var waiting bool
		_ = admin.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE `+lockKeyPredicate+` AND NOT granted AND pid = $1)`, testLockID), pid).Scan(&waiting)
		bPid = pid
		return waiting
	})
	if aPid == bPid {
		t.Fatal("A and B share a session; the test is not checking anything")
	}
	select {
	case o := <-bDone:
		t.Fatalf("migrator B finished while A held the lock: %+v", o)
	case o := <-aDone:
		t.Fatalf("migrator A finished while blocked on the gate: %+v", o)
	default:
	}
	t.Logf("A (session %d) holds the lock and is blocked on the gate. B (session %d) waits on the lock", aPid, bPid)

	// Release the gate: A finishes everything, then B gets the lock and finds nothing to do.
	if err := gate.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a, b := <-aDone, <-bDone
	if a.err != nil || b.err != nil {
		t.Fatalf("A err=%v, B err=%v", a.err, b.err)
	}
	if len(a.res) != 2 || len(b.res) != 0 {
		t.Fatalf("A applied %d (want 2), B applied %d (want 0)", len(a.res), len(b.res))
	}

	var n, versions int
	if err := admin.QueryRow(ctx, `SELECT count(*), count(DISTINCT version) FROM applied`).Scan(&n, &versions); err != nil {
		t.Fatal(err)
	}
	var history int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id > 0`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if n != 2 || versions != 2 || history != 2 {
		t.Fatalf("applied rows=%d distinct=%d history=%d; each migration must apply exactly once", n, versions, history)
	}
	if _, held := lockHolder(t, admin); held {
		t.Fatal("migration lock still held after both migrators returned")
	}
}

func TestWaitingMigratorStopsWhenItsContextEnds(t *testing.T) {
	db := freshDB(t)
	ctx := context.Background()
	admin := poolNamed(t, db, "t1-admin")

	// Simulate another migrator by holding the advisory lock on a separate session.
	holder, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Always give the connection back, even when an assertion fails:
	// pgxpool.Close (test cleanup) blocks until every acquired connection is
	// released, so a leaked holder would hang the run instead of failing it.
	// Releasing the session also releases its advisory lock.
	defer holder.Release()
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_lock($1)`, testLockID); err != nil {
		t.Fatal(err)
	}

	fsys := fstest.MapFS{"00001_x.sql": {Data: []byte("-- +goose Up\nCREATE TABLE x (i INT);\n-- +goose Down\nDROP TABLE x;\n")}}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	poolC := poolNamed(t, db, "migrator-c")
	done := make(chan error, 1)
	go func() {
		_, err := Up(cctx, poolC, fsys, Options{LockID: testLockID})
		done <- err
	}()
	waitFor(t, "migrator C waiting on the lock", func() bool {
		pid, ok := pidOf(t, admin, "migrator-c")
		if !ok {
			return false
		}
		var waiting bool
		_ = admin.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE `+lockKeyPredicate+` AND NOT granted AND pid = $1)`, testLockID), pid).Scan(&waiting)
		return waiting
	})
	cancel()
	if err := <-done; err == nil {
		t.Fatal("a cancelled migrator must return an error")
	}
	// It never applied anything and left no waiting or held lock behind.
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'x')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("cancelled migrator applied a migration")
	}
	waitFor(t, "cancelled migrator's lock request gone", func() bool {
		var n int
		_ = admin.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM pg_locks WHERE `+lockKeyPredicate+` AND NOT granted`, testLockID)).Scan(&n)
		return n == 0
	})
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, testLockID); err != nil {
		t.Fatal(err)
	}
}

func TestUpIsRepeatableAndHonoursVersionTable(t *testing.T) {
	db := freshDB(t)
	ctx := context.Background()
	pool := poolNamed(t, db, "t1-repeat")
	fsys := fstest.MapFS{"00001_y.sql": {Data: []byte("-- +goose Up\nCREATE TABLE y (i INT);\n-- +goose Down\nDROP TABLE y;\n")}}
	opts := Options{LockID: testLockID, VersionTable: "custom_goose_version"}

	first, err := Up(ctx, pool, fsys, opts)
	if err != nil || len(first) != 1 || first[0].Version != 1 {
		t.Fatalf("first Up = %+v, %v", first, err)
	}
	second, err := Up(ctx, pool, fsys, opts)
	if err != nil || len(second) != 0 {
		t.Fatalf("second Up = %+v, %v (want nothing to apply)", second, err)
	}
	var defaultTable bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'goose_db_version')`).Scan(&defaultTable); err != nil {
		t.Fatal(err)
	}
	var custom int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM custom_goose_version WHERE version_id = 1`).Scan(&custom); err != nil {
		t.Fatal(err)
	}
	if defaultTable || custom != 1 {
		t.Fatalf("version table option not honoured: default exists=%v custom rows=%d", defaultTable, custom)
	}
}
