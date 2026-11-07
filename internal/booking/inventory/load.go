package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInventoryExists means the event already has seats or GA buckets. Load
// never merges, replaces or silently duplicates inventory.
var ErrInventoryExists = errors.New("inventory: event already has inventory")

// Load is the PRODUCTION loader. It writes inv to booking_db in one
// transaction: seats as AVAILABLE with no hold, and buckets with
// remaining = capacity. It first applies the production rules
// (Inventory.Validate): bucket numbers 0..31, which the database cannot
// check, and exactly 32 buckets per GA section, so a SingleBucket benchmark
// inventory is refused. If two loads of one event race, the primary keys
// make the second one fail.
func Load(ctx context.Context, pool *pgxpool.Pool, inv Inventory) error {
	if err := inv.Validate(); err != nil {
		return fmt.Errorf("inventory: refusing to load invalid inventory: %w", err)
	}
	return load(ctx, pool, inv)
}

// LoadBenchmark is Load for benchmark and test layouts. It also accepts a
// SingleBucket GA section (T7's 32-vs-1 measurement, PDF p.35).
// BENCHMARK/TEST ONLY: service code must call Load.
func LoadBenchmark(ctx context.Context, pool *pgxpool.Pool, inv Inventory) error {
	if err := inv.ValidateBenchmark(); err != nil {
		return fmt.Errorf("inventory: refusing to load invalid benchmark inventory: %w", err)
	}
	return load(ctx, pool, inv)
}

func load(ctx context.Context, pool *pgxpool.Pool, inv Inventory) error {
	event, err := toUUID(inv.EventID)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM seats WHERE event_id = $1)
		OR EXISTS (SELECT 1 FROM ga_buckets WHERE event_id = $1)`, event).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %s", ErrInventoryExists, inv.EventID)
	}

	if len(inv.Seats) > 0 {
		rows := make([][]any, len(inv.Seats))
		for i, s := range inv.Seats {
			rows[i] = []any{event, s.SeatID, s.Section, s.RowLabel, int32(s.SeatNumber), s.PricePaise}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"seats"},
			[]string{"event_id", "seat_id", "section", "row_label", "seat_number", "price_paise"},
			pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("inventory: load seats: %w", err)
		}
	}
	if len(inv.Buckets) > 0 {
		rows := make([][]any, len(inv.Buckets))
		for i, b := range inv.Buckets {
			rows[i] = []any{event, b.Section, int16(b.Number), int32(b.Capacity), int32(b.Capacity)}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"ga_buckets"},
			[]string{"event_id", "section", "bucket", "capacity", "remaining"},
			pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("inventory: load GA buckets: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// VerifyLoaded checks that Postgres holds exactly the inventory that layout
// generates for a freshly loaded event: the same seat and bucket rows (no
// more, no fewer), every seat AVAILABLE with no hold, and every bucket full.
// The configured capacity therefore equals the stored capacity.
func VerifyLoaded(ctx context.Context, pool *pgxpool.Pool, layout Layout) error {
	want, err := Generate(layout)
	if err != nil {
		return err
	}
	event, err := toUUID(want.EventID)
	if err != nil {
		return err
	}
	var errs []error

	rows, err := pool.Query(ctx, `SELECT seat_id, section, row_label, seat_number, price_paise, status,
		hold_id IS NULL, hold_expires_at IS NULL FROM seats WHERE event_id = $1 ORDER BY seat_id`, event)
	if err != nil {
		return err
	}
	got := map[string]Seat{}
	for rows.Next() {
		var s Seat
		var status string
		var noHold, noExpiry bool
		if err := rows.Scan(&s.SeatID, &s.Section, &s.RowLabel, &s.SeatNumber, &s.PricePaise, &status, &noHold, &noExpiry); err != nil {
			rows.Close()
			return err
		}
		s.EventID = want.EventID
		got[s.SeatID] = s
		if status != "AVAILABLE" || !noHold || !noExpiry {
			errs = append(errs, fmt.Errorf("seat %s is not fresh (status %s)", s.SeatID, status))
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, w := range want.Seats {
		g, ok := got[w.SeatID]
		if !ok {
			errs = append(errs, fmt.Errorf("seat %s missing", w.SeatID))
			continue
		}
		if g != w {
			errs = append(errs, fmt.Errorf("seat %s stored as %+v, configured %+v", w.SeatID, g, w))
		}
		delete(got, w.SeatID)
	}
	for id := range got {
		errs = append(errs, fmt.Errorf("seat %s stored but not configured", id))
	}

	brows, err := pool.Query(ctx, `SELECT section, bucket, capacity, remaining FROM ga_buckets WHERE event_id = $1`, event)
	if err != nil {
		return err
	}
	type key struct {
		section string
		bucket  int
	}
	gotB := map[key]int{}
	for brows.Next() {
		var sec string
		var b, c, r int
		if err := brows.Scan(&sec, &b, &c, &r); err != nil {
			brows.Close()
			return err
		}
		gotB[key{sec, b}] = c
		if r != c {
			errs = append(errs, fmt.Errorf("bucket %s/%d is not full (%d of %d)", sec, b, r, c))
		}
	}
	if err := brows.Err(); err != nil {
		return err
	}
	for _, w := range want.Buckets {
		k := key{w.Section, w.Number}
		c, ok := gotB[k]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("bucket %s/%d missing", w.Section, w.Number))
		case c != w.Capacity:
			errs = append(errs, fmt.Errorf("bucket %s/%d capacity %d, configured %d", w.Section, w.Number, c, w.Capacity))
		}
		delete(gotB, k)
	}
	for k := range gotB {
		errs = append(errs, fmt.Errorf("bucket %s/%d stored but not configured", k.section, k.bucket))
	}
	return errors.Join(errs...)
}

func toUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return u, fmt.Errorf("inventory: event id %q: %w", s, err)
	}
	return u, nil
}
