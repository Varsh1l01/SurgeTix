package holds

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

// HoldDuration is how long a hold lasts (PDF p.28: 10 minutes).
const HoldDuration = 10 * time.Minute

// LockTimeout bounds every lock wait in a hold transaction, so it finishes
// inside the 1 s HoldSeats deadline (PDF p.27; owner decision D1,
// 2026-11-06). A timeout is a non-deterministic failure: nothing is stored
// and the caller may retry with the same key (plan §4.1, §7.4).
const LockTimeout = 500 * time.Millisecond

// lockTimeout is the value applied; tests lengthen it to build forced lock
// schedules. Production code never changes it.
var lockTimeout = LockTimeout

// afterEffectsHook, if set, runs after every effect of a successful hold is
// written and before the idempotency completion. Tests use it to inject a
// failure at the latest point; it is nil in production.
var afterEffectsHook func(ctx context.Context, tx pgx.Tx) error

// SQLSTATEs a caller may want to recognise.
const (
	SQLStateLockTimeout = "55P03" // lock_not_available
	SQLStateDeadlock    = "40P01" // deadlock_detected: a bug under plan §10
)

// Outcome of HoldSeats when it returns no error.
type Outcome int

const (
	Held         Outcome = iota + 1 // 201: the seats are held
	Unavailable                     // 409 seats-unavailable (stored)
	UnknownSeats                    // 422 invalid-request (stored)
)

func (o Outcome) String() string {
	switch o {
	case Held:
		return "held"
	case Unavailable:
		return "unavailable"
	case UnknownSeats:
		return "unknown_seats"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// SeatsResult is a committed, deterministic result. Response is exactly what
// was stored in the idempotency record.
type SeatsResult struct {
	Outcome  Outcome
	HoldID   string // Held only
	Response idempotency.Response
}

type takenSeat struct {
	HeldSeat
	oldHold *string
}

// HoldSeats runs the chosen-seat hold transaction of plan §4.1 for a
// validated seats request, under the caller's idempotency claim.
//
// Lock order (plan §10): the claim row (rank 0, via Fence), then the
// requested seats in ascending seat_id (rank 2). Old holds and their other
// seats are only ever taken with SKIP LOCKED (reclaim, §8.3).
//
// It returns a SeatsResult when a deterministic outcome (201, 409, 422) was
// committed together with its idempotency record. Any error means the
// transaction rolled back and nothing was stored, including:
// idempotency.ErrNotOwner (the claim was lost), a lock timeout (55P03), a
// deadlock (40P01, a bug), cancellation, or a database failure.
func HoldSeats(ctx context.Context, pool *pgxpool.Pool, claim idempotency.Claim, req Request) (SeatsResult, error) {
	if req.Mode != ModeSeats || len(req.SeatIDs) < MinQuantity || len(req.SeatIDs) > MaxQuantity {
		return SeatsResult{}, errors.New("holds: HoldSeats needs a validated seat_ids request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SeatsResult{}, fmt.Errorf("holds: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }() // no-op after Commit

	res, err := holdSeatsTx(ctx, tx, claim, req)
	if err != nil {
		return SeatsResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: commit: %w", err)
	}
	return res, nil
}

func holdSeatsTx(ctx context.Context, tx pgx.Tx, claim idempotency.Claim, req Request) (SeatsResult, error) {
	// Set the lock timeout first: it takes no lock, and it then bounds the
	// fence's wait too. The new hold id comes from the database, like now().
	var holdID string
	if err := tx.QueryRow(ctx,
		`SELECT set_config('lock_timeout', $1, true), gen_random_uuid()::text`,
		fmt.Sprintf("%dms", lockTimeout.Milliseconds())).Scan(new(string), &holdID); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: start: %w", err)
	}
	// (0) Prove ownership and lock the claim row (rank 0).
	if err := idempotency.Fence(ctx, tx, claim); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: fence: %w", err)
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT hold_attempt`); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: savepoint: %w", err)
	}

	// (1) Take the requested seats, all or nothing, locking in seat_id
	// order (PDF p.31; rank 2 ascending). An expired HELD seat qualifies:
	// inline reclaim (PDF p.28, plan §8.2).
	ids := append([]string(nil), req.SeatIDs...)
	sort.Strings(ids)
	rows, err := tx.Query(ctx, `
		WITH target AS (
		  SELECT seat_id, hold_id AS old_hold FROM seats
		   WHERE event_id = $1 AND seat_id = ANY($2)
		     AND (status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < now()))
		   ORDER BY seat_id
		     FOR UPDATE)
		UPDATE seats s
		   SET status = 'HELD', hold_id = $3, hold_expires_at = now() + make_interval(secs => $4)
		  FROM target t
		 WHERE s.event_id = $1 AND s.seat_id = t.seat_id
		RETURNING s.seat_id, s.section, s.row_label, s.seat_number, s.price_paise, t.old_hold::text, s.hold_expires_at`,
		req.EventID, ids, holdID, HoldDuration.Seconds())
	if err != nil {
		return SeatsResult{}, fmt.Errorf("holds: take seats: %w", err)
	}
	var (
		taken     []takenSeat
		expiresAt time.Time
	)
	for rows.Next() {
		var s takenSeat
		if err := rows.Scan(&s.SeatID, &s.Section, &s.Row, &s.Number, &s.PricePaise, &s.oldHold, &expiresAt); err != nil {
			rows.Close()
			return SeatsResult{}, fmt.Errorf("holds: take seats: %w", err)
		}
		taken = append(taken, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: take seats: %w", err)
	}

	if len(taken) != len(ids) {
		return refuse(ctx, tx, claim, req.EventID, ids, taken)
	}
	return commitHold(ctx, tx, claim, req, holdID, expiresAt, taken)
}

// refuse undoes the partial take and stores the deterministic refusal: 422
// if any requested seat does not exist for the event, else 409 listing the
// seats that could not be taken.
func refuse(ctx context.Context, tx pgx.Tx, claim idempotency.Claim, eventID string, ids []string, taken []takenSeat) (SeatsResult, error) {
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT hold_attempt`); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: rollback to savepoint: %w", err)
	}
	var existing []string
	if err := pgxCollectStrings(ctx, tx, &existing,
		`SELECT seat_id FROM seats WHERE event_id = $1 AND seat_id = ANY($2)`, eventID, ids); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: read seats: %w", err)
	}
	exists := make(map[string]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}
	got := make(map[string]bool, len(taken))
	for _, s := range taken {
		got[s.SeatID] = true
	}
	var unknown, unavailable []string
	for _, id := range ids { // ids are sorted, so both lists are too
		switch {
		case !exists[id]:
			unknown = append(unknown, id)
		case !got[id]:
			unavailable = append(unavailable, id)
		}
	}
	var (
		resp    idempotency.Response
		outcome Outcome
		err     error
	)
	if len(unknown) > 0 {
		outcome = UnknownSeats
		resp, err = unknownSeatsResponse(unknown)
	} else {
		outcome = Unavailable
		resp, err = unavailableResponse(unavailable)
	}
	if err != nil {
		return SeatsResult{}, err
	}
	if err := idempotency.Complete(ctx, tx, claim, resp); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: complete: %w", err)
	}
	return SeatsResult{Outcome: outcome, Response: resp}, nil
}

func commitHold(ctx context.Context, tx pgx.Tx, claim idempotency.Claim, req Request, holdID string, expiresAt time.Time, taken []takenSeat) (SeatsResult, error) {
	sort.Slice(taken, func(i, j int) bool { return taken[i].SeatID < taken[j].SeatID })
	var (
		amount  int64
		seatIDs = make([]string, len(taken))
		section = make(map[string]string, len(taken))
		seats   = make([]HeldSeat, len(taken))
		oldSet  = map[string]bool{}
	)
	for i, s := range taken {
		amount += s.PricePaise
		seatIDs[i] = s.SeatID
		section[s.SeatID] = s.Section
		seats[i] = s.HeldSeat
		if s.oldHold != nil {
			oldSet[*s.oldHold] = true
		}
	}

	// (2) holds + hold_seats in one round trip. expires_at is the same
	// expression as the seat mirror, with the same now(): equal (inv. 9).
	b := &pgx.Batch{}
	b.Queue(`INSERT INTO holds (hold_id, event_id, user_id, kind, status, expires_at)
	         VALUES ($1, $2, $3, 'SEATS', 'ACTIVE', now() + make_interval(secs => $4))`,
		holdID, req.EventID, claim.UserID, HoldDuration.Seconds())
	b.Queue(`INSERT INTO hold_seats (hold_id, event_id, seat_id) SELECT $1, $2, unnest($3::text[])`,
		holdID, req.EventID, seatIDs)
	br := tx.SendBatch(ctx, b)
	for i := 0; i < b.Len(); i++ {
		tag, err := br.Exec()
		if err != nil {
			_ = br.Close()
			return SeatsResult{}, fmt.Errorf("holds: insert hold: %w", err)
		}
		if want := []int64{1, int64(len(seatIDs))}[i]; tag.RowsAffected() != want {
			_ = br.Close()
			return SeatsResult{}, fmt.Errorf("holds: insert hold: %d rows, want %d", tag.RowsAffected(), want)
		}
	}
	if err := br.Close(); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: insert hold: %w", err)
	}

	// (3) Partial reclaim of the expired holds we took seats from (§8.3).
	old := make([]string, 0, len(oldSet))
	for h := range oldSet {
		old = append(old, h)
	}
	sort.Strings(old)
	if err := reclaim(ctx, tx, req.EventID, old); err != nil {
		return SeatsResult{}, err
	}

	// (4) Outbox: HoldCreated on booking.holds, one InventoryChanged per
	// section on booking.inventory (PDF p.24, p.28; plan §9).
	exp := expiresAt.UTC().Format(outbox.OccurredAtLayout)
	if _, err := outbox.Insert(ctx, tx, outbox.Event{Type: outbox.HoldCreated, Key: holdID, Data: holdCreatedData{
		HoldID: holdID, EventID: req.EventID, UserID: claim.UserID, Kind: "SEATS",
		ExpiresAt: exp, AmountPaise: amount, Seats: seatIDs,
	}}); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: outbox: %w", err)
	}
	if err := writeInventoryChanged(ctx, tx, req.EventID, "HELD", section); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: outbox: %w", err)
	}

	if afterEffectsHook != nil {
		if err := afterEffectsHook(ctx, tx); err != nil {
			return SeatsResult{}, err
		}
	}

	// (5) Fenced completion with the 201 body, last before COMMIT.
	resp, err := createdResponse(Created{HoldID: holdID, Seats: seats, ExpiresAt: exp, Amount: amount})
	if err != nil {
		return SeatsResult{}, err
	}
	if err := idempotency.Complete(ctx, tx, claim, resp); err != nil {
		return SeatsResult{}, fmt.Errorf("holds: complete: %w", err)
	}
	return SeatsResult{Outcome: Held, HoldID: holdID, Response: resp}, nil
}

func pgxCollectStrings(ctx context.Context, tx pgx.Tx, out *[]string, sql string, args ...any) error {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	v, err := pgx.CollectRows(rows, pgx.RowTo[string])
	*out = v
	return err
}
