//go:build integration

package expiry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/booking/holds"
	"github.com/Varsh1l01/SurgeTix/internal/booking/inventory"
	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const (
	eventID = "5a4b3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d"
	userA   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

var keySeq atomic.Int64

type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	svc  *holds.Service
}

// newEnv: real schema, generated inventory (sections A 2x10, B 1x10, C
// 10x50), and the T10 hold service to create holds the production way.
func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	small := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(ctx, small, nil); err != nil {
		t.Fatal(err)
	}
	cfg := small.Config().Copy()
	cfg.MaxConns = 32
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	inv, err := inventory.Generate(inventory.Layout{EventID: eventID, Seats: []inventory.SeatSection{
		{Name: "A", Rows: 2, SeatsPerRow: 10, PricePaise: 100},
		{Name: "B", Rows: 1, SeatsPerRow: 10, PricePaise: 100},
		{Name: "C", Rows: 10, SeatsPerRow: 50, PricePaise: 100},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Load(ctx, pool, inv); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, pool: pool, svc: holds.NewService(pool)}
}

// hold creates a hold through the T10 service and returns its id.
func (e *env) hold(ids ...string) string {
	e.t.Helper()
	body, _ := json.Marshal(map[string]any{"seat_ids": ids})
	r, err := e.svc.Hold(context.Background(), userA, fmt.Sprintf("k-%d", keySeq.Add(1)), eventID, body)
	if err != nil || r.Status != http.StatusCreated {
		e.t.Fatalf("hold %v: %v %d %s", ids, err, r.Status, r.Body)
	}
	var c holds.Created
	_ = json.Unmarshal(r.Body, &c)
	return c.HoldID
}

// expire moves a hold and its seats' mirror into the past together.
func (e *env) expire(id string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `UPDATE holds SET expires_at = now() - interval '1 minute' WHERE hold_id = $1`, id); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE seats s SET hold_expires_at = h.expires_at FROM holds h
		WHERE h.hold_id = $1 AND s.hold_id = h.hold_id AND s.status = 'HELD'`, id); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) worker(cfg Config) (*Worker, *prometheus.Registry) {
	e.t.Helper()
	reg := prometheus.NewRegistry()
	w, err := New(e.pool, reg, cfg)
	if err != nil {
		e.t.Fatal(err)
	}
	return w, reg
}

func (e *env) runOnce(w *Worker) holds.ExpireResult {
	e.t.Helper()
	r, err := w.RunOnce(context.Background())
	if err != nil {
		e.t.Fatalf("RunOnce: %v", err)
	}
	return r
}

func (e *env) status(id string) string {
	e.t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM holds WHERE hold_id = $1`, id).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) seat(id string) (status, holdID string) {
	e.t.Helper()
	var h *string
	if err := e.pool.QueryRow(context.Background(), `SELECT status, hold_id::text FROM seats WHERE event_id = $1 AND seat_id = $2`,
		eventID, id).Scan(&status, &h); err != nil {
		e.t.Fatal(err)
	}
	if h != nil {
		holdID = *h
	}
	return
}

func (e *env) expectSeat(id, status, holdID string) {
	e.t.Helper()
	s, h := e.seat(id)
	if s != status || h != holdID {
		e.t.Fatalf("seat %s: %s/%q, want %s/%q", id, s, h, status, holdID)
	}
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (e *env) audit() {
	e.t.Helper()
	rep, err := inventory.Audit(context.Background(), e.pool)
	if err != nil {
		e.t.Fatal(err)
	}
	if !rep.Clean() {
		e.t.Fatalf("audit not clean:\n%s", rep)
	}
}

type expiryEvents struct {
	expired  map[string]int // hold -> HoldExpired count
	returned map[string]int // seat -> times returned with source "expired"
}

// events reads the worker's events (HoldExpired, and InventoryReturned
// with source "expired").
func (e *env) events() expiryEvents {
	e.t.Helper()
	out := expiryEvents{expired: map[string]int{}, returned: map[string]int{}}
	rows, err := e.pool.Query(context.Background(), `SELECT topic, key, payload FROM outbox`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var topic, key string
		var payload []byte
		if err := rows.Scan(&topic, &key, &payload); err != nil {
			e.t.Fatal(err)
		}
		var env struct {
			Type string          `json:"event_type"`
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(payload, &env)
		switch env.Type {
		case outbox.HoldExpired:
			var d struct {
				HoldID  string `json:"hold_id"`
				EventID string `json:"event_id"`
				Reason  string `json:"reason"`
			}
			_ = json.Unmarshal(env.Data, &d)
			if topic != outbox.TopicHolds || key != d.HoldID || d.EventID != eventID || d.Reason != "expired" {
				e.t.Fatalf("HoldExpired %s %s %+v", topic, key, d)
			}
			out.expired[d.HoldID]++
		case outbox.InventoryReturned:
			var d struct {
				EventID string   `json:"event_id"`
				Section string   `json:"section"`
				Source  string   `json:"source"`
				Seats   []string `json:"seats"`
			}
			_ = json.Unmarshal(env.Data, &d)
			if d.Source != "expired" {
				continue // inline reclaims ("reclaimed") are T9's
			}
			if topic != outbox.TopicInventory || key != d.EventID+":"+d.Section {
				e.t.Fatalf("InventoryReturned %s %s %+v", topic, key, d)
			}
			for _, s := range d.Seats {
				out.returned[s]++
			}
		}
	}
	return out
}

func metric(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		m := mf.GetMetric()[0]
		if c := m.GetCounter(); c != nil {
			return c.GetValue()
		}
		return m.GetGauge().GetValue()
	}
	t.Fatalf("metric %s not registered", name)
	return 0
}

// lockTx opens a transaction holding row locks taken by sql (rolled back
// by the returned function, or at test end).
func (e *env) lockTx(sql string, args ...any) func() {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if n == 0 {
		e.t.Fatalf("lock query locked nothing: %s", sql)
	}
	done := false
	release := func() {
		if !done {
			done = true
			_ = tx.Rollback(context.Background())
		}
	}
	e.t.Cleanup(release)
	return release
}

func (e *env) waitBlocked(n int) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var got int
		_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&got)
		if got >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %d blocked sessions", n)
}

// TestRunOnceExpiresDueHolds: only expired holds are processed; their seats
// return once with the right events; metrics count the batch.
func TestRunOnceExpiresDueHolds(t *testing.T) {
	e := newEnv(t)
	h1 := e.hold("A-R01-01", "B-R01-01")
	h2 := e.hold("A-R01-02")
	live := e.hold("A-R01-03")
	e.expire(h1)
	e.expire(h2)
	w, reg := e.worker(Config{})
	r := e.runOnce(w)
	if r.Candidates != 2 || r.Released != 3 || r.Expired != 2 || r.Partial != 0 {
		t.Fatalf("result %+v", r)
	}
	for _, id := range []string{h1, h2} {
		if e.status(id) != "EXPIRED" {
			t.Fatalf("hold %s %s", id, e.status(id))
		}
	}
	e.expectSeat("A-R01-01", "AVAILABLE", "")
	e.expectSeat("B-R01-01", "AVAILABLE", "")
	e.expectSeat("A-R01-02", "AVAILABLE", "")
	e.expectSeat("A-R01-03", "HELD", live)
	if e.status(live) != "ACTIVE" {
		t.Fatal("live hold touched")
	}
	ev := e.events()
	if len(ev.expired) != 2 || ev.expired[h1] != 1 || ev.expired[h2] != 1 {
		t.Fatalf("HoldExpired %v", ev.expired)
	}
	if len(ev.returned) != 3 || ev.returned["A-R01-01"] != 1 || ev.returned["B-R01-01"] != 1 || ev.returned["A-R01-02"] != 1 {
		t.Fatalf("returned %v", ev.returned)
	}
	if n := e.count(`SELECT count(*) FROM outbox WHERE convert_from(payload,'UTF8')::jsonb->>'event_type' = 'InventoryReturned'
		AND convert_from(payload,'UTF8')::jsonb->'data'->>'source' = 'expired'`); n != 2 {
		t.Fatalf("InventoryReturned %d, want one per section (A, B)", n)
	}
	if metric(t, reg, "booking_expiry_batches_total") != 1 || metric(t, reg, "booking_holds_expired_total") != 2 ||
		metric(t, reg, "booking_expiry_partial_total") != 0 || metric(t, reg, "booking_expiry_lag_seconds") != 0 {
		t.Fatal("metrics wrong")
	}
	e.audit()
}

// E1 (and liveness): a hold whose row another transaction locks is skipped
// without waiting, and processed by the next run once the lock is gone.
func TestE1LockedHoldSkippedThenProcessed(t *testing.T) {
	e := newEnv(t)
	h := e.hold("A-R01-04", "A-R01-05")
	other := e.hold("A-R01-06")
	e.expire(h)
	e.expire(other)
	release := e.lockTx(`SELECT hold_id FROM holds WHERE hold_id = $1 FOR UPDATE`, h)
	w, reg := e.worker(Config{})
	start := time.Now()
	r := e.runOnce(w)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("worker waited %v on a locked hold", d)
	}
	if r.Candidates != 1 || r.Expired != 1 || e.status(other) != "EXPIRED" {
		t.Fatalf("first run %+v", r)
	}
	if e.status(h) != "ACTIVE" {
		t.Fatal("locked hold processed")
	}
	e.expectSeat("A-R01-04", "HELD", h)
	if lag := metric(t, reg, "booking_expiry_lag_seconds"); lag < 50 {
		t.Fatalf("lag %v: the skipped hold (expired 1 min ago) must show", lag)
	}
	e.audit()
	release()
	r = e.runOnce(w)
	if r.Candidates != 1 || r.Expired != 1 || r.Released != 2 || e.status(h) != "EXPIRED" {
		t.Fatalf("second run %+v", r)
	}
	if metric(t, reg, "booking_expiry_lag_seconds") != 0 {
		t.Fatal("lag not cleared")
	}
	e.audit()
}

// E2: one seat is locked by an in-flight transaction. The worker returns the
// others, leaves the hold live (partial, no orphan), and finishes it later.
func TestE2LockedSeatPartialThenFinished(t *testing.T) {
	e := newEnv(t)
	h := e.hold("A-R01-07", "A-R01-08", "A-R01-09")
	e.expire(h)
	release := e.lockTx(`SELECT seat_id FROM seats WHERE event_id = $1 AND seat_id = 'A-R01-08' FOR UPDATE`, eventID)
	w, reg := e.worker(Config{})
	r := e.runOnce(w)
	if r.Candidates != 1 || r.Released != 2 || r.Expired != 0 || r.Partial != 1 {
		t.Fatalf("partial run %+v", r)
	}
	e.expectSeat("A-R01-07", "AVAILABLE", "")
	e.expectSeat("A-R01-08", "HELD", h)
	e.expectSeat("A-R01-09", "AVAILABLE", "")
	if e.status(h) != "ACTIVE" || metric(t, reg, "booking_expiry_partial_total") != 1 {
		t.Fatal("hold not left live for the next run")
	}
	e.audit() // no orphan: the HELD seat's hold is still ACTIVE
	release()
	r = e.runOnce(w)
	if r.Released != 1 || r.Expired != 1 || e.status(h) != "EXPIRED" {
		t.Fatalf("finishing run %+v", r)
	}
	ev := e.events()
	for _, s := range []string{"A-R01-07", "A-R01-08", "A-R01-09"} {
		if ev.returned[s] != 1 {
			t.Fatalf("seat %s returned %d times", s, ev.returned[s])
		}
	}
	if ev.expired[h] != 1 {
		t.Fatalf("HoldExpired %d", ev.expired[h])
	}
	e.audit()
}

// E4: two workers run concurrently over 150 expired holds with small,
// overlapping batches. Every hold is expired exactly once, every seat is
// returned exactly once, and both workers do work.
func TestE4TwoWorkersDisjoint(t *testing.T) {
	e := newEnv(t)
	const n = 150
	ids := make([]string, n)
	seats := 0
	for i := 0; i < n; i++ {
		row, base := i/25+1, (i%25)*2+1
		if i%3 == 0 {
			ids[i] = e.hold(fmt.Sprintf("C-R%02d-%02d", row, base))
			seats++
		} else {
			ids[i] = e.hold(fmt.Sprintf("C-R%02d-%02d", row, base), fmt.Sprintf("C-R%02d-%02d", row, base+1))
			seats += 2
		}
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE holds SET expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE seats s SET hold_expires_at = h.expires_at FROM holds h WHERE s.hold_id = h.hold_id AND s.status = 'HELD'`); err != nil {
		t.Fatal(err)
	}
	var totals [2]holds.ExpireResult
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for k := 0; k < 2; k++ {
		w, _ := e.worker(Config{BatchSize: 20})
		// Hold each batch's locks a little, so the two workers overlap.
		w.beforeCommit = func(context.Context, pgx.Tx) error { time.Sleep(15 * time.Millisecond); return nil }
		wg.Add(1)
		go func(k int, w *Worker) {
			defer wg.Done()
			for empty := 0; empty < 3; {
				r, err := w.RunOnce(context.Background())
				if err != nil {
					errs[k] = err
					return
				}
				if r.Candidates == 0 {
					empty++
					time.Sleep(5 * time.Millisecond)
					continue
				}
				totals[k].Candidates += r.Candidates
				totals[k].Released += r.Released
				totals[k].Expired += r.Expired
			}
		}(k, w)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if totals[0].Expired+totals[1].Expired != n || totals[0].Released+totals[1].Released != seats {
		t.Fatalf("totals %+v %+v, want %d holds and %d seats", totals[0], totals[1], n, seats)
	}
	if totals[0].Expired == 0 || totals[1].Expired == 0 {
		t.Fatalf("one worker did all the work: %+v %+v", totals[0], totals[1])
	}
	ev := e.events()
	if len(ev.expired) != n {
		t.Fatalf("HoldExpired for %d holds", len(ev.expired))
	}
	for id, c := range ev.expired {
		if c != 1 {
			t.Fatalf("hold %s expired %d times", id, c)
		}
	}
	if len(ev.returned) != seats {
		t.Fatalf("%d seats returned, want %d", len(ev.returned), seats)
	}
	for s, c := range ev.returned {
		if c != 1 {
			t.Fatalf("seat %s returned %d times", s, c)
		}
	}
	if e.count(`SELECT count(*) FROM holds WHERE status <> 'EXPIRED'`) != 0 || e.count(`SELECT count(*) FROM seats WHERE status = 'HELD'`) != 0 {
		t.Fatal("work left undone")
	}
	t.Logf("worker 1: %d holds, worker 2: %d holds", totals[0].Expired, totals[1].Expired)
	e.audit()
}

// E5: the worker's session is killed mid-batch (after its updates, before
// COMMIT). Nothing changes; the rerun does the work exactly once.
func TestE5KilledMidBatchRerunsOnce(t *testing.T) {
	e := newEnv(t)
	h := e.hold("B-R01-02", "B-R01-03")
	e.expire(h)
	w, _ := e.worker(Config{})
	w.beforeCommit = func(ctx context.Context, tx pgx.Tx) error {
		var pid int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		if _, err := e.pool.Exec(context.Background(), `SELECT pg_terminate_backend($1)`, pid); err != nil {
			return err
		}
		return nil // COMMIT now fails: the session is gone
	}
	if _, err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("killed batch reported success")
	}
	if e.status(h) != "ACTIVE" {
		t.Fatal("killed batch left changes")
	}
	e.expectSeat("B-R01-02", "HELD", h)
	if ev := e.events(); len(ev.expired)+len(ev.returned) != 0 {
		t.Fatal("killed batch left events")
	}
	e.audit()
	w.beforeCommit = nil
	r := e.runOnce(w)
	if r.Expired != 1 || r.Released != 2 {
		t.Fatalf("rerun %+v", r)
	}
	ev := e.events()
	if ev.expired[h] != 1 || ev.returned["B-R01-02"] != 1 || ev.returned["B-R01-03"] != 1 {
		t.Fatalf("events after rerun %+v", ev)
	}
	e.audit()
}

// E6: rerunning after everything is done is a no-op.
func TestE6RerunIsNoOp(t *testing.T) {
	e := newEnv(t)
	h := e.hold("B-R01-04")
	e.expire(h)
	w, reg := e.worker(Config{})
	e.runOnce(w)
	before := e.count(`SELECT count(*) FROM outbox`)
	for i := 0; i < 3; i++ {
		if r := e.runOnce(w); r != (holds.ExpireResult{}) {
			t.Fatalf("rerun %d: %+v", i, r)
		}
	}
	if e.count(`SELECT count(*) FROM outbox`) != before {
		t.Fatal("rerun wrote events")
	}
	if metric(t, reg, "booking_holds_expired_total") != 1 || metric(t, reg, "booking_expiry_batches_total") != 4 {
		t.Fatal("metrics after reruns")
	}
	e.audit()
}

// A hold an inline reclaim left live with no seats (T9, P4) is finished by
// the worker: EXPIRED, HoldExpired, nothing returned.
func TestSeatlessExpiredHoldFinished(t *testing.T) {
	e := newEnv(t)
	h := e.hold("B-R01-05")
	e.expire(h)
	// Its seat was taken elsewhere: simulate with test SQL.
	if _, err := e.pool.Exec(context.Background(), `UPDATE seats SET status = 'AVAILABLE', hold_id = NULL, hold_expires_at = NULL
		WHERE event_id = $1 AND seat_id = 'B-R01-05'`, eventID); err != nil {
		t.Fatal(err)
	}
	w, _ := e.worker(Config{})
	r := e.runOnce(w)
	if r.Expired != 1 || r.Released != 0 || e.status(h) != "EXPIRED" {
		t.Fatalf("result %+v", r)
	}
	if ev := e.events(); ev.expired[h] != 1 || len(ev.returned) != 0 {
		t.Fatalf("events %+v", ev)
	}
	e.audit()
}

// GA holds are M3: the M1 worker leaves them alone.
func TestGAHoldsIgnoredUntilM3(t *testing.T) {
	e := newEnv(t)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO holds (hold_id, event_id, user_id, kind, status, expires_at)
		VALUES (gen_random_uuid(), $1, $2, 'GA', 'ACTIVE', now() - interval '1 minute')`, eventID, userA); err != nil {
		t.Fatal(err)
	}
	w, _ := e.worker(Config{})
	if r := e.runOnce(w); r.Candidates != 0 {
		t.Fatalf("GA hold processed: %+v", r)
	}
}

// L1 against the real worker: the worker has locked an expired hold and
// released its seats but not yet committed; a buyer wants one of those
// seats. The buyer waits on the seat (rank 2, allowed), its inline reclaim
// skips the hold row the worker owns, and when the worker commits the buyer
// takes the seat as AVAILABLE. No deadlock; the hold is expired once.
func TestL1WorkerVersusBuyer(t *testing.T) {
	e := newEnv(t)
	h := e.hold("A-R02-01", "A-R02-02")
	e.expire(h)
	w, _ := e.worker(Config{})
	paused, resume := make(chan struct{}), make(chan struct{})
	w.beforeCommit = func(context.Context, pgx.Tx) error { close(paused); <-resume; return nil }
	wDone := make(chan error, 1)
	go func() { _, err := w.RunOnce(context.Background()); wDone <- err }()
	<-paused

	body, _ := json.Marshal(map[string]any{"seat_ids": []string{"A-R02-01"}})
	key := fmt.Sprintf("k-%d", keySeq.Add(1))
	bDone := make(chan holds.Response, 1)
	go func() { r, _ := e.svc.Hold(context.Background(), userA, key, eventID, body); bDone <- r }()
	e.waitBlocked(1) // the buyer waits on the seat the worker updated
	close(resume)
	if err := <-wDone; err != nil {
		t.Fatalf("worker: %v", err)
	}
	r := <-bDone
	if r.Status == http.StatusServiceUnavailable {
		// The 500 ms lock timeout fired before the worker committed (a slow
		// machine). Nothing was stored; the client's retry must succeed.
		t.Logf("buyer timed out first (%v); retrying with the same key", r.Cause)
		r, _ = e.svc.Hold(context.Background(), userA, key, eventID, body)
	}
	if r.Status != http.StatusCreated {
		t.Fatalf("buyer: %d %s (cause %v)", r.Status, r.Body, r.Cause)
	}
	var c holds.Created
	_ = json.Unmarshal(r.Body, &c)
	e.expectSeat("A-R02-01", "HELD", c.HoldID)
	e.expectSeat("A-R02-02", "AVAILABLE", "")
	if e.status(h) != "EXPIRED" || e.events().expired[h] != 1 {
		t.Fatal("old hold not expired exactly once")
	}
	e.audit()
}

// Pacing: while batches are full the worker runs again at once (the 1 h
// interval is never waited); it stops promptly when its context ends.
func TestRunBackToBackWhileFullThenStops(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= 7; i++ {
		e.expire(e.hold(fmt.Sprintf("C-R10-%02d", i)))
	}
	w, reg := e.worker(Config{BatchSize: 3, Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	deadline := time.Now().Add(20 * time.Second)
	for e.count(`SELECT count(*) FROM holds WHERE status = 'EXPIRED'`) < 7 {
		if time.Now().After(deadline) {
			t.Fatal("full batches were not run back to back")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if b := metric(t, reg, "booking_expiry_batches_total"); b != 3 {
		t.Fatalf("batches %v, want 3 (3+3+1)", b)
	}
	e.audit()
}

// Graceful stop: an in-flight batch still commits within ShutdownTimeout;
// past it, the batch is cancelled and rolls back completely.
func TestGracefulStop(t *testing.T) {
	e := newEnv(t)
	h1 := e.hold("C-R09-01")
	e.expire(h1)

	w, _ := e.worker(Config{Interval: time.Hour, ShutdownTimeout: 5 * time.Second})
	paused, resume := make(chan struct{}), make(chan struct{})
	w.beforeCommit = func(context.Context, pgx.Tx) error { close(paused); <-resume; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	<-paused
	cancel() // stop signal arrives mid-batch
	time.Sleep(100 * time.Millisecond)
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if e.status(h1) != "EXPIRED" {
		t.Fatal("in-flight batch did not commit within the shutdown timeout")
	}

	h2 := e.hold("C-R09-02")
	e.expire(h2)
	w2, _ := e.worker(Config{Interval: time.Hour, ShutdownTimeout: 200 * time.Millisecond})
	paused2 := make(chan struct{})
	w2.beforeCommit = func(bctx context.Context, _ pgx.Tx) error {
		close(paused2)
		<-bctx.Done() // stuck until the shutdown timeout cancels it
		return bctx.Err()
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan error, 1)
	go func() { done2 <- w2.Run(ctx2) }()
	<-paused2
	cancel2()
	select {
	case err := <-done2:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stuck batch not cancelled after the shutdown timeout")
	}
	if e.status(h2) != "ACTIVE" {
		t.Fatal("cancelled batch left changes")
	}
	e.expectSeat("C-R09-02", "HELD", h2)
	e.audit()
}

var _ = errors.New
