//go:build integration

package inventory

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

// migratedDB is a fresh booking_db (real schema, T2 migrations) as booking_svc.
func migratedDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(context.Background(), pool, nil); err != nil {
		t.Fatal(err)
	}
	return pool
}

// mixedLayout is a production layout: seated sections plus GA sections with
// exactly 32 buckets each.
func mixedLayout(event string) Layout {
	return Layout{EventID: event,
		Seats: []SeatSection{{Name: "A", Rows: 20, SeatsPerRow: 25, PricePaise: 100000}, {Name: "B", Rows: 2, SeatsPerRow: 150, PricePaise: 0}},
		GA: []GASection{
			{Name: "GA", Capacity: 2001},
			{Name: "FLOOR", Capacity: 96, Mode: ExplicitBuckets, Capacities: SplitEven(96)},
			{Name: "EMPTY", Capacity: 0},
		}}
}

// benchmarkLayout adds the T7 one-bucket section (benchmark/test only).
func benchmarkLayout(event string) Layout {
	l := mixedLayout(event)
	l.GA = append(l.GA, GASection{Name: "SOLO", Capacity: 2000, Mode: SingleBucket})
	return l
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLoadMatchesConfiguredCapacityExactly(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	l := mixedLayout(ev)
	inv, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	if err := Load(ctx, pool, inv); err != nil { // every row passes the schema's CHECKs
		t.Fatal(err)
	}
	if err := VerifyLoaded(ctx, pool, l); err != nil {
		t.Fatalf("stored inventory differs from configuration: %v", err)
	}
	seats, ga := l.Capacity()
	if got := count(t, pool, `SELECT count(*) FROM seats WHERE event_id = $1 AND status = 'AVAILABLE' AND hold_id IS NULL`, ev); got != seats {
		t.Fatalf("seats in Postgres = %d, configured %d", got, seats)
	}
	if got := count(t, pool, `SELECT coalesce(sum(capacity), 0) FROM ga_buckets WHERE event_id = $1`, ev); got != ga {
		t.Fatalf("GA capacity in Postgres = %d, configured %d", got, ga)
	}
	if got := count(t, pool, `SELECT count(*) FROM ga_buckets WHERE event_id = $1 AND remaining <> capacity`, ev); got != 0 {
		t.Fatalf("%d buckets not full after load", got)
	}
	// Exactly 32 buckets numbered 0..31 in every production GA section.
	for sec, want := range map[string]int{"GA": 32, "FLOOR": 32, "EMPTY": 32} {
		n := count(t, pool, `SELECT count(*) FROM ga_buckets WHERE event_id = $1 AND section = $2`, ev, sec)
		lo := count(t, pool, `SELECT min(bucket) FROM ga_buckets WHERE event_id = $1 AND section = $2`, ev, sec)
		hi := count(t, pool, `SELECT max(bucket) FROM ga_buckets WHERE event_id = $1 AND section = $2`, ev, sec)
		if n != want || lo != 0 || hi != want-1 {
			t.Fatalf("section %s: %d buckets numbered %d..%d, want %d numbered 0..%d", sec, n, lo, hi, want, want-1)
		}
	}
}

// best-available orders by (row_label, seat_number) (PDF p.28). The stored
// labels must give the same order as the generator's (row, seat) order.
func TestStoredOrderMatchesRowSeatOrder(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	l := mixedLayout(ev)
	inv, _ := Generate(l)
	if err := Load(ctx, pool, inv); err != nil {
		t.Fatal(err)
	}
	for _, sec := range []string{"A", "B"} {
		rows, err := pool.Query(ctx, `SELECT seat_id FROM seats WHERE event_id = $1 AND section = $2 ORDER BY row_label, seat_number`, ev, sec)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			got = append(got, id)
		}
		rows.Close()
		var want []string
		for _, s := range inv.Seats {
			if s.Section == sec {
				want = append(want, s.SeatID)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("section %s: database order differs from generated order", sec)
		}
	}
}

func TestRegenerationIsIdenticalAndLoadNeverDuplicates(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	l := ComparisonLayout(ev)
	first, _ := Generate(l)
	if err := Load(ctx, pool, first); err != nil {
		t.Fatal(err)
	}
	again, _ := Generate(l) // deterministic regeneration
	err := Load(ctx, pool, again)
	if !errors.Is(err, ErrInventoryExists) {
		t.Fatalf("second load: want ErrInventoryExists, got %v", err)
	}
	if err := VerifyLoaded(ctx, pool, l); err != nil {
		t.Fatalf("a refused second load must leave the first intact: %v", err)
	}
	if got := count(t, pool, `SELECT count(*) FROM seats WHERE event_id = $1`, ev); got != 500 {
		t.Fatalf("seats = %d, want 500 (no duplication)", got)
	}
}

func TestConcurrentLoadsOfOneEventLoadOnce(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	inv, _ := Generate(mixedLayout(ev))
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = Load(ctx, pool, inv) }()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d concurrent loads succeeded, want exactly 1: %v", ok, errs)
	}
	if err := VerifyLoaded(ctx, pool, mixedLayout(ev)); err != nil {
		t.Fatal(err)
	}
}

// The schema has no CHECK on ga_buckets.bucket (PDF DDL kept exact). This
// test shows Postgres itself would accept bucket 32, which is why Load
// validates. It then shows Load refuses it and writes nothing.
func TestBucketRangeIsEnforcedByLoadNotTheSchema(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO ga_buckets VALUES ($1, 'RAW', 32, 1, 1), ($1, 'RAW', -1, 1, 1)`, ev); err != nil {
		t.Fatalf("expected the PDF schema to accept out-of-range buckets (no CHECK), got %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ga_buckets WHERE event_id = $1`, ev); err != nil {
		t.Fatal(err)
	}

	other := "5e1f0000-0000-4000-8000-0000000000e3"
	for _, bad := range []int{-1, 32} {
		inv, _ := Generate(Layout{EventID: other, GA: []GASection{{Name: "GA", Capacity: 64}}})
		inv.Buckets[0].Number = bad // tamper after generation
		if err := Load(ctx, pool, inv); err == nil {
			t.Fatalf("Load accepted bucket %d", bad)
		}
		if got := count(t, pool, `SELECT count(*) FROM ga_buckets WHERE event_id = $1`, other); got != 0 {
			t.Fatalf("refused load of bucket %d still wrote %d rows", bad, got)
		}
	}
}

func TestVerifyLoadedDetectsDrift(t *testing.T) {
	ctx := context.Background()
	tamper := map[string]string{
		"missing seat":     `DELETE FROM seats WHERE event_id = $1 AND seat_id = 'A-R10-10'`,
		"extra seat":       `INSERT INTO seats (event_id, seat_id, section, row_label, seat_number, price_paise) VALUES ($1, 'A-R21-01', 'A', 'R21', 1, 100000)`,
		"changed price":    `UPDATE seats SET price_paise = 1 WHERE event_id = $1 AND seat_id = 'A-R01-01'`,
		"held seat":        `UPDATE seats SET status = 'HELD', hold_id = gen_random_uuid(), hold_expires_at = now() WHERE event_id = $1 AND seat_id = 'A-R01-02'`,
		"missing bucket":   `DELETE FROM ga_buckets WHERE event_id = $1 AND section = 'GA' AND bucket = 31`,
		"extra bucket":     `INSERT INTO ga_buckets VALUES ($1, 'NEW', 0, 5, 5)`,
		"changed capacity": `UPDATE ga_buckets SET capacity = capacity + 1, remaining = remaining + 1 WHERE event_id = $1 AND section = 'GA' AND bucket = 0`,
		"partly sold":      `UPDATE ga_buckets SET remaining = remaining - 1 WHERE event_id = $1 AND section = 'GA' AND bucket = 3`,
	}
	for name, sql := range tamper {
		t.Run(name, func(t *testing.T) {
			pool := migratedDB(t)
			inv, _ := Generate(mixedLayout(ev))
			if err := Load(ctx, pool, inv); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, sql, ev); err != nil {
				t.Fatal(err)
			}
			if err := VerifyLoaded(ctx, pool, mixedLayout(ev)); err == nil {
				t.Fatal("drift not detected")
			}
		})
	}
}

// The production loader rejects a SingleBucket inventory against a real
// database and writes nothing. The benchmark loader accepts it, and the
// result matches the layout exactly.
func TestProductionLoadRejectsSingleBucket(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	l := benchmarkLayout(ev)
	inv, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	err = Load(ctx, pool, inv)
	if err == nil || !strings.Contains(err.Error(), "benchmark-only") {
		t.Fatalf("production Load must refuse a SingleBucket inventory, got %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM seats WHERE event_id = $1`, ev) + count(t, pool, `SELECT count(*) FROM ga_buckets WHERE event_id = $1`, ev); n != 0 {
		t.Fatalf("refused production load still wrote %d rows", n)
	}
	if err := LoadBenchmark(ctx, pool, inv); err != nil {
		t.Fatalf("benchmark load: %v", err)
	}
	if err := VerifyLoaded(ctx, pool, l); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM ga_buckets WHERE event_id = $1 AND section = 'SOLO'`, ev); n != 1 {
		t.Fatalf("SOLO buckets = %d, want 1", n)
	}
}
