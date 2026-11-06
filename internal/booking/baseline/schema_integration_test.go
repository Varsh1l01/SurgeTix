//go:build unsafe_baseline && integration

package baseline

import (
	"context"
	"testing"
)

func TestMigrateSeedsDeterministicEvent(t *testing.T) {
	s, pool := openStore(t, 4)
	ctx := context.Background()

	// Migrating twice is a no-op (goose tracks its own version table).
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var seats, rows, perRow, price int64
	var minPrice, maxPrice int64
	err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT row_label), max(seat_number), min(price_paise), max(price_paise)
		FROM baseline.seats WHERE event_id = $1`, SyntheticEventID).Scan(&seats, &rows, &perRow, &minPrice, &maxPrice)
	if err != nil {
		t.Fatal(err)
	}
	price = minPrice
	if seats != SeatCount || rows != 20 || perRow != 25 || minPrice != maxPrice || price != 100000 {
		t.Fatalf("seed = %d seats, %d rows, %d per row, price %d..%d; want 500, 20, 25, 100000", seats, rows, perRow, minPrice, maxPrice)
	}
	for _, n := range []int{1, 250, 500} {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM baseline.seats WHERE id = $1::uuid)`, SeatID(n)).Scan(&ok); err != nil || !ok {
			t.Errorf("seat %d (%s) missing: %v", n, SeatID(n), err)
		}
	}

	a, err := s.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.SeatsTotal != 500 || a.SeatsAvailable != 500 || a.Bookings != 0 || a.DoubleBookedSeats != 0 || a.StatusMismatches != 0 {
		t.Fatalf("fresh audit = %+v", a)
	}
}

// The baseline lives in its own schema and goose table. It must not create
// anything in public (where the real Booking schema goes in phase 3) other
// than its own version table.
func TestBaselineIsConfinedToItsSchema(t *testing.T) {
	_, pool := openStore(t, 4)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT table_schema || '.' || table_name FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		got = append(got, n)
	}
	want := []string{"baseline.booking_seats", "baseline.bookings", "baseline.events", "baseline.seats", "public." + VersionTable}
	if len(got) != len(want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tables = %v, want %v", got, want)
		}
	}
	// And no uniqueness on booking_seats.seat_id: that is the point.
	var unique int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'baseline' AND c.relname = 'booking_seats' AND i.indisunique
		  AND i.indnatts = 1`).Scan(&unique)
	if err != nil {
		t.Fatal(err)
	}
	if unique != 0 {
		t.Fatalf("booking_seats has %d single-column unique index(es); the baseline must not be protected", unique)
	}
}

func TestResetRestoresSeededState(t *testing.T) {
	s, _ := openStore(t, 4)
	ctx := context.Background()
	if _, err := s.Book(ctx, SyntheticEventID, "buyer-1", []string{SeatID(1), SeatID(2)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := s.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.SeatsAvailable != 500 || a.Bookings != 0 || a.BookedSeatRows != 0 {
		t.Fatalf("after reset = %+v", a)
	}
}
