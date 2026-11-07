package holds

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

// ExpireResult summarises one expiry batch.
type ExpireResult struct {
	Candidates int // expired holds this batch locked (≤ limit)
	Released   int // seats returned to AVAILABLE
	Expired    int // holds marked EXPIRED (drained)
	Partial    int // locked holds left live: a seat was locked by someone else
}

// ExpireBatch is one expiry-worker batch (plan §8.1, PDF p.30), run inside
// the caller's transaction (M1-T11: SEATS holds; the GA step lands in M3).
//
//  1. Lock up to limit expired ACTIVE/PAYMENT_PENDING holds, oldest first
//     (expires_at, hold_id), SKIP LOCKED: holds another worker or an inline
//     reclaim owns are left for them.
//  2. Return the seats those holds still own (hold_id = h AND status =
//     'HELD'), SKIP LOCKED: a seat an in-flight hold transaction locks is
//     left, and the hold stays live (partial) for a later batch.
//  3. Mark EXPIRED only the drained holds: zero HELD seats and zero
//     unreleased GA allocations (drained-only rule, AMB-50).
//  4. Outbox: HoldExpired per drained hold, InventoryReturned (source
//     "expired") per event and section.
//
// It never waits on a lock (every lock is SKIP LOCKED), so it adds no edge
// to the wait-for graph (plan §10). It is the same ownership rule as the
// inline reclaim (§8.3), selected by age instead of by the seats a request
// took. Re-running it after a commit is a no-op for the work already done.
func ExpireBatch(ctx context.Context, tx pgx.Tx, limit int) (ExpireResult, error) {
	if limit < 1 {
		return ExpireResult{}, fmt.Errorf("holds: expire batch limit %d", limit)
	}
	var locked []string
	if err := pgxCollectStrings(ctx, tx, &locked, `
		SELECT hold_id::text FROM holds
		 WHERE status IN ('ACTIVE','PAYMENT_PENDING') AND expires_at < now() AND kind = 'SEATS'
		 ORDER BY expires_at, hold_id
		 LIMIT $1
		   FOR UPDATE SKIP LOCKED`, limit); err != nil {
		return ExpireResult{}, fmt.Errorf("holds: expire: lock holds: %w", err)
	}
	res := ExpireResult{Candidates: len(locked)}
	if len(locked) == 0 {
		return res, nil
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
		return ExpireResult{}, fmt.Errorf("holds: expire: release seats: %w", err)
	}
	released := map[string]map[string]string{} // event -> seat -> section
	for rows.Next() {
		var ev, seat, sec string
		if err := rows.Scan(&ev, &seat, &sec); err != nil {
			rows.Close()
			return ExpireResult{}, fmt.Errorf("holds: expire: release seats: %w", err)
		}
		if released[ev] == nil {
			released[ev] = map[string]string{}
		}
		released[ev][seat] = sec
		res.Released++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ExpireResult{}, fmt.Errorf("holds: expire: release seats: %w", err)
	}

	rows, err = tx.Query(ctx, `
		UPDATE holds h SET status = 'EXPIRED'
		 WHERE h.hold_id = ANY($1::uuid[]) AND h.status IN ('ACTIVE','PAYMENT_PENDING') AND h.expires_at < now()
		   AND NOT EXISTS (SELECT 1 FROM seats s WHERE s.hold_id = h.hold_id AND s.status = 'HELD')
		   AND NOT EXISTS (SELECT 1 FROM hold_ga_allocations a WHERE a.hold_id = h.hold_id AND a.released_at IS NULL)
		RETURNING h.hold_id::text, h.event_id::text`, locked)
	if err != nil {
		return ExpireResult{}, fmt.Errorf("holds: expire: expire drained holds: %w", err)
	}
	type drainedHold struct{ id, event string }
	drained, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (drainedHold, error) {
		var d drainedHold
		return d, r.Scan(&d.id, &d.event)
	})
	if err != nil {
		return ExpireResult{}, fmt.Errorf("holds: expire: expire drained holds: %w", err)
	}
	sort.Slice(drained, func(i, j int) bool { return drained[i].id < drained[j].id })
	res.Expired = len(drained)
	res.Partial = res.Candidates - res.Expired

	for _, d := range drained {
		if _, err := outbox.Insert(ctx, tx, outbox.Event{Type: outbox.HoldExpired, Key: d.id,
			Data: holdExpiredData{HoldID: d.id, EventID: d.event, Reason: "expired"}}); err != nil {
			return ExpireResult{}, fmt.Errorf("holds: expire: outbox: %w", err)
		}
	}
	events := make([]string, 0, len(released))
	for ev := range released {
		events = append(events, ev)
	}
	sort.Strings(events)
	for _, ev := range events {
		if err := writeInventoryReturned(ctx, tx, ev, "expired", released[ev]); err != nil {
			return ExpireResult{}, fmt.Errorf("holds: expire: outbox: %w", err)
		}
	}
	return res, nil
}
