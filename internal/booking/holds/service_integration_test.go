//go:build integration

package holds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
)

func seatsBody(ids ...string) []byte {
	b, _ := json.Marshal(map[string]any{"seat_ids": ids})
	return b
}

func newKey() string { return fmt.Sprintf("svc-%d", keySeq.Add(1)) }

func (e *env) svc() *Service { return NewService(e.pool) }

func (e *env) call(s *Service, user, key string, body []byte) Response {
	e.t.Helper()
	r, err := s.Hold(context.Background(), user, key, eventID, body)
	if err != nil {
		e.t.Fatalf("Hold: %v", err)
	}
	return r
}

func holdIDOf(t *testing.T, r Response) string {
	t.Helper()
	if r.Status != http.StatusCreated {
		t.Fatalf("status %d body %s", r.Status, r.Body)
	}
	var c Created
	if err := json.Unmarshal(r.Body, &c); err != nil || c.HoldID == "" {
		t.Fatalf("201 body %s: %v", r.Body, err)
	}
	return c.HoldID
}

func problemType(t *testing.T, r Response) string {
	t.Helper()
	var p problemBody
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatalf("body %s: %v", r.Body, err)
	}
	if p.Status != r.Status {
		t.Fatalf("problem status %d != %d", p.Status, r.Status)
	}
	return p.Type
}

// ageClaim moves a live claim's claimed_at back past the 30 s lease, as if
// its owner had stalled or crashed 31 s ago (test SQL, as in T5).
func (e *env) ageClaim(user, key string) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE idempotency_keys SET claimed_at = claimed_at - interval '31 seconds'
		WHERE scope = $1 AND user_id = $2 AND key = $3`, HoldScope, user, key); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) holdCount() int { return e.count(`SELECT count(*) FROM holds`) }

// I1 (process crash): the session dies while blocked on a seat lock and the
// process never gets to abandon. No hold exists; retries get 409 in-progress
// until the lease expires, then one retry succeeds: exactly one hold.
func TestI1CrashWhileBlockedThenLeaseTakeover(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	crashed := e.svc()
	crashed.abandon = func(context.Context, idempotency.Claim) (bool, error) { return false, errors.New("process gone") }
	release := e.lockSeats("A-R01-01")
	key, body := newKey(), seatsBody("A-R01-01")

	done := make(chan Response, 1)
	go func() { r, _ := crashed.Hold(context.Background(), userA, key, eventID, body); done <- r }()
	e.waitBlocked(1)
	// Terminate the blocked session (the crash).
	if _, err := e.pool.Exec(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	if r := <-done; r.Status != http.StatusServiceUnavailable {
		t.Fatalf("crashed call: %d %s", r.Status, r.Body)
	}
	release()
	if e.holdCount() != 0 {
		t.Fatal("a hold exists after the crash")
	}
	s := e.svc()
	for i := 0; i < 3; i++ { // inside the lease: 409, nothing executes
		r := e.call(s, userA, key, body)
		if r.Status != http.StatusConflict || problemType(t, r) != problemIdempotencyInProgress {
			t.Fatalf("retry %d inside the lease: %d %s", i, r.Status, r.Body)
		}
	}
	e.ageClaim(userA, key)
	r := e.call(s, userA, key, body)
	id := holdIDOf(t, r)
	if r.Replayed || e.holdCount() != 1 {
		t.Fatalf("takeover: replayed %v holds %d", r.Replayed, e.holdCount())
	}
	again := e.call(s, userA, key, body)
	if !again.Replayed || holdIDOf(t, again) != id || e.holdCount() != 1 {
		t.Fatal("retry after takeover did not replay the one hold")
	}
	e.audit()
}

// I1 (live process): the session is terminated but the service survives,
// so it abandons the claim and the retry proceeds at once.
func TestI1SessionKilledServiceAbandons(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	s := e.svc()
	release := e.lockSeats("A-R01-02")
	key, body := newKey(), seatsBody("A-R01-02")
	done := make(chan Response, 1)
	go func() { r, _ := s.Hold(context.Background(), userA, key, eventID, body); done <- r }()
	e.waitBlocked(1)
	if _, err := e.pool.Exec(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	if r := <-done; r.Status != http.StatusServiceUnavailable || r.Cause == nil {
		t.Fatalf("killed call: %d %v", r.Status, r.Cause)
	}
	release()
	r := e.call(s, userA, key, body) // no lease wait: the claim was abandoned
	holdIDOf(t, r)
	if e.holdCount() != 1 {
		t.Fatalf("holds %d", e.holdCount())
	}
	e.audit()
}

// I2: the hold commits but the process dies before the service returns
// (the transaction ran, its result never left). The retry replays it.
func TestI2CrashAfterCommitReplays(t *testing.T) {
	e := newEnv(t)
	key, body := newKey(), seatsBody("A-R01-03", "B-R01-03")
	req, _ := ParseRequest(eventID, body)
	canon, _ := req.Canonical()
	h, _ := idempotency.RequestHash(HoldScope, canon)
	res, err := e.store.Claim(context.Background(), idempotency.Key{Scope: HoldScope, UserID: userA, Key: key}, h)
	if err != nil || res.Outcome != idempotency.Proceed {
		t.Fatal(res.Outcome, err)
	}
	committed, err := HoldSeats(context.Background(), e.pool, res.Claim, req)
	if err != nil || committed.Outcome != Held {
		t.Fatal(committed, err)
	}
	// ... crash: nothing returned to the client.
	r := e.call(e.svc(), userA, key, body)
	if !r.Replayed || holdIDOf(t, r) != committed.HoldID || e.holdCount() != 1 {
		t.Fatalf("replay %v %s, holds %d", r.Replayed, r.Body, e.holdCount())
	}
	e.audit()
}

// I3: the 201 is produced but lost on the way to the client; the retry
// (with the body re-serialised differently) replays the same hold_id.
func TestI3ResponseLostReplaysSameHold(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	key := newKey()
	first := holdIDOf(t, e.call(s, userA, key, []byte(`{"seat_ids":["A-R01-04","A-R01-05"]}`)))
	r := e.call(s, userA, key, []byte(` { "seat_ids" : [ "A-R01-05", "A-R01-04" ] } `))
	if !r.Replayed || holdIDOf(t, r) != first || e.holdCount() != 1 {
		t.Fatalf("replay %v, holds %d", r.Replayed, e.holdCount())
	}
	e.audit()
}

// I4: same key and body, sequentially and 50 at once: one hold, every
// other answer is the replay (or 409 in-progress while the first runs).
func TestI4SameKeySameBody(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	key, body := newKey(), seatsBody("A-R01-06")
	id := holdIDOf(t, e.call(s, userA, key, body))
	for i := 0; i < 3; i++ {
		if r := e.call(s, userA, key, body); !r.Replayed || holdIDOf(t, r) != id {
			t.Fatalf("sequential replay %d", i)
		}
	}

	// Warm the pool first: opening 50 connections on a loaded machine can
	// take seconds, which must not eat into the owner's 30 s lease.
	warm := make([]interface{ Release() }, 0, 55)
	for i := 0; i < 55; i++ {
		c, err := e.pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		warm = append(warm, c)
	}
	for _, c := range warm {
		c.Release()
	}

	// (a) Deterministic: the first request owns the claim and is held just
	// before its transaction while 49 more arrive. While its 30 s lease is
	// live, all 49 must get 409 in-progress and none may execute. If the
	// machine is so slow that the lease runs out first, a takeover is the
	// specified behaviour (plan §7.2): then exactly one request executes,
	// and the stalled owner is fenced out with a 503. Never two holds.
	keyD, bodyD := newKey(), seatsBody("A-R01-09")
	owner := e.svc()
	claimed, finish := make(chan struct{}), make(chan struct{})
	owner.beforeExecute = func(context.Context, idempotency.Claim) { close(claimed); <-finish }
	ownerOut := make(chan Response, 1)
	go func() { r, _ := owner.Hold(context.Background(), userA, keyD, eventID, bodyD); ownerOut <- r }()
	<-claimed
	claimedAt := time.Now()
	var wgD sync.WaitGroup
	others := make([]Response, 49)
	for i := range others {
		wgD.Add(1)
		go func(i int) {
			defer wgD.Done()
			others[i], _ = s.Hold(context.Background(), userA, keyD, eventID, bodyD)
		}(i)
	}
	wgD.Wait()
	elapsed := time.Since(claimedAt)
	close(finish)
	ownerRes := <-ownerOut

	executed := 0
	if ownerRes.Status == http.StatusCreated && !ownerRes.Replayed {
		executed++
	}
	for i, r := range others {
		switch {
		case r.Status == http.StatusConflict && problemType(t, r) == problemIdempotencyInProgress:
		case r.Status == http.StatusCreated && elapsed >= idempotency.Lease:
			if !r.Replayed {
				executed++
			}
		default:
			t.Fatalf("concurrent call %d (after %v, lease %v): %d %s", i, elapsed, idempotency.Lease, r.Status, r.Body)
		}
	}
	if executed != 1 {
		t.Fatalf("%d executions for one key, want exactly 1", executed)
	}
	if elapsed < idempotency.Lease {
		if ownerRes.Status != http.StatusCreated {
			t.Fatalf("owner within its lease: %d %s", ownerRes.Status, ownerRes.Body)
		}
	} else {
		t.Logf("slow machine: the 49 calls took %v, past the %v lease; takeover path checked", elapsed, idempotency.Lease)
		if ownerRes.Status == http.StatusServiceUnavailable && !errors.Is(ownerRes.Cause, idempotency.ErrNotOwner) {
			t.Fatalf("taken-over owner: cause %v", ownerRes.Cause)
		}
	}
	if r := e.call(s, userA, keyD, bodyD); !r.Replayed || r.Status != http.StatusCreated {
		t.Fatal("retry after the owner finished did not replay")
	}
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE seat_id = 'A-R01-09'`); n != 1 {
		t.Fatalf("A-R01-09 in %d holds", n)
	}

	// (b) Free-for-all: 50 calls released at once on the warmed pool (so
	// the first does not finish before the others even have a connection).
	key2, body2 := newKey(), seatsBody("A-R01-07", "A-R01-08")
	const n = 50
	out := make([]Response, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			out[i], _ = s.Hold(context.Background(), userB, key2, eventID, body2)
		}(i)
	}
	close(start)
	wg.Wait()
	created, replays, inProgress := 0, 0, 0
	var winner string
	for i, r := range out {
		switch {
		case r.Status == http.StatusCreated && !r.Replayed:
			created++
			winner = holdIDOf(t, r)
		case r.Status == http.StatusCreated && r.Replayed:
			replays++
		case r.Status == http.StatusConflict && problemType(t, r) == problemIdempotencyInProgress:
			inProgress++
		default:
			t.Fatalf("call %d: %d %s", i, r.Status, r.Body)
		}
	}
	if created != 1 {
		t.Fatalf("%d calls executed the hold, want exactly 1", created)
	}
	for _, r := range out {
		if r.Replayed && holdIDOf(t, r) != winner {
			t.Fatal("a replay returned a different hold")
		}
	}
	if e.count(`SELECT count(*) FROM holds WHERE user_id = $1`, userB) != 1 {
		t.Fatal("more than one hold for the key")
	}
	if e.holdCount() != 3 { // first sequential key, (a), (b)
		t.Fatalf("holds %d, want 3", e.holdCount())
	}
	t.Logf("50 concurrent: 1 executed, %d replays, %d in-progress", replays, inProgress)
	e.audit()
}

// I5: same key, different body: 422 key-reused, no effect, the original
// result stays (both after completion and while in flight).
func TestI5SameKeyDifferentBody(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	s := e.svc()
	key := newKey()
	id := holdIDOf(t, e.call(s, userA, key, seatsBody("A-R01-09")))
	r := e.call(s, userA, key, seatsBody("A-R01-10"))
	if r.Status != http.StatusUnprocessableEntity || problemType(t, r) != problemIdempotencyKeyReused {
		t.Fatalf("got %d %s", r.Status, r.Body)
	}
	e.expectSeat("A-R01-10", "AVAILABLE", "")
	if again := e.call(s, userA, key, seatsBody("A-R01-09")); !again.Replayed || holdIDOf(t, again) != id {
		t.Fatal("original result not kept")
	}

	// In flight: the first request is blocked on a seat lock.
	release := e.lockSeats("A-R02-01")
	key2 := newKey()
	done := make(chan Response, 1)
	go func() { r, _ := s.Hold(context.Background(), userA, key2, eventID, seatsBody("A-R02-01")); done <- r }()
	e.waitBlocked(1)
	r = e.call(s, userA, key2, seatsBody("A-R02-02"))
	if r.Status != http.StatusUnprocessableEntity || problemType(t, r) != problemIdempotencyKeyReused {
		t.Fatalf("in flight: %d %s", r.Status, r.Body)
	}
	release()
	holdIDOf(t, <-done)
	e.expectSeat("A-R02-02", "AVAILABLE", "")
	e.audit()
}

// I6 end to end: request A claims and stalls; its lease runs out; request B
// (same key and body) takes over and completes. A then wakes up: its fenced
// transaction does nothing, it answers 503, and its client's retry replays
// B's hold. B's record is never overwritten. One hold.
func TestI6StaleClaimantAfterTakeover(t *testing.T) {
	e := newEnv(t)
	key, body := newKey(), seatsBody("A-R02-03")
	stalled := e.svc()
	resume, bDone := make(chan struct{}), make(chan struct{})
	stalled.beforeExecute = func(context.Context, idempotency.Claim) {
		close(resume)
		<-bDone
	}
	aOut := make(chan Response, 1)
	go func() { r, _ := stalled.Hold(context.Background(), userA, key, eventID, body); aOut <- r }()
	<-resume // A owns generation 1 and is stalled before its transaction

	e.ageClaim(userA, key)
	b := e.call(e.svc(), userA, key, body) // takes over (generation 2) and completes
	bID := holdIDOf(t, b)
	if b.Replayed {
		t.Fatal("B should have executed")
	}
	close(bDone)

	a := <-aOut
	if a.Status != http.StatusServiceUnavailable || !errors.Is(a.Cause, idempotency.ErrNotOwner) {
		t.Fatalf("stale A: %d cause %v", a.Status, a.Cause)
	}
	if e.holdCount() != 1 {
		t.Fatalf("holds %d", e.holdCount())
	}
	retry := e.call(e.svc(), userA, key, body)
	if !retry.Replayed || holdIDOf(t, retry) != bID {
		t.Fatal("A's retry did not replay B's hold")
	}
	var status string
	var gen int64
	if err := e.pool.QueryRow(context.Background(), `SELECT status, claim_generation FROM idempotency_keys
		WHERE scope = $1 AND user_id = $2 AND key = $3`, HoldScope, userA, key).Scan(&status, &gen); err != nil {
		t.Fatal(err)
	}
	if status != "COMPLETED" || gen != 2 {
		t.Fatalf("record %s gen %d, want B's COMPLETED gen 2", status, gen)
	}
	e.audit()
}

// I7: a key past its 24 h retention is a new request, even with another
// body. The old hold is not replayed.
func TestI7ExpiredKeyReuse(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	key := newKey()
	first := holdIDOf(t, e.call(s, userA, key, seatsBody("A-R02-04")))
	if _, err := e.pool.Exec(context.Background(), `UPDATE idempotency_keys SET expires_at = now() - interval '1 second'
		WHERE scope = $1 AND user_id = $2 AND key = $3`, HoldScope, userA, key); err != nil {
		t.Fatal(err)
	}
	r := e.call(s, userA, key, seatsBody("A-R02-05"))
	second := holdIDOf(t, r)
	if r.Replayed || second == first {
		t.Fatal("expired key replayed the old result")
	}
	if again := e.call(s, userA, key, seatsBody("A-R02-05")); !again.Replayed || holdIDOf(t, again) != second {
		t.Fatal("the new record is not replayed")
	}
	e.audit()
}

// I8: a stored 409 replays even after the seat becomes free; a new key
// then succeeds.
func TestI8Stored409Replay(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	owner := holdIDOf(t, e.call(s, userA, newKey(), seatsBody("A-R02-06")))
	key, body := newKey(), seatsBody("A-R02-06")
	r := e.call(s, userB, key, body)
	if r.Status != http.StatusConflict || problemType(t, r) != problemSeatsUnavailable || r.Replayed {
		t.Fatalf("got %d %s", r.Status, r.Body)
	}
	e.expire(owner) // the seat is reclaimable now
	r = e.call(s, userB, key, body)
	if r.Status != http.StatusConflict || !r.Replayed {
		t.Fatalf("replay: %d replayed %v", r.Status, r.Replayed)
	}
	e.expectSeat("A-R02-06", "HELD", owner) // the replay did not execute
	holdIDOf(t, e.call(s, userB, newKey(), body))
	e.audit()
}

// TestLockTimeoutAbandonsAndRetrySucceeds: a lock timeout (500 ms) is a
// 503 with nothing stored; the claim is abandoned, so the client's retry
// with the same key runs immediately (no 30 s wait) once the seat frees.
func TestLockTimeoutAbandonsAndRetrySucceeds(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	release := e.lockSeats("B-R01-07")
	key, body := newKey(), seatsBody("B-R01-07")
	r := e.call(s, userA, key, body)
	if r.Status != http.StatusServiceUnavailable || sqlState(r.Cause) != SQLStateLockTimeout {
		t.Fatalf("got %d cause %v", r.Status, r.Cause)
	}
	if status, _, _ := e.idemRow(idempotency.Claim{Key: idempotency.Key{Scope: HoldScope, UserID: userA, Key: key}}); status != "IN_PROGRESS" {
		t.Fatalf("record %s", status)
	}
	release()
	ok := e.call(s, userA, key, body)
	if ok.Replayed || holdIDOf(t, ok) == "" || e.holdCount() != 1 {
		t.Fatal("retry after abandon did not execute once")
	}
	e.audit()
}

// TestCancelledRequestIsRetryable: the caller's context ends while the
// transaction waits; 503, nothing stored, claim abandoned.
func TestCancelledRequestIsRetryable(t *testing.T) {
	withLockTimeout(t, 20*time.Second)
	e := newEnv(t)
	s := e.svc()
	release := e.lockSeats("B-R01-08")
	key, body := newKey(), seatsBody("B-R01-08")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := s.Hold(ctx, userA, key, eventID, body)
	if err != nil || r.Status != http.StatusServiceUnavailable || r.Cause == nil {
		t.Fatalf("got %d %v %v", r.Status, r.Cause, err)
	}
	release()
	if holdIDOf(t, e.call(s, userA, key, body)) == "" || e.holdCount() != 1 {
		t.Fatal("retry did not execute once")
	}
	e.audit()
}

// TestServiceRefusalsAreNotClaimed: bad identity/key, invalid bodies and
// not-yet-available modes never claim the key or touch inventory.
func TestServiceRefusalsAreNotClaimed(t *testing.T) {
	e := newEnv(t)
	s := e.svc()
	cases := []struct {
		user, key string
		body      []byte
		status    int
		typ       string
	}{
		{"not-a-uuid", "k1", seatsBody("A-R01-01"), 400, problemBadRequest},
		{"", "k1", seatsBody("A-R01-01"), 400, problemBadRequest},
		{userA, "", seatsBody("A-R01-01"), 400, problemBadRequest},
		{userA, "k2", []byte(`{"seat_ids":[]}`), 422, problemInvalidRequest},
		{userA, "k3", []byte(`{"seat_ids":["A-R01-01"],"x":1}`), 422, problemInvalidRequest},
		{userA, "k4", []byte(`{"best_available":{"tier":"G","quantity":2}}`), 422, problemInvalidRequest},
	}
	for _, c := range cases {
		r, err := s.Hold(context.Background(), c.user, c.key, eventID, c.body)
		if err != nil || r.Status != c.status || problemType(t, r) != c.typ || r.Replayed {
			t.Fatalf("%+v: got %d %s %v", c, r.Status, r.Body, err)
		}
	}
	for _, body := range [][]byte{
		[]byte(`{"best_available":{"section":"A","quantity":2}}`),
		[]byte(`{"ga":{"section":"FLOOR","quantity":2}}`),
	} {
		if _, err := s.Hold(context.Background(), userA, "k5", eventID, body); !errors.Is(err, ErrModeNotAvailable) {
			t.Fatalf("%s: got %v", body, err)
		}
	}
	if n := e.count(`SELECT count(*) FROM idempotency_keys`); n != 0 {
		t.Fatalf("%d keys claimed by refused requests", n)
	}
	// A key first used with an invalid body is still free for a valid one.
	if holdIDOf(t, e.call(s, userA, "k2", seatsBody("A-R01-01"))) == "" {
		t.Fatal("valid request after an invalid one failed")
	}
	if e.holdCount() != 1 {
		t.Fatalf("holds %d", e.holdCount())
	}
	e.audit()
}
