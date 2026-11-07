package holds

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

// reclaim is the partial reclaim of plan §8.3 (AMB-50 ✅), run inside the
// hold transaction after it has taken seats from the expired holds in old.
//
// Lock discipline (plan §10): this transaction already holds rank-2 seat
// locks, so every lock here (old holds, rank 1; their other seats, rank 2)
// is taken with SKIP LOCKED and never waits. Work skipped because of a lock
// stays visible to the expiry worker: the old hold remains ACTIVE or
// PAYMENT_PENDING with an expired expires_at (§10.4).
//
//   - Only seats the old hold still owns are released (hold_id = h AND
//     status = 'HELD'), so another hold's seat is never released, including
//     the ones this transaction just took (their hold_id is now ours).
//   - An old hold becomes EXPIRED only when it owns no HELD seat any more
//     (drained-only rule). A seat skipped because another transaction locks
//     it still shows hold_id = h, so h is never expired too early.
//   - Events: HoldExpired for each drained hold, InventoryReturned (source
//     "reclaimed") per section for the released seats.
func reclaim(ctx context.Context, tx pgx.Tx, eventID string, old []string) error {
	if len(old) == 0 {
		return nil
	}
	var locked []string
	if err := pgxCollectStrings(ctx, tx, &locked, `
		SELECT hold_id::text FROM holds
		 WHERE hold_id = ANY($1::uuid[]) AND status IN ('ACTIVE','PAYMENT_PENDING') AND expires_at < now()
		 ORDER BY hold_id
		   FOR UPDATE SKIP LOCKED`, old); err != nil {
		return fmt.Errorf("holds: reclaim: lock old holds: %w", err)
	}
	if len(locked) == 0 {
		return nil // every old hold is owned by a worker or another reclaimer
	}

	rows, err := tx.Query(ctx, `
		WITH rest AS (
		  SELECT event_id, seat_id FROM seats
		   WHERE hold_id = ANY($1::uuid[]) AND status = 'HELD'
		   ORDER BY event_id, seat_id
		     FOR UPDATE SKIP LOCKED)
		UPDATE seats s SET status = 'AVAILABLE', hold_id = NULL, hold_expires_at = NULL
		  FROM rest r
		 WHERE s.event_id = r.event_id AND s.seat_id = r.seat_id
		   AND s.hold_id = ANY($1::uuid[]) AND s.status = 'HELD'
		RETURNING s.event_id::text, s.seat_id, s.section`, locked)
	if err != nil {
		return fmt.Errorf("holds: reclaim: release seats: %w", err)
	}
	released := map[string]map[string]string{} // event -> seat -> section
	for rows.Next() {
		var ev, seat, sec string
		if err := rows.Scan(&ev, &seat, &sec); err != nil {
			rows.Close()
			return fmt.Errorf("holds: reclaim: release seats: %w", err)
		}
		if released[ev] == nil {
			released[ev] = map[string]string{}
		}
		released[ev][seat] = sec
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("holds: reclaim: release seats: %w", err)
	}

	// Drained-only EXPIRED: conditional on status (invariant 1) and on the
	// hold owning no HELD seat in the state this transaction sees.
	rows, err = tx.Query(ctx, `
		UPDATE holds h SET status = 'EXPIRED'
		 WHERE h.hold_id = ANY($1::uuid[]) AND h.status IN ('ACTIVE','PAYMENT_PENDING') AND h.expires_at < now()
		   AND NOT EXISTS (SELECT 1 FROM seats s WHERE s.hold_id = h.hold_id AND s.status = 'HELD')
		RETURNING h.hold_id::text, h.event_id::text`, locked)
	if err != nil {
		return fmt.Errorf("holds: reclaim: expire drained holds: %w", err)
	}
	type drainedHold struct{ id, event string }
	drained, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (drainedHold, error) {
		var d drainedHold
		return d, r.Scan(&d.id, &d.event)
	})
	if err != nil {
		return fmt.Errorf("holds: reclaim: expire drained holds: %w", err)
	}

	for _, d := range drained {
		if _, err := outbox.Insert(ctx, tx, outbox.Event{Type: outbox.HoldExpired, Key: d.id,
			Data: holdExpiredData{HoldID: d.id, EventID: d.event, Reason: "expired"}}); err != nil {
			return fmt.Errorf("holds: reclaim: outbox: %w", err)
		}
	}
	for ev, seats := range released {
		if ev != eventID {
			// Holds belong to one event, and old holds come from this
			// event's seats, so this cannot happen. Refuse rather than
			// write events for a state we did not expect.
			return fmt.Errorf("holds: reclaim: released seats of event %s while holding for %s", ev, eventID)
		}
		if err := writeInventoryReturned(ctx, tx, ev, "reclaimed", seats); err != nil {
			return fmt.Errorf("holds: reclaim: outbox: %w", err)
		}
	}
	return nil
}
