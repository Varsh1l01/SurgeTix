//go:build integration

package holds

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

func returnedSeats(t *testing.T, evs []event) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, ev := range eventsOf(evs, outbox.InventoryReturned) {
		var d inventoryReturnedData
		_ = json.Unmarshal(ev.Data, &d)
		if d.Source != "reclaimed" || ev.key != eventID+":"+d.Section || d.EventID != eventID {
			t.Fatalf("InventoryReturned %+v key %s", d, ev.key)
		}
		for _, id := range d.Seats {
			if _, dup := out[id]; dup {
				t.Fatalf("seat %s returned twice", id)
			}
			out[id] = d.Section
		}
	}
	return out
}

func expiredHolds(t *testing.T, evs []event) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, ev := range eventsOf(evs, outbox.HoldExpired) {
		var d holdExpiredData
		_ = json.Unmarshal(ev.Data, &d)
		if d.HoldID != ev.key || d.EventID != eventID || d.Reason != "expired" {
			t.Fatalf("HoldExpired %+v key %s", d, ev.key)
		}
		if out[d.HoldID] {
			t.Fatalf("hold %s expired twice", d.HoldID)
		}
		out[d.HoldID] = true
	}
	return out
}

// TestInlineReclaimOfExpiredSeat (plan §8.2): an expired HELD seat is taken
// like an AVAILABLE one; an unexpired one is not.
func TestInlineReclaimOfExpiredSeat(t *testing.T) {
	e := newEnv(t)
	live := e.mustHold(userA, "A-R01-05")
	if r := e.hold(userB, "A-R01-05"); r.Outcome != Unavailable {
		t.Fatalf("unexpired seat: %v", r.Outcome)
	}
	e.expire(live)
	got := e.mustHold(userB, "A-R01-05")
	e.expectSeat("A-R01-05", "HELD", got)
	if h := e.holdRow(live); h.status != "EXPIRED" {
		t.Fatalf("old hold %s", h.status)
	}
	e.audit()
}

// P1: reclaiming one of three seats releases the other two and expires the
// old hold, with HoldExpired and InventoryReturned events.
func TestP1ReclaimOneOfThree(t *testing.T) {
	e := newEnv(t)
	old := e.mustHold(userA, "A-R01-01", "A-R01-02", "B-R01-01")
	e.expire(old)
	before := len(e.events())
	nw := e.mustHold(userB, "A-R01-01")

	e.expectSeat("A-R01-01", "HELD", nw)
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	e.expectSeat("B-R01-01", "AVAILABLE", "")
	if e.seat("A-R01-02").expiry != nil {
		t.Fatal("released seat kept its mirror")
	}
	if h := e.holdRow(old); h.status != "EXPIRED" {
		t.Fatalf("old hold %s, want EXPIRED (drained)", h.status)
	}
	evs := e.events()[before:]
	if exp := expiredHolds(t, evs); len(exp) != 1 || !exp[old] {
		t.Fatalf("HoldExpired %v", exp)
	}
	ret := returnedSeats(t, evs)
	if len(ret) != 2 || ret["A-R01-02"] != "A" || ret["B-R01-01"] != "B" {
		t.Fatalf("returned %v", ret)
	}
	if n := len(eventsOf(evs, outbox.InventoryReturned)); n != 2 {
		t.Fatalf("InventoryReturned %d, want one per section", n)
	}
	if len(eventsOf(evs, outbox.HoldCreated)) != 1 || len(eventsOf(evs, outbox.InventoryChanged)) != 1 {
		t.Fatal("new hold's events missing")
	}
	e.audit()
}

// P2: a seat of the old hold is locked by a concurrent transaction. It is
// skipped (no wait), the old hold stays live, and a later reclaim finishes
// it (the expiry worker's job in production, T11).
func TestP2ConcurrentLockSkipped(t *testing.T) {
	e := newEnv(t)
	old := e.mustHold(userA, "A-R01-01", "A-R01-02", "A-R01-03")
	e.expire(old)
	release := e.lockSeats("A-R01-02")
	start := time.Now()
	nw := e.mustHold(userB, "A-R01-01")
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("reclaim waited %v on a skipped lock", d)
	}
	e.expectSeat("A-R01-01", "HELD", nw)
	e.expectSeat("A-R01-02", "HELD", old) // skipped
	e.expectSeat("A-R01-03", "AVAILABLE", "")
	if h := e.holdRow(old); h.status != "ACTIVE" {
		t.Fatalf("old hold %s, want ACTIVE (not drained)", h.status)
	}
	e.audit()
	release()
	// Finishing the old hold: a reclaim of its last seat drains it.
	last := e.mustHold(userB, "A-R01-02")
	e.expectSeat("A-R01-02", "HELD", last)
	if h := e.holdRow(old); h.status != "EXPIRED" {
		t.Fatalf("old hold %s after its last seat", h.status)
	}
	e.audit()
}

// P3: a seat listed in the old hold's hold_seats but now owned by another
// hold is never released by a later reclaim of the old hold.
func TestP3AnotherHoldsSeatNeverReleased(t *testing.T) {
	e := newEnv(t)
	old := e.mustHold(userA, "A-R01-01", "A-R01-02", "A-R01-03")
	e.expire(old)
	release := e.lockSeats("A-R01-03")
	first := e.mustHold(userB, "A-R01-01") // releases A-R01-02, skips A-R01-03
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	other := e.mustHold(userA, "A-R01-02") // another hold now owns A-R01-02
	release()
	last := e.mustHold(userB, "A-R01-03") // reclaims the old hold again
	e.expectSeat("A-R01-01", "HELD", first)
	e.expectSeat("A-R01-02", "HELD", other) // untouched
	e.expectSeat("A-R01-03", "HELD", last)
	if h := e.holdRow(other); h.status != "ACTIVE" {
		t.Fatalf("other hold %s", h.status)
	}
	if h := e.holdRow(old); h.status != "EXPIRED" {
		t.Fatalf("old hold %s", h.status)
	}
	e.audit()
}

// P4: two buyers reclaim different seats of one old hold at the same time.
// Neither waits on the other's old-hold work, no seat is lost or doubled,
// and the old hold is expired at most once (or left for the worker when
// the drained check cannot yet see the other buyer's commit).
func TestP4ConcurrentReclaimsOfOneHold(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	expiredNow, leftLive, releasedThenTaken := 0, 0, 0
	for round := 0; round < 40; round++ {
		row := round%2 + 1
		n := (round / 2 % 2) * 4
		seats := []string{seatID("A", row, n+1), seatID("A", row, n+2), seatID("A", row, n+3), seatID("A", row, n+4)}
		e.resetSeats(seats...)
		old := e.mustHold(userA, seats...)
		e.expire(old)
		before := len(e.events())
		var wg sync.WaitGroup
		res := make([]SeatsResult, 2)
		errs := make([]error, 2)
		reqs := []Request{seatsRequest(t, seats[0]), seatsRequest(t, seats[1])}
		claims := []claimT{e.claim(userB, reqs[0]), e.claim(userB, reqs[1])}
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				res[i], errs[i] = HoldSeats(context.Background(), e.pool, claims[i], reqs[i])
			}(i)
		}
		close(start)
		wg.Wait()
		for i := range errs {
			if errs[i] != nil || res[i].Outcome != Held {
				t.Fatalf("round %d buyer %d: %+v %v", round, i, res[i], errs[i])
			}
		}
		e.expectSeat(seats[0], "HELD", res[0].HoldID)
		e.expectSeat(seats[1], "HELD", res[1].HoldID)
		e.expectSeat(seats[2], "AVAILABLE", "")
		e.expectSeat(seats[3], "AVAILABLE", "")
		evs := e.events()[before:]
		// The two unrequested seats are returned exactly once. A requested
		// seat may also have been returned: the reclaimer released it
		// before the other buyer locked it, and that buyer then took it as
		// AVAILABLE (its InventoryChanged follows). The final state above
		// shows nobody lost a seat they had taken.
		ret := returnedSeats(t, evs)
		if _, ok := ret[seats[2]]; !ok {
			t.Fatalf("round %d: %s not returned: %v", round, seats[2], ret)
		}
		if _, ok := ret[seats[3]]; !ok {
			t.Fatalf("round %d: %s not returned: %v", round, seats[3], ret)
		}
		for id := range ret {
			if id != seats[0] && id != seats[1] && id != seats[2] && id != seats[3] {
				t.Fatalf("round %d: returned foreign seat %s", round, id)
			}
			if id == seats[0] || id == seats[1] {
				releasedThenTaken++
			}
		}
		switch e.holdRow(old).status {
		case "EXPIRED":
			expiredNow++
			if len(expiredHolds(t, evs)) != 1 {
				t.Fatalf("round %d: EXPIRED without exactly one HoldExpired", round)
			}
		case "ACTIVE": // left for the worker: it must own no seat
			leftLive++
			if n := e.count(`SELECT count(*) FROM seats WHERE hold_id = $1`, old); n != 0 {
				t.Fatalf("round %d: live old hold still owns %d seats", round, n)
			}
		default:
			t.Fatalf("round %d: old hold %s", round, e.holdRow(old).status)
		}
		e.audit()
	}
	t.Logf("old hold expired inline %d times, left for the worker %d times; requested seat released before its buyer locked it %d times", expiredNow, leftLive, releasedThenTaken)
}

// P5: two buyers race for the same expired seat: exactly one wins, the
// other gets 409, and the old hold is expired exactly once.
func TestP5ReclaimRacesNewHold(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	for round := 0; round < 60; round++ {
		seat := seatID("B", 1, round%10+1)
		e.resetSeats(seat)
		old := e.mustHold(userA, seat)
		e.expire(old)
		before := len(e.events())
		reqs := []Request{seatsRequest(t, seat), seatsRequest(t, seat)}
		claims := []claimT{e.claim(userA, reqs[0]), e.claim(userB, reqs[1])}
		res := make([]SeatsResult, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				res[i], errs[i] = HoldSeats(context.Background(), e.pool, claims[i], reqs[i])
			}(i)
		}
		close(start)
		wg.Wait()
		wins := 0
		for i := range res {
			if errs[i] != nil {
				t.Fatalf("round %d: %v", round, errs[i])
			}
			if res[i].Outcome == Held {
				wins++
				e.expectSeat(seat, "HELD", res[i].HoldID)
			} else if res[i].Outcome != Unavailable {
				t.Fatalf("round %d: %v", round, res[i].Outcome)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: %d winners", round, wins)
		}
		if e.holdRow(old).status != "EXPIRED" || len(expiredHolds(t, e.events()[before:])) != 1 {
			t.Fatalf("round %d: old hold not expired exactly once", round)
		}
		e.audit()
	}
}

// L1: reclaim vs the expiry worker on one hold. The worker (simulated with
// its §8.1 lock pattern: the hold row, then its seats, both SKIP LOCKED)
// owns the old hold. The reclaim must neither wait on the hold nor touch
// the seats the worker owns, and neither side may deadlock.
func TestL1ReclaimVersusWorker(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old := e.mustHold(userA, "A-R01-01", "A-R01-02", "A-R01-03")
	e.expire(old)

	// (a) The worker owns the hold row and one seat; the buyer wants a
	// seat the worker has not locked. The buyer proceeds at once.
	w, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Rollback(ctx)
	if n := lockCount(t, w, `SELECT hold_id FROM holds WHERE hold_id = $1 FOR UPDATE SKIP LOCKED`, old); n != 1 {
		t.Fatalf("worker locked %d holds", n)
	}
	if n := lockCount(t, w, `SELECT seat_id FROM seats WHERE hold_id = $1 AND seat_id = 'A-R01-02' ORDER BY seat_id FOR UPDATE SKIP LOCKED`, old); n != 1 {
		t.Fatalf("worker locked %d seats", n)
	}
	start := time.Now()
	nw := e.mustHold(userB, "A-R01-01")
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("buyer waited %v behind the worker", d)
	}
	e.expectSeat("A-R01-01", "HELD", nw)
	e.expectSeat("A-R01-02", "HELD", old) // the worker's: untouched
	e.expectSeat("A-R01-03", "HELD", old) // the hold row was the worker's: no reclaim
	// The worker finishes its batch: it releases what it owns and can lock
	// (SKIP LOCKED, never waiting on the buyer's committed seat).
	if _, err := w.Exec(ctx, `UPDATE seats SET status = 'AVAILABLE', hold_id = NULL, hold_expires_at = NULL
		WHERE seat_id IN (SELECT seat_id FROM seats WHERE hold_id = $1 AND status = 'HELD' ORDER BY seat_id FOR UPDATE SKIP LOCKED)`, old); err != nil {
		t.Fatalf("worker: %v (sqlstate %s)", err, sqlState(err))
	}
	if _, err := w.Exec(ctx, `UPDATE holds SET status = 'EXPIRED' WHERE hold_id = $1 AND status = 'ACTIVE'`, old); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	e.expectSeat("A-R01-03", "AVAILABLE", "")
	e.expectSeat("A-R01-01", "HELD", nw)
	e.audit()

	// (b) The worker owns the seat the buyer wants. The buyer waits on that
	// seat (rank 2, allowed); the worker never waits on the buyer, so it
	// finishes and the buyer then takes the released seat.
	withLockTimeout(t, 20*time.Second)
	old2 := e.mustHold(userA, "B-R01-01", "B-R01-02")
	e.expire(old2)
	w2, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Rollback(ctx)
	lockCount(t, w2, `SELECT hold_id FROM holds WHERE hold_id = $1 FOR UPDATE SKIP LOCKED`, old2)
	lockCount(t, w2, `SELECT seat_id FROM seats WHERE hold_id = $1 ORDER BY seat_id FOR UPDATE SKIP LOCKED`, old2)
	ch := make(chan SeatsResult, 1)
	errc := make(chan error, 1)
	req := seatsRequest(t, "B-R01-01")
	c := e.claim(userB, req)
	go func() { r, err := HoldSeats(ctx, e.pool, c, req); ch <- r; errc <- err }()
	e.waitBlocked(1)
	if _, err := w2.Exec(ctx, `UPDATE seats SET status = 'AVAILABLE', hold_id = NULL, hold_expires_at = NULL WHERE hold_id = $1 AND status = 'HELD'`, old2); err != nil {
		t.Fatal(err)
	}
	if _, err := w2.Exec(ctx, `UPDATE holds SET status = 'EXPIRED' WHERE hold_id = $1 AND status = 'ACTIVE'`, old2); err != nil {
		t.Fatal(err)
	}
	if err := w2.Commit(ctx); err != nil {
		t.Fatalf("worker: %v (sqlstate %s)", err, sqlState(err))
	}
	r, err := <-ch, <-errc
	if err != nil || r.Outcome != Held {
		t.Fatalf("buyer after worker: %+v %v (sqlstate %s)", r, err, sqlState(err))
	}
	e.expectSeat("B-R01-01", "HELD", r.HoldID)
	e.expectSeat("B-R01-02", "AVAILABLE", "")
	e.audit()
}

func lockCount(t *testing.T, tx pgx.Tx, sql string, args ...any) int {
	t.Helper()
	rows, err := tx.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return n
}

// L2: two partial reclaims whose old holds cross: buyer 1 takes a seat of
// H1 and a seat of H2, buyer 2 takes the other seats of H2 and H1. Under a
// naive order each would wait on the other's old hold. 300 rounds, 0
// deadlocks, a clean audit.
func TestL2CrossingPartialReclaims(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	deadlocks := 0
	for round := 0; round < 300; round++ {
		a1, a2 := seatID("A", 1, round%5*2+1), seatID("A", 1, round%5*2+2)
		b1, b2 := seatID("A", 2, round%5*2+1), seatID("A", 2, round%5*2+2)
		e.resetSeats(a1, a2, b1, b2)
		h1 := e.mustHold(userA, a1, a2)
		h2 := e.mustHold(userA, b1, b2)
		e.expire(h1)
		e.expire(h2)
		reqs := []Request{seatsRequest(t, a1, b1), seatsRequest(t, b2, a2)}
		claims := []claimT{e.claim(userB, reqs[0]), e.claim(userB, reqs[1])}
		res := make([]SeatsResult, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				res[i], errs[i] = HoldSeats(context.Background(), e.pool, claims[i], reqs[i])
			}(i)
		}
		close(start)
		wg.Wait()
		for i := range errs {
			if sqlState(errs[i]) == SQLStateDeadlock {
				deadlocks++
				continue
			}
			if errs[i] != nil || res[i].Outcome != Held {
				t.Fatalf("round %d buyer %d: %+v %v", round, i, res[i], errs[i])
			}
		}
		// Every seat is held by exactly the buyer that asked for it.
		e.expectSeat(a1, "HELD", res[0].HoldID)
		e.expectSeat(b1, "HELD", res[0].HoldID)
		e.expectSeat(b2, "HELD", res[1].HoldID)
		e.expectSeat(a2, "HELD", res[1].HoldID)
		for _, h := range []string{h1, h2} {
			st := e.holdRow(h).status
			if st != "EXPIRED" && st != "ACTIVE" {
				t.Fatalf("round %d: old hold %s", round, st)
			}
			if n := e.count(`SELECT count(*) FROM seats WHERE hold_id = $1`, h); n != 0 {
				t.Fatalf("round %d: old hold still owns %d seats", round, n)
			}
		}
		if round%50 == 0 {
			e.audit()
		}
	}
	if deadlocks != 0 {
		t.Fatalf("%d deadlocks (40P01)", deadlocks)
	}
	e.audit()
}

// D5 for the reclaim path: a failure after the reclaim leaves the old hold
// and its seats exactly as they were.
func TestReclaimRollbackLeavesOldHold(t *testing.T) {
	e := newEnv(t)
	old := e.mustHold(userA, "A-R01-01", "A-R01-02")
	e.expire(old)
	before := len(e.events())
	boom := errors.New("injected failure")
	afterEffectsHook = func(context.Context, pgx.Tx) error { return boom }
	t.Cleanup(func() { afterEffectsHook = nil })
	req := seatsRequest(t, "A-R01-01")
	if _, err := HoldSeats(context.Background(), e.pool, e.claim(userB, req), req); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	afterEffectsHook = nil
	e.expectSeat("A-R01-01", "HELD", old)
	e.expectSeat("A-R01-02", "HELD", old)
	if h := e.holdRow(old); h.status != "ACTIVE" {
		t.Fatalf("old hold %s", h.status)
	}
	if n := len(e.events()); n != before {
		t.Fatalf("events %d, want %d", n, before)
	}
	e.audit()
}

// TestReclaimTouchesOnlyLockedHolds: the reclaim releases seats only of
// the old holds this transaction took seats from and locked. Another
// expired hold (the worker's job) and a live hold keep their seats.
func TestReclaimTouchesOnlyLockedHolds(t *testing.T) {
	e := newEnv(t)
	h1 := e.mustHold(userA, "A-R01-01", "A-R01-02")
	h2 := e.mustHold(userA, "A-R01-03")
	h3 := e.mustHold(userA, "A-R01-04")
	e.expire(h1)
	e.expire(h2)
	before := len(e.events())
	nw := e.mustHold(userB, "A-R01-01")
	e.expectSeat("A-R01-01", "HELD", nw)
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	e.expectSeat("A-R01-03", "HELD", h2) // expired, but not ours to reclaim
	e.expectSeat("A-R01-04", "HELD", h3) // live
	if e.holdRow(h1).status != "EXPIRED" || e.holdRow(h2).status != "ACTIVE" || e.holdRow(h3).status != "ACTIVE" {
		t.Fatal("hold statuses changed beyond the reclaimed hold")
	}
	ret := returnedSeats(t, e.events()[before:])
	if len(ret) != 1 || ret["A-R01-02"] != "A" {
		t.Fatalf("returned %v", ret)
	}
	if exp := expiredHolds(t, e.events()[before:]); len(exp) != 1 || !exp[h1] {
		t.Fatalf("expired %v", exp)
	}
	e.audit()
}
