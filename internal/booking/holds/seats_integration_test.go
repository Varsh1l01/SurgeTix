//go:build integration

package holds

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/booking/inventory"
	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

// TestHoldSeatsCreatesHold: a hold spanning two sections commits the seats,
// the hold, hold_seats, the outbox events and the stored 201 together.
func TestHoldSeatsCreatesHold(t *testing.T) {
	e := newEnv(t)
	req := seatsRequest(t, "B-R01-01", "A-R01-02", "A-R01-01")
	c := e.claim(userA, req)
	res, err := HoldSeats(context.Background(), e.pool, c, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Held || res.Response.Code != http.StatusCreated {
		t.Fatalf("got %v %d", res.Outcome, res.Response.Code)
	}
	body := decodeCreated(t, res)
	if body.HoldID != res.HoldID || body.Amount != 2*priceA+priceB || len(body.Seats) != 3 {
		t.Fatalf("201 body %+v", body)
	}
	wantSeats := []HeldSeat{
		{SeatID: "A-R01-01", Section: "A", Row: "R01", Number: 1, PricePaise: priceA},
		{SeatID: "A-R01-02", Section: "A", Row: "R01", Number: 2, PricePaise: priceA},
		{SeatID: "B-R01-01", Section: "B", Row: "R01", Number: 1, PricePaise: priceB},
	}
	for i, s := range wantSeats {
		if body.Seats[i] != s {
			t.Fatalf("seat %d: %+v, want %+v", i, body.Seats[i], s)
		}
	}

	h := e.holdRow(res.HoldID)
	if h.status != "ACTIVE" || h.kind != "SEATS" || h.user != userA {
		t.Fatalf("hold row %+v", h)
	}
	if got := h.expiresAt.Sub(h.createdAt); got != HoldDuration {
		t.Fatalf("expires_at - created_at = %v, want %v", got, HoldDuration)
	}
	if body.ExpiresAt != h.expiresAt.UTC().Format(outbox.OccurredAtLayout) {
		t.Fatalf("201 expires_at %s, hold %v", body.ExpiresAt, h.expiresAt)
	}
	for _, s := range wantSeats {
		e.expectSeat(s.SeatID, "HELD", res.HoldID)
		if m := e.seat(s.SeatID).expiry; m == nil || !m.Equal(h.expiresAt) {
			t.Fatalf("seat %s mirror %v != holds.expires_at %v (invariant 9)", s.SeatID, m, h.expiresAt)
		}
	}
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE hold_id = $1`, res.HoldID); n != 3 {
		t.Fatalf("hold_seats = %d", n)
	}

	status, code, stored := e.idemRow(c)
	var a, b any
	_ = json.Unmarshal(stored, &a)
	_ = json.Unmarshal(res.Response.Body, &b)
	if status != "COMPLETED" || code == nil || *code != 201 || !jsonEqual(a, b) {
		t.Fatalf("idempotency row %s %v %s", status, code, stored)
	}

	evs := e.events()
	if len(evs) != 3 {
		t.Fatalf("%d events, want HoldCreated + 2 InventoryChanged", len(evs))
	}
	hc := eventsOf(evs, outbox.HoldCreated)
	if len(hc) != 1 || hc[0].topic != outbox.TopicHolds || hc[0].key != res.HoldID {
		t.Fatalf("HoldCreated %+v", hc)
	}
	var d holdCreatedData
	_ = json.Unmarshal(hc[0].Data, &d)
	if d.HoldID != res.HoldID || d.EventID != eventID || d.UserID != userA || d.Kind != "SEATS" ||
		d.AmountPaise != body.Amount || d.ExpiresAt != body.ExpiresAt || joinIDs(d.Seats) != "A-R01-01,A-R01-02,B-R01-01" {
		t.Fatalf("HoldCreated data %+v", d)
	}
	ic := eventsOf(evs, outbox.InventoryChanged)
	keys := map[string]string{}
	for _, ev := range ic {
		var d inventoryChangedData
		_ = json.Unmarshal(ev.Data, &d)
		var ids []string
		for _, s := range d.Seats {
			if s.Status != "HELD" {
				t.Fatalf("InventoryChanged status %s", s.Status)
			}
			ids = append(ids, s.SeatID)
		}
		if ev.topic != outbox.TopicInventory || ev.key != eventID+":"+d.Section {
			t.Fatalf("InventoryChanged topic/key %s %s", ev.topic, ev.key)
		}
		keys[d.Section] = joinIDs(ids)
	}
	if len(keys) != 2 || keys["A"] != "A-R01-01,A-R01-02" || keys["B"] != "B-R01-01" {
		t.Fatalf("InventoryChanged per section %v", keys)
	}
	e.audit()
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// TestHoldSeatsUnavailableStores409: a taken seat refuses the request;
// nothing changes but the stored 409.
func TestHoldSeatsUnavailableStores409(t *testing.T) {
	e := newEnv(t)
	first := e.mustHold(userA, "A-R01-01")
	req := seatsRequest(t, "A-R01-02", "A-R01-01")
	c := e.claim(userB, req)
	res, err := HoldSeats(context.Background(), e.pool, c, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Unavailable || res.Response.Code != http.StatusConflict || res.HoldID != "" {
		t.Fatalf("got %+v", res)
	}
	var p seatsUnavailableProblem
	_ = json.Unmarshal(res.Response.Body, &p)
	if p.Type != problemSeatsUnavailable || p.Status != 409 || joinIDs(p.UnavailableSeatIDs) != "A-R01-01" {
		t.Fatalf("409 body %s", res.Response.Body)
	}
	e.expectSeat("A-R01-01", "HELD", first)
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	if n := e.count(`SELECT count(*) FROM holds`); n != 1 {
		t.Fatalf("holds = %d", n)
	}
	if n := len(e.events()); n != 2 {
		t.Fatalf("events = %d, want only the first hold's 2", n)
	}
	if status, code, _ := e.idemRow(c); status != "COMPLETED" || *code != 409 {
		t.Fatalf("idempotency %s %v", status, code)
	}
	// The stored 409 replays for the same key and body (T5 + T7 hash).
	cc, _ := req.Canonical()
	h, _ := idempotency.RequestHash(holdScope, cc)
	r, err := e.store.Claim(context.Background(), c.Key, h)
	if err != nil || r.Outcome != idempotency.Replay || r.Response.Code != 409 {
		t.Fatalf("replay: %v %+v", err, r)
	}
	e.audit()
}

// TestHoldSeatsUnknownSeatsStores422: a seat that does not exist for the
// event makes the request invalid (plan §11.2), even if another requested
// seat is also unavailable. Nothing changes but the stored 422.
func TestHoldSeatsUnknownSeatsStores422(t *testing.T) {
	e := newEnv(t)
	taken := e.mustHold(userA, "A-R01-01")
	for _, ids := range [][]string{
		{"A-R01-02", "Z-R01-01"},
		{"A-R01-01", "A-R09-09"}, // unavailable + unknown → 422
		{"a-R01-02"},             // ids are case-sensitive
	} {
		req := seatsRequest(t, ids...)
		c := e.claim(userB, req)
		res, err := HoldSeats(context.Background(), e.pool, c, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Outcome != UnknownSeats || res.Response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v: got %+v", ids, res)
		}
		var p unknownSeatsProblem
		_ = json.Unmarshal(res.Response.Body, &p)
		if p.Type != problemInvalidRequest || len(p.UnknownSeatIDs) != 1 {
			t.Fatalf("422 body %s", res.Response.Body)
		}
		if status, code, _ := e.idemRow(c); status != "COMPLETED" || *code != 422 {
			t.Fatalf("idempotency %s %v", status, code)
		}
	}
	// An unknown event: every seat is unknown.
	other := "0b9e5d4c-3a21-4f10-8e7d-6c5b4a392817"
	body, _ := json.Marshal(map[string]any{"seat_ids": []string{"A-R01-03"}})
	req, err := ParseRequest(other, body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := HoldSeats(context.Background(), e.pool, e.claim(userB, req), req)
	if err != nil || res.Outcome != UnknownSeats {
		t.Fatalf("unknown event: %+v %v", res, err)
	}
	e.expectSeat("A-R01-01", "HELD", taken)
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	if n := e.count(`SELECT count(*) FROM holds`); n != 1 {
		t.Fatalf("holds = %d", n)
	}
	if n := len(e.events()); n != 2 {
		t.Fatalf("events = %d", n)
	}
	e.audit()
}

// TestD1TwoBuyersOneSeatForcedOrder: buyer A queues on the seat first, then
// buyer B. When the in-flight holder lets go, A wins and B gets the 409.
func TestD1TwoBuyersOneSeatForcedOrder(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	for i, seat := range []string{"A-R01-01", "A-R02-05", "B-R01-10"} {
		release := e.lockSeats(seat) // an in-flight transaction owns the row
		reqA, reqB := seatsRequest(t, seat), seatsRequest(t, seat)
		cA, cB := e.claim(userA, reqA), e.claim(userB, reqB)
		type out struct {
			r   SeatsResult
			err error
		}
		chA, chB := make(chan out, 1), make(chan out, 1)
		go func() { r, err := HoldSeats(context.Background(), e.pool, cA, reqA); chA <- out{r, err} }()
		e.waitBlocked(1)
		go func() { r, err := HoldSeats(context.Background(), e.pool, cB, reqB); chB <- out{r, err} }()
		e.waitBlocked(2)
		release()
		a, b := <-chA, <-chB
		if a.err != nil || b.err != nil {
			t.Fatalf("round %d: %v / %v", i, a.err, b.err)
		}
		if a.r.Outcome != Held || b.r.Outcome != Unavailable {
			t.Fatalf("round %d: A %v, B %v (A queued first and must win)", i, a.r.Outcome, b.r.Outcome)
		}
		e.expectSeat(seat, "HELD", a.r.HoldID)
		e.audit()
	}
}

// TestD2ManyBuyersLimitedSeats: 80 concurrent buyers ask for random 1-4
// seats out of 10. Every seat ends with at most one hold, every 201 holds
// exactly its seats, and the events match the outcomes.
func TestD2ManyBuyersLimitedSeats(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	rng := rand.New(rand.NewSource(42))
	const buyers = 80
	reqs := make([]Request, buyers)
	claims := make([]idempotency.Claim, buyers)
	for i := range reqs {
		n := 1 + rng.Intn(4)
		perm := rng.Perm(10)[:n]
		ids := make([]string, n)
		for j, p := range perm {
			ids[j] = seatID("A", 1, p+1)
		}
		reqs[i] = seatsRequest(t, ids...)
		claims[i] = e.claim([]string{userA, userB}[i%2], reqs[i])
	}
	results := make([]SeatsResult, buyers)
	errs := make([]error, buyers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range reqs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = HoldSeats(context.Background(), e.pool, claims[i], reqs[i])
		}(i)
	}
	close(start)
	wg.Wait()

	owner := map[string]string{}
	held, refused := 0, 0
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("buyer %d: %v (sqlstate %s)", i, errs[i], sqlState(errs[i]))
		}
		switch r.Outcome {
		case Held:
			held++
			for _, id := range reqs[i].SeatIDs {
				if prev, dup := owner[id]; dup {
					t.Fatalf("seat %s in holds %s and %s", id, prev, r.HoldID)
				}
				owner[id] = r.HoldID
				e.expectSeat(id, "HELD", r.HoldID)
			}
		case Unavailable:
			refused++
			var p seatsUnavailableProblem
			_ = json.Unmarshal(r.Response.Body, &p)
			if len(p.UnavailableSeatIDs) == 0 {
				t.Fatalf("buyer %d: empty 409 list", i)
			}
			req := map[string]bool{}
			for _, id := range reqs[i].SeatIDs {
				req[id] = true
			}
			for _, id := range p.UnavailableSeatIDs {
				if !req[id] {
					t.Fatalf("buyer %d: 409 lists %s it did not ask for", i, id)
				}
			}
		default:
			t.Fatalf("buyer %d: %v", i, r.Outcome)
		}
	}
	if held == 0 || refused == 0 {
		t.Fatalf("held %d refused %d: the test did not contend", held, refused)
	}
	if n := e.count(`SELECT count(*) FROM seats WHERE status = 'HELD'`); n != len(owner) {
		t.Fatalf("HELD seats %d, seats in 201s %d", n, len(owner))
	}
	evs := e.events()
	if n := len(eventsOf(evs, outbox.HoldCreated)); n != held {
		t.Fatalf("HoldCreated %d, holds %d", n, held)
	}
	if n := len(eventsOf(evs, outbox.InventoryChanged)); n != held { // one section
		t.Fatalf("InventoryChanged %d, holds %d", n, held)
	}
	if n := e.count(`SELECT count(*) FROM idempotency_keys WHERE status = 'COMPLETED'`); n != buyers {
		t.Fatalf("completed keys %d, want %d", n, buyers)
	}
	t.Logf("80 buyers: %d holds, %d refusals, %d seats held", held, refused, len(owner))
	e.audit()
}

// TestD3AllOrNothing: one unavailable seat refuses the whole request; the
// free seats stay untouched.
func TestD3AllOrNothing(t *testing.T) {
	e := newEnv(t)
	taken := e.mustHold(userA, "B-R01-05")
	res := e.hold(userB, "A-R01-01", "B-R01-05", "B-R01-06")
	if res.Outcome != Unavailable {
		t.Fatalf("got %v", res.Outcome)
	}
	e.expectSeat("A-R01-01", "AVAILABLE", "")
	e.expectSeat("B-R01-06", "AVAILABLE", "")
	e.expectSeat("B-R01-05", "HELD", taken)
	if e.seat("A-R01-01").expiry != nil {
		t.Fatal("refused request left a mirror on a free seat")
	}
	if n := e.count(`SELECT count(*) FROM holds`); n != 1 {
		t.Fatalf("holds = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM hold_seats`); n != 1 {
		t.Fatalf("hold_seats = %d", n)
	}
	e.audit()
}

// TestD4ReversedOverlappingSets: 1,000 rounds of two buyers asking for
// overlapping seat sets in opposite orders, at the same instant. Exactly
// one wins each round and no transaction ever deadlocks (40P01).
func TestD4ReversedOverlappingSets(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t, inventory.SeatSection{Name: "C", Rows: 40, SeatsPerRow: 100, PricePaise: 1000})
	const rounds = 1000
	deadlocks, other := 0, 0
	for i := 0; i < rounds; i++ {
		row, base := i/25+1, (i%25)*4+1
		s := []string{seatID3("C", row, base), seatID3("C", row, base+1), seatID3("C", row, base+2), seatID3("C", row, base+3)}
		// Unsorted requests built directly (ParseRequest would sort them):
		// the transaction's own ordering must prevent the deadlock.
		ra := Request{EventID: eventID, Mode: ModeSeats, SeatIDs: []string{s[0], s[1], s[2]}, Quantity: 3}
		rb := Request{EventID: eventID, Mode: ModeSeats, SeatIDs: []string{s[3], s[2], s[1]}, Quantity: 3}
		ca, cb := e.claim(userA, ra), e.claim(userB, rb)
		var wg sync.WaitGroup
		var resA, resB SeatsResult
		var errA, errB error
		start := make(chan struct{})
		wg.Add(2)
		go func() { defer wg.Done(); <-start; resA, errA = HoldSeats(context.Background(), e.pool, ca, ra) }()
		go func() { defer wg.Done(); <-start; resB, errB = HoldSeats(context.Background(), e.pool, cb, rb) }()
		close(start)
		wg.Wait()
		for _, err := range []error{errA, errB} {
			if err != nil {
				if sqlState(err) == SQLStateDeadlock {
					deadlocks++
				} else {
					other++
					t.Logf("round %d: %v", i, err)
				}
			}
		}
		if errA == nil && errB == nil {
			wins := 0
			for _, r := range []SeatsResult{resA, resB} {
				if r.Outcome == Held {
					wins++
				}
			}
			if wins != 1 {
				t.Fatalf("round %d: %d winners", i, wins)
			}
		}
	}
	if deadlocks != 0 || other != 0 {
		t.Fatalf("%d deadlocks (40P01), %d other errors in %d rounds", deadlocks, other, rounds)
	}
	if n := e.count(`SELECT count(*) FROM holds`); n != rounds {
		t.Fatalf("holds %d, want %d (one per round)", n, rounds)
	}
	e.audit()
}

// TestD5RollbackLeavesNothing: a failure after every effect was written
// (just before the idempotency completion) leaves no trace, and the claim
// is untouched so the same request can still run.
func TestD5RollbackLeavesNothing(t *testing.T) {
	e := newEnv(t)
	req := seatsRequest(t, "A-R01-01", "B-R01-01")
	c := e.claim(userA, req)
	boom := errors.New("injected failure")
	afterEffectsHook = func(context.Context, pgx.Tx) error { return boom }
	t.Cleanup(func() { afterEffectsHook = nil })
	if _, err := HoldSeats(context.Background(), e.pool, c, req); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	afterEffectsHook = nil
	e.expectSeat("A-R01-01", "AVAILABLE", "")
	e.expectSeat("B-R01-01", "AVAILABLE", "")
	for _, tbl := range []string{"holds", "hold_seats", "outbox"} {
		if n := e.count(`SELECT count(*) FROM ` + tbl); n != 0 {
			t.Fatalf("%s has %d rows after rollback", tbl, n)
		}
	}
	if status, code, _ := e.idemRow(c); status != "IN_PROGRESS" || code != nil {
		t.Fatalf("claim changed: %s %v", status, code)
	}
	e.audit()
	// Same claim, no failure: it runs normally.
	res, err := HoldSeats(context.Background(), e.pool, c, req)
	if err != nil || res.Outcome != Held {
		t.Fatalf("retry: %+v %v", res, err)
	}
	e.audit()
}

// TestLostClaimChangesNothing: a claim that is no longer current (here:
// abandoned) is refused at the fence, before any seat is touched.
func TestLostClaimChangesNothing(t *testing.T) {
	e := newEnv(t)
	req := seatsRequest(t, "A-R01-01")
	c := e.claim(userA, req)
	if ok, err := e.store.Abandon(context.Background(), c); err != nil || !ok {
		t.Fatalf("abandon: %v %v", ok, err)
	}
	if _, err := HoldSeats(context.Background(), e.pool, c, req); !errors.Is(err, idempotency.ErrNotOwner) {
		t.Fatalf("got %v, want ErrNotOwner", err)
	}
	e.expectSeat("A-R01-01", "AVAILABLE", "")
	if n := e.count(`SELECT count(*) FROM holds`) + e.count(`SELECT count(*) FROM outbox`); n != 0 {
		t.Fatalf("%d rows written", n)
	}
	e.audit()
}

// TestLostClaimTakesNoSeatLock: the fence is the first lock. A lost claim
// is refused before any seat lock is requested, so it never queues behind
// (or delays) the real owner of a seat.
func TestLostClaimTakesNoSeatLock(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	release := e.lockSeats("A-R01-04")
	defer release()
	req := seatsRequest(t, "A-R01-04")
	c := e.claim(userA, req)
	if ok, err := e.store.Abandon(context.Background(), c); err != nil || !ok {
		t.Fatalf("abandon: %v %v", ok, err)
	}
	start := time.Now()
	_, err := HoldSeats(context.Background(), e.pool, c, req)
	if !errors.Is(err, idempotency.ErrNotOwner) {
		t.Fatalf("got %v, want ErrNotOwner from the fence", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("lost claim waited %v on a seat lock", d)
	}
}

// TestLockTimeoutIsApplied: with the production value (500 ms), a seat
// locked by a stuck transaction makes HoldSeats fail with 55P03 after about
// 500 ms. Nothing is stored and the claim stays live for a retry.
func TestLockTimeoutIsApplied(t *testing.T) {
	if lockTimeout != 500*time.Millisecond || LockTimeout != 500*time.Millisecond {
		t.Fatalf("lock timeout %v / %v, want the approved 500ms", lockTimeout, LockTimeout)
	}
	e := newEnv(t)
	release := e.lockSeats("A-R01-03")
	defer release()
	req := seatsRequest(t, "A-R01-02", "A-R01-03")
	c := e.claim(userA, req)
	start := time.Now()
	_, err := HoldSeats(context.Background(), e.pool, c, req)
	elapsed := time.Since(start)
	if sqlState(err) != SQLStateLockTimeout {
		t.Fatalf("got %v, want lock timeout 55P03", err)
	}
	if elapsed < 450*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("failed after %v, want about 500ms", elapsed)
	}
	release()
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	if status, code, _ := e.idemRow(c); status != "IN_PROGRESS" || code != nil {
		t.Fatalf("timeout stored a result: %s %v", status, code)
	}
	res, err := HoldSeats(context.Background(), e.pool, c, req)
	if err != nil || res.Outcome != Held {
		t.Fatalf("retry after timeout: %+v %v", res, err)
	}
	e.audit()
}

// TestOutboxMatchesOutcomes (O): only committed holds have events, and
// each has exactly HoldCreated plus one InventoryChanged per section.
func TestOutboxMatchesOutcomes(t *testing.T) {
	e := newEnv(t)
	h1 := e.mustHold(userA, "A-R01-01", "A-R02-01") // one section
	h2 := e.mustHold(userB, "A-R01-02", "B-R01-02") // two sections
	_ = e.hold(userB, "A-R01-01")                   // 409: no events
	_ = e.hold(userB, "Q-R01-01")                   // 422: no events
	want := map[string]int{h1: 1, h2: 1}
	evs := e.events()
	got := map[string]int{}
	for _, ev := range eventsOf(evs, outbox.HoldCreated) {
		got[ev.key]++
	}
	if len(got) != 2 || got[h1] != want[h1] || got[h2] != want[h2] {
		t.Fatalf("HoldCreated per hold %v", got)
	}
	if n := len(eventsOf(evs, outbox.InventoryChanged)); n != 3 {
		t.Fatalf("InventoryChanged %d, want 1 + 2", n)
	}
	if len(evs) != 5 {
		t.Fatalf("%d events, want 5", len(evs))
	}
	e.audit()
}
