//go:build integration

package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const scope = "POST /v1/events/{eventID}/holds"

type env struct {
	t     *testing.T
	pool  *pgxpool.Pool
	store *Store
}

// newEnv: a fresh booking_db with the approved schema (T2 migrations,
// including 00003 idempotency_keys and 00004 claim_generation/claimed_at),
// plus a test-only effects table standing in for "the protected operation".
func newEnv(t *testing.T) *env {
	t.Helper()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	ctx := context.Background()
	if _, err := migrations.Up(ctx, pool, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE effects (id serial PRIMARY KEY, idem_key text NOT NULL, generation bigint NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, pool: pool, store: New(pool)}
}

// begin starts a test transaction that is always rolled back at test end,
// even if an assertion fails first. t.Cleanup runs LIFO, before the pool's
// Close, so a failing test can never hang on a connection it still holds.
func (e *env) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := e.pool.Begin(ctx)
	if err == nil {
		e.t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	}
	return tx, err
}

func key(k string) Key {
	return Key{Scope: scope, UserID: "11111111-1111-4111-8111-111111111111", Key: k}
}

func (e *env) hash(body string) []byte {
	e.t.Helper()
	h, err := RequestHash(scope, []byte(body))
	if err != nil {
		e.t.Fatal(err)
	}
	return h
}

func (e *env) claim(k Key, body string) Result {
	e.t.Helper()
	r, err := e.store.Claim(context.Background(), k, e.hash(body))
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

// run executes the protected operation the way a business transaction must:
// Fence first, the effect, Complete last, then COMMIT.
func (e *env) run(c Claim, resp Response) error {
	ctx := context.Background()
	tx, err := e.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Fence(ctx, tx, c); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO effects (idem_key, generation) VALUES ($1, $2)`, c.Key.Key, c.Generation); err != nil {
		return err
	}
	if err := Complete(ctx, tx, c, resp); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e *env) effects(k string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM effects WHERE idem_key = $1`, k).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// age simulates "this claim was made `by` ago" without sleeping: it moves
// the row's claimed_at back AND updates the claimant's token to match, as if
// the clock had advanced. (In reality claimed_at never changes while time
// passes. Changing only the row would invalidate the token for the wrong
// reason and hide what the test means to prove: generation fencing.)
func (e *env) age(c *Claim, by time.Duration) {
	e.t.Helper()
	err := e.pool.QueryRow(context.Background(), `UPDATE idempotency_keys SET claimed_at = claimed_at - $4::interval
		WHERE scope = $1 AND user_id = $2 AND key = $3 RETURNING claimed_at`,
		c.Scope, c.UserID, c.Key.Key, by.String()).Scan(&c.ClaimedAt)
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) record(k Key) (status string, gen int64, code *int32, body []byte) {
	e.t.Helper()
	err := e.pool.QueryRow(context.Background(), `SELECT status, claim_generation, response_code, response_body
		FROM idempotency_keys WHERE scope = $1 AND user_id = $2 AND key = $3`, k.Scope, k.UserID, k.Key).Scan(&status, &gen, &code, &body)
	if err != nil {
		e.t.Fatal(err)
	}
	return
}

var created = Response{Code: 201, Body: json.RawMessage(`{"hold_id":"h-1","expires_at":"2026-11-06T10:00:00Z"}`)}

func sameJSON(t *testing.T, a, b []byte) {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		t.Fatalf("invalid JSON: %s / %s", a, b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	if string(xa) != string(ya) {
		t.Fatalf("JSON differs: %s vs %s", a, b)
	}
}

func TestFreshKeyProceedsAtGenerationOne(t *testing.T) {
	e := newEnv(t)
	r := e.claim(key("k1"), `{"seat_ids":["A-R01-01"]}`)
	if r.Outcome != Proceed || r.Claim.Generation != 1 {
		t.Fatalf("result = %+v", r)
	}
	if status, gen, _, _ := e.record(key("k1")); status != "IN_PROGRESS" || gen != 1 {
		t.Fatalf("record = %s gen %d", status, gen)
	}
}

func TestSameKeySameRequestReplaysStoredResponse(t *testing.T) {
	for name, resp := range map[string]Response{
		"stored 201":               created,
		"stored deterministic 409": {Code: 409, Body: json.RawMessage(`{"type":"urn:surgetix:problem:seats-unavailable","unavailable_seat_ids":["A-R01-01"]}`)},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			r := e.claim(key("k"), `{"seat_ids":["A-R01-01"]}`)
			if err := e.run(r.Claim, resp); err != nil {
				t.Fatal(err)
			}
			// Semantically equivalent request (whitespace, key order) replays too.
			for _, body := range []string{`{"seat_ids":["A-R01-01"]}`, `{ "seat_ids" : [ "A-R01-01" ] }`} {
				again := e.claim(key("k"), body)
				if again.Outcome != Replay || again.Response.Code != resp.Code {
					t.Fatalf("retry = %+v", again)
				}
				sameJSON(t, again.Response.Body, resp.Body)
			}
			if n := e.effects("k"); n != 1 {
				t.Fatalf("effect ran %d times, want 1", n)
			}
		})
	}
}

func TestSameKeyDifferentRequestIsMismatch(t *testing.T) {
	e := newEnv(t)
	r := e.claim(key("k"), `{"seat_ids":["A-R01-01"]}`)
	if got := e.claim(key("k"), `{"seat_ids":["A-R01-02"]}`); got.Outcome != Mismatch {
		t.Fatalf("while in progress: %+v", got)
	}
	if err := e.run(r.Claim, created); err != nil {
		t.Fatal(err)
	}
	if got := e.claim(key("k"), `{"seat_ids":["A-R01-02"]}`); got.Outcome != Mismatch {
		t.Fatalf("after completion: %+v", got)
	}
	// The mismatch changed nothing: the original replays, the effect ran once.
	if got := e.claim(key("k"), `{"seat_ids":["A-R01-01"]}`); got.Outcome != Replay {
		t.Fatalf("original request after mismatch: %+v", got)
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d", n)
	}
}

func TestConcurrentSameKeyExecutesOnce(t *testing.T) {
	e := newEnv(t)
	const n = 50
	var wg sync.WaitGroup
	results := make([]Outcome, n)
	var runErr sync.Map
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.store.Claim(context.Background(), key("k"), e.hash(`{"seat_ids":["A-R01-01"]}`))
			if err != nil {
				runErr.Store(i, err)
				return
			}
			results[i] = r.Outcome
			if r.Outcome == Proceed {
				if err := e.run(r.Claim, created); err != nil {
					runErr.Store(i, err)
				}
			}
		}()
	}
	wg.Wait()
	runErr.Range(func(k, v any) bool { t.Errorf("request %v: %v", k, v); return true })
	proceeded := 0
	for _, o := range results {
		switch o {
		case Proceed:
			proceeded++
		case InProgress, Replay:
		default:
			t.Fatalf("unexpected outcome %v", o)
		}
	}
	if proceeded != 1 || e.effects("k") != 1 {
		t.Fatalf("%d proceeded, effect ran %d times; want exactly 1", proceeded, e.effects("k"))
	}
}

func TestCrashBeforeCommit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	first := e.claim(key("k"), body).Claim

	// The owner's business transaction is open (fenced, effect written) when
	// its database session dies: nothing it did can commit.
	tx, err := e.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Fence(ctx, tx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO effects (idem_key, generation) VALUES ('k', 1)`); err != nil {
		t.Fatal(err)
	}
	var pid int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("commit after the session was terminated must fail")
	}

	if n := e.effects("k"); n != 0 {
		t.Fatalf("crashed transaction left %d effects", n)
	}
	// Within the lease: retry is told to wait.
	if got := e.claim(key("k"), body); got.Outcome != InProgress {
		t.Fatalf("retry within lease: %+v", got)
	}
	// After the lease: a retry takes over at generation 2 and executes once.
	e.age(&first, Lease+time.Second)
	second := e.claim(key("k"), body)
	if second.Outcome != Proceed || second.Claim.Generation != 2 {
		t.Fatalf("retry after lease: %+v", second)
	}
	if err := e.run(second.Claim, created); err != nil {
		t.Fatal(err)
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
	if err := e.run(first, created); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("the crashed generation-1 claim must be fenced out, got %v", err)
	}
}

func TestCrashAfterCommit(t *testing.T) {
	e := newEnv(t)
	body := `{"seat_ids":["A-R01-01"]}`
	r := e.claim(key("k"), body)
	if err := e.run(r.Claim, created); err != nil {
		t.Fatal(err)
	}
	// The process dies right after COMMIT, before replying (nothing else to do).
	// The client retries: the committed result is returned, nothing re-executes.
	for range 3 {
		got := e.claim(key("k"), body)
		if got.Outcome != Replay || got.Response.Code != 201 {
			t.Fatalf("retry after commit: %+v", got)
		}
		sameJSON(t, got.Response.Body, created.Body)
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

// The commit succeeds but the caller's context is cancelled before it can
// use the result (the response never reaches the client). A retry receives
// the original committed result.
func TestCommitSucceedsButResponseLost(t *testing.T) {
	e := newEnv(t)
	body := `{"seat_ids":["A-R01-01","A-R01-02"]}`
	r := e.claim(key("k"), body)
	reqCtx, cancel := context.WithCancel(context.Background())
	tx, err := e.begin(reqCtx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Fence(reqCtx, tx, r.Claim); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(reqCtx, `INSERT INTO effects (idem_key, generation) VALUES ('k', $1)`, r.Claim.Generation); err != nil {
		t.Fatal(err)
	}
	if err := Complete(reqCtx, tx, r.Claim, created); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(reqCtx); err != nil {
		t.Fatal(err)
	}
	cancel() // the client connection drops: the 201 is lost in flight

	got := e.claim(key("k"), body)
	if got.Outcome != Replay || got.Response.Code != 201 {
		t.Fatalf("retry: %+v", got)
	}
	sameJSON(t, got.Response.Body, created.Body)
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

func TestLeaseExpiryAndReclaimByNewGeneration(t *testing.T) {
	e := newEnv(t)
	body := `{"seat_ids":["A-R01-01"]}`
	g1 := e.claim(key("k"), body)
	e.age(&g1.Claim, Lease-time.Second) // 29 s: still live
	if got := e.claim(key("k"), body); got.Outcome != InProgress {
		t.Fatalf("at 29 s: %+v", got)
	}
	e.age(&g1.Claim, 2*time.Second) // now 31 s: stale
	g2 := e.claim(key("k"), body)
	if g2.Outcome != Proceed || g2.Claim.Generation != g1.Claim.Generation+1 {
		t.Fatalf("takeover: %+v", g2)
	}
	if got := e.claim(key("k"), body); got.Outcome != InProgress {
		t.Fatalf("the new claim has a fresh lease: %+v", got)
	}
	if got := e.claim(key("k"), `{"seat_ids":["A-R09-09"]}`); got.Outcome != Mismatch {
		t.Fatalf("a takeover is only for the same request: %+v", got)
	}
}

func TestStaleClaimantCannotCompleteOverwriteOrDelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	old := e.claim(key("k"), body).Claim
	e.age(&old, Lease+time.Second) // old's token is exact: only the generation fences it out
	fresh := e.claim(key("k"), body).Claim
	if fresh.Generation != old.Generation+1 {
		t.Fatalf("generations %d -> %d", old.Generation, fresh.Generation)
	}

	// The old claimant wakes up: every write path is fenced out.
	if err := e.run(old, Response{Code: 201, Body: json.RawMessage(`{"hold_id":"OLD"}`)}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("old claimant's run: %v", err)
	}
	tx, _ := e.begin(ctx)
	if err := Complete(ctx, tx, old, created); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("old Complete without Fence: %v", err)
	}
	_ = tx.Rollback(ctx)
	if abandoned, err := e.store.Abandon(ctx, old); err != nil || abandoned {
		t.Fatalf("old Abandon abandoned=%v err=%v; must not touch the newer claim", abandoned, err)
	}
	if n := e.effects("k"); n != 0 {
		t.Fatalf("the fenced-out claimant produced %d effects", n)
	}

	// The newer claimant completes normally, and the old one still cannot touch it.
	if err := e.run(fresh, created); err != nil {
		t.Fatal(err)
	}
	if err := e.run(old, Response{Code: 201, Body: json.RawMessage(`{"hold_id":"OLD"}`)}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("old claimant after the new one completed: %v", err)
	}
	if abandoned, _ := e.store.Abandon(ctx, old); abandoned {
		t.Fatal("old Abandon touched a completed record")
	}
	status, gen, code, stored := e.record(key("k"))
	if status != "COMPLETED" || gen != fresh.Generation || code == nil || *code != 201 {
		t.Fatalf("record = %s gen %d code %v", status, gen, code)
	}
	sameJSON(t, stored, created.Body)
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

// A live owner inside its business transaction holds the row lock from Fence.
// A takeover attempted meanwhile (even with an aged lease) waits for it, and
// then replays instead of re-executing.
func TestTakeoverWaitsForOwnerInsideTransaction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	owner := e.claim(key("k"), body).Claim
	// Make the lease look stale BEFORE the owner fences: aging the row is
	// itself an UPDATE and would wait on the owner's row lock.
	e.age(&owner, Lease+time.Second)
	tx, err := e.begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Fence(ctx, tx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO effects (idem_key, generation) VALUES ('k', $1)`, owner.Generation); err != nil {
		t.Fatal(err)
	}
	// The lease looks stale, but the owner is alive and holds the row lock.

	cfg := e.pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["application_name"] = "retrier"
	retrierPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer retrierPool.Close()
	done := make(chan Result, 1)
	go func() {
		r, err := New(retrierPool).Claim(ctx, key("k"), e.hash(body))
		if err != nil {
			t.Error(err)
		}
		done <- r
	}()
	// Observed (not timed): the retrier is blocked on the owner's row lock.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting bool
		_ = e.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE application_name = 'retrier' AND wait_event_type = 'Lock')`).Scan(&waiting)
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retrier never blocked on the owner's lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case r := <-done:
		t.Fatalf("retrier finished while the owner held the lock: %+v", r)
	default:
	}
	if err := Complete(ctx, tx, owner, created); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.Outcome != Replay || r.Response.Code != 201 {
		t.Fatalf("retrier after the owner committed: %+v", r)
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

// Completion and the effect commit together or not at all.
func TestCompletionAndEffectAreAtomic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	c := e.claim(key("k"), body).Claim
	tx, _ := e.begin(ctx)
	if err := Fence(ctx, tx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO effects (idem_key, generation) VALUES ('k', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := Complete(ctx, tx, c, created); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := e.record(key("k")); status != "IN_PROGRESS" {
		t.Fatalf("rolled-back completion persisted: %s", status)
	}
	if n := e.effects("k"); n != 0 {
		t.Fatalf("rolled-back effect persisted: %d", n)
	}
	// The claim is still owned and can be completed properly.
	if err := e.run(c, created); err != nil {
		t.Fatal(err)
	}
}

func TestAbandonIsFencedAndAllowsImmediateRetry(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	c := e.claim(key("k"), body).Claim
	abandoned, err := e.store.Abandon(ctx, c)
	if err != nil || !abandoned {
		t.Fatalf("abandon own claim: abandoned=%v err=%v", abandoned, err)
	}
	// The row is kept (never deleted, so generations are never reused) ...
	if status, gen, _, _ := e.record(key("k")); status != "IN_PROGRESS" || gen != c.Generation+1 {
		t.Fatalf("after abandon: %s gen %d", status, gen)
	}
	// ... and a retry takes over at once, without waiting out the 30 s lease.
	retry := e.claim(key("k"), body)
	if retry.Outcome != Proceed || retry.Claim.Generation != c.Generation+2 {
		t.Fatalf("retry after abandon: %+v", retry)
	}
	// The key stays bound to its request: a different body is a mismatch.
	if got := e.claim(key("k"), `{"seat_ids":["A-R09-09"]}`); got.Outcome != Mismatch {
		t.Fatalf("different body after abandon: %+v", got)
	}
	// The abandoned token can no longer touch the new claim in any way.
	if again, _ := e.store.Abandon(ctx, c); again {
		t.Fatal("re-abandoning a stale token invalidated the new claim")
	}
	if err := e.run(c, created); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("stale token completed: %v", err)
	}
	if err := e.run(retry.Claim, created); err != nil {
		t.Fatal(err)
	}
	if done, _ := e.store.Abandon(ctx, retry.Claim); done {
		t.Fatal("Abandon touched a completed record")
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

// The hole found during T5: if a row is deleted (retention cleanup) and the
// key reused, the new row starts at generation 1 again. The token's
// claimed_at must still fence the old claimant out.
func TestTokenSurvivesRowDeletionAndKeyReuse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	old := e.claim(key("k"), body).Claim
	if _, err := e.pool.Exec(ctx, `UPDATE idempotency_keys SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if n, err := e.store.Cleanup(ctx, 10); err != nil || n != 1 {
		t.Fatalf("cleanup: %d %v", n, err)
	}
	fresh := e.claim(key("k"), body).Claim
	if fresh.Generation != old.Generation || fresh.Generation != 1 {
		t.Fatalf("precondition: the reused key restarts at generation 1 (old %d, fresh %d)", old.Generation, fresh.Generation)
	}
	if fresh.ClaimedAt.Equal(old.ClaimedAt) {
		t.Fatal("precondition: claimed_at must differ between the two claims")
	}
	if err := e.run(old, created); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("old token with the same generation completed the new claim: %v", err)
	}
	if abandoned, _ := e.store.Abandon(ctx, old); abandoned {
		t.Fatal("old token with the same generation abandoned the new claim")
	}
	if err := e.run(fresh, created); err != nil {
		t.Fatalf("the new owner must still complete: %v", err)
	}
	if n := e.effects("k"); n != 1 {
		t.Fatalf("effects = %d, want 1", n)
	}
}

func TestExpiredKeyIsReusedAsNewRequest(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old := e.claim(key("k"), `{"seat_ids":["A-R01-01"]}`)
	if err := e.run(old.Claim, created); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE idempotency_keys SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	// Past retention the record no longer counts, even for a different body.
	r := e.claim(key("k"), `{"seat_ids":["A-R05-05"]}`)
	if r.Outcome != Proceed || r.Claim.Generation != old.Claim.Generation+1 {
		t.Fatalf("reuse after retention: %+v", r)
	}
	var keep bool
	if err := e.pool.QueryRow(ctx, `SELECT expires_at > now() + interval '23 hours 59 minutes' FROM idempotency_keys`).Scan(&keep); err != nil || !keep {
		t.Fatalf("the reused record must get a fresh 24 h retention (err %v)", err)
	}
}

func TestCleanupDeletesOnlyExpiredRecords(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	body := `{"seat_ids":["A-R01-01"]}`
	done := e.claim(key("done"), body)
	if err := e.run(done.Claim, created); err != nil {
		t.Fatal(err)
	}
	e.claim(key("busy"), body)
	for _, k := range []string{"old1", "old2", "oldlocked"} {
		e.claim(key(k), body)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE idempotency_keys SET expires_at = now() - interval '1 second' WHERE key LIKE 'old%'`); err != nil {
		t.Fatal(err)
	}
	// One expired record is row-locked by an in-flight transaction: skipped, not waited for.
	lock, _ := e.begin(ctx)
	if _, err := lock.Exec(ctx, `SELECT 1 FROM idempotency_keys WHERE key = 'oldlocked' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	n, err := e.store.Cleanup(ctx, 100)
	if err != nil || n != 2 {
		t.Fatalf("cleanup deleted %d (err %v), want the 2 unlocked expired records", n, err)
	}
	_ = lock.Rollback(ctx)
	if n, _ := e.store.Cleanup(ctx, 100); n != 1 {
		t.Fatalf("second cleanup deleted %d, want the previously locked one", n)
	}
	var left []string
	rows, _ := e.pool.Query(ctx, `SELECT key FROM idempotency_keys ORDER BY key`)
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		left = append(left, k)
	}
	rows.Close()
	if len(left) != 2 || left[0] != "busy" || left[1] != "done" {
		t.Fatalf("unexpired records must survive cleanup; left = %v", left)
	}
	// Exactly-at-boundary: a record expiring later is never touched.
	if got := e.claim(key("done"), body); got.Outcome != Replay {
		t.Fatalf("unexpired completed record lost: %+v", got)
	}
	if _, err := e.store.Cleanup(ctx, 0); err == nil {
		t.Fatal("a non-positive limit must be rejected")
	}
}

func TestCompleteRejectsInvalidResponses(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c := e.claim(key("k"), `{"a":1}`).Claim
	tx, _ := e.begin(ctx)
	defer func() { _ = tx.Rollback(ctx) }()
	if err := Fence(ctx, tx, c); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Response{{Code: 0, Body: json.RawMessage(`{}`)}, {Code: 700, Body: json.RawMessage(`{}`)}, {Code: 201, Body: json.RawMessage(`{bad`)}} {
		if err := Complete(ctx, tx, c, r); err == nil || errors.Is(err, ErrNotOwner) {
			t.Fatalf("invalid response %+v accepted or misreported: %v", r, err)
		}
	}
}

func TestClaimValidatesInput(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	h := e.hash(`{}`)
	for _, k := range []Key{{Scope: "", UserID: key("k").UserID, Key: "k"}, {Scope: scope, UserID: "not-a-uuid", Key: "k"}, {Scope: scope, UserID: key("k").UserID, Key: ""}} {
		if _, err := e.store.Claim(ctx, k, h); err == nil {
			t.Errorf("invalid key %+v accepted", k)
		}
	}
	if _, err := e.store.Claim(ctx, key("k"), nil); err == nil {
		t.Error("empty hash accepted")
	}
	// Different users never share a key.
	other := key("shared")
	other.UserID = "22222222-2222-4222-8222-222222222222"
	if e.claim(key("shared"), `{"a":1}`).Outcome != Proceed || e.claim(other, `{"a":2}`).Outcome != Proceed {
		t.Fatal("the same key for two users must be independent")
	}
}
