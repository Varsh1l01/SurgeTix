//go:build unsafe_baseline

package baseline

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyntheticEventID is the seeded event (migrations/00002_baseline_seed.sql).
const SyntheticEventID = "5e1f0000-0000-4000-8000-000000000001"

// SeatCount is the number of seeded seats.
const SeatCount = 500

// SeatID returns the deterministic id of seeded seat n (1..SeatCount).
func SeatID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

var (
	// ErrUnavailable means the CHECK saw at least one seat that is not AVAILABLE.
	ErrUnavailable = errors.New("baseline: seats not available")
	// ErrUnknownSeats means a requested seat does not exist in the event.
	ErrUnknownSeats = errors.New("baseline: unknown seats for this event")
)

// Seat is one booked seat as returned to the client.
type Seat struct {
	ID         string `json:"seat_id"`
	Section    string `json:"section"`
	Row        string `json:"row"`
	Number     int    `json:"number"`
	PricePaise int64  `json:"price_paise"`
}

// Booking is the result of a successful (possibly oversold) booking.
type Booking struct {
	ID          string
	Seats       []Seat
	AmountPaise int64
}

// UnavailableError lists the seats the CHECK found taken.
type UnavailableError struct{ SeatIDs []string }

func (e *UnavailableError) Error() string { return ErrUnavailable.Error() }
func (e *UnavailableError) Unwrap() error { return ErrUnavailable }

// Checkpoint runs between CHECK and BOOK inside the booking transaction.
// It exists only so tests can force every buyer past the availability check
// before any of them books (a barrier). It is nil outside tests, which adds
// no delay to the request path.
type Checkpoint func(ctx context.Context, tx pgx.Tx)

// Store is the naive check-then-book data access.
type Store struct {
	pool       *pgxpool.Pool
	afterCheck Checkpoint
}

// NewStore returns a Store over the Booking service's own pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// WithCheckpoint returns a copy of s that runs cp between CHECK and BOOK.
// Test use only.
func (s *Store) WithCheckpoint(cp Checkpoint) *Store {
	return &Store{pool: s.pool, afterCheck: cp}
}

// Isolation is set explicitly so a changed server default
// (default_transaction_isolation) cannot silently protect the baseline.
// Under REPEATABLE READ or SERIALIZABLE, concurrent writers of the same seat
// would fail with SQLSTATE 40001 instead of both succeeding.
var txOptions = pgx.TxOptions{IsoLevel: pgx.ReadCommitted}

const (
	// CHECK: a plain read. No FOR UPDATE / FOR SHARE / SKIP LOCKED, no advisory lock.
	checkSQL = `SELECT id::text, section, row_label, seat_number, price_paise, status
		FROM baseline.seats
		WHERE event_id = $1 AND id = ANY($2::uuid[])`

	// BOOK: deliberately unconditional. There is no "AND status = 'AVAILABLE'"
	// and the rows-affected count is ignored. That predicate and check are
	// exactly the protection SurgeTix adds (CLAUDE.md invariant 1).
	// Postgres still row-locks each updated seat, as every UPDATE does. A
	// second writer waits for the first to commit, then (READ COMMITTED)
	// re-evaluates only "id = ANY(...)", which still matches, and proceeds.
	markBookedSQL = `UPDATE baseline.seats SET status = 'BOOKED', booked_at = now()
		WHERE id = ANY($1::uuid[])`

	insertBookingSQL = `INSERT INTO baseline.bookings (event_id, buyer_id, amount_paise)
		VALUES ($1, $2, $3) RETURNING id::text`

	insertSeatsSQL = `INSERT INTO baseline.booking_seats (booking_id, seat_id)
		SELECT $1::uuid, unnest($2::uuid[])`
)

// Book runs the naive check-then-book. seatIDs must be distinct, valid UUIDs
// (the HTTP layer validates them).
func (s *Store) Book(ctx context.Context, eventID, buyerID string, seatIDs []string) (Booking, error) {
	tx, err := s.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return Booking{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	// 1. CHECK
	rows, err := tx.Query(ctx, checkSQL, eventID, seatIDs)
	if err != nil {
		return Booking{}, err
	}
	var seats []Seat
	var taken []string
	var amount int64
	for rows.Next() {
		var st Seat
		var status string
		if err := rows.Scan(&st.ID, &st.Section, &st.Row, &st.Number, &st.PricePaise, &status); err != nil {
			rows.Close()
			return Booking{}, err
		}
		if status != "AVAILABLE" {
			taken = append(taken, st.ID)
		}
		seats = append(seats, st)
		amount += st.PricePaise
	}
	if err := rows.Err(); err != nil {
		return Booking{}, err
	}
	if len(seats) != len(seatIDs) {
		return Booking{}, ErrUnknownSeats
	}
	if len(taken) > 0 {
		return Booking{}, &UnavailableError{SeatIDs: taken}
	}

	// The race window: the seats were AVAILABLE when read, but nothing stops
	// another transaction booking them before the statements below run.
	if s.afterCheck != nil {
		s.afterCheck(ctx, tx)
	}

	// 2. BOOK, based on the (possibly stale) result of the CHECK.
	if _, err := tx.Exec(ctx, markBookedSQL, seatIDs); err != nil {
		return Booking{}, err
	}
	var id string
	if err := tx.QueryRow(ctx, insertBookingSQL, eventID, buyerID, amount).Scan(&id); err != nil {
		return Booking{}, err
	}
	if _, err := tx.Exec(ctx, insertSeatsSQL, id, seatIDs); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Booking{}, err
	}
	return Booking{ID: id, Seats: seats, AmountPaise: amount}, nil
}

// AuditResult is one row of sql/audit.sql.
type AuditResult struct {
	SeatsTotal         int64 `json:"seats_total"`
	SeatsBooked        int64 `json:"seats_booked"`
	SeatsAvailable     int64 `json:"seats_available"`
	Bookings           int64 `json:"bookings"`
	BookedSeatRows     int64 `json:"booked_seat_rows"`
	DistinctSeatsSold  int64 `json:"distinct_seats_sold"`
	DoubleBookedSeats  int64 `json:"double_booked_seats"`
	ExcessSeatBookings int64 `json:"excess_seat_bookings"`
	MaxBookingsPerSeat int64 `json:"max_bookings_per_seat"`
	StatusMismatches   int64 `json:"status_mismatches"`
}

// Audit reconciles inventory with bookings.
func (s *Store) Audit(ctx context.Context) (AuditResult, error) {
	var a AuditResult
	err := s.pool.QueryRow(ctx, auditSQL).Scan(&a.SeatsTotal, &a.SeatsBooked, &a.SeatsAvailable,
		&a.Bookings, &a.BookedSeatRows, &a.DistinctSeatsSold, &a.DoubleBookedSeats,
		&a.ExcessSeatBookings, &a.MaxBookingsPerSeat, &a.StatusMismatches)
	return a, err
}

// Reset restores the seeded state: every seat AVAILABLE, no bookings.
func (s *Store) Reset(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, resetSQL)
	return err
}
