//go:build integration

package expiry

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/booking/holds"
)

// TestL6RandomizedStress (plan §13, L6): thousands of mixed operations on a
// small, contended seat pool:
//   - new holds of 1-4 random seats (inline reclaim of expired seats included),
//   - retries of earlier keys (must return the original result),
//   - holds aged into expiry (as the clock would), and
//   - two expiry workers running continuously.
//
// Afterwards: zero deadlocks (40P01) anywhere, replays consistent, every
// worker batch succeeded, and the audit is clean once the workers drain.
func TestL6RandomizedStress(t *testing.T) {
	e := newEnv(t)
	seats := make([]string, 0, 30)
	for i := 1; i <= 10; i++ {
		seats = append(seats, fmt.Sprintf("A-R01-%02d", i), fmt.Sprintf("A-R02-%02d", i), fmt.Sprintf("B-R01-%02d", i))
	}

	const buyers, opsPerBuyer = 12, 170 // ~2,000 operations
	var (
		deadlocks, timeouts, workerErrs atomic.Int64
		created, refused, replays       atomic.Int64
		agedCount                       atomic.Int64
		mu                              sync.Mutex
		first                           = map[string]holds.Response{} // key -> first committed answer
		bodies                          = map[string][]byte{}
	)

	stopWorkers := make(chan struct{})
	var wwg sync.WaitGroup
	for k := 0; k < 2; k++ {
		w, _ := e.worker(Config{BatchSize: 5})
		wwg.Add(1)
		go func() {
			defer wwg.Done()
			for {
				select {
				case <-stopWorkers:
					return
				default:
				}
				if _, err := w.RunOnce(context.Background()); err != nil {
					workerErrs.Add(1)
					t.Logf("worker: %v (sqlstate %s)", err, sqlState(err))
					if sqlState(err) == "40P01" {
						deadlocks.Add(1)
					}
				}
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}()
	}

	var bwg sync.WaitGroup
	for b := 0; b < buyers; b++ {
		bwg.Add(1)
		go func(b int) {
			defer bwg.Done()
			rng := rand.New(rand.NewSource(int64(1000 + b)))
			var mine []string
			for op := 0; op < opsPerBuyer; op++ {
				switch p := rng.Intn(100); {
				case p < 15 && len(mine) > 0: // retry an earlier key
					key := mine[rng.Intn(len(mine))]
					mu.Lock()
					body := bodies[key]
					mu.Unlock()
					r, _ := e.svc.Hold(context.Background(), userA, key, eventID, body)
					check(t, r, &deadlocks, &timeouts)
					mu.Lock()
					orig, done := first[key]
					mu.Unlock()
					if r.Replayed {
						replays.Add(1)
						if done && (r.Status != orig.Status || holdOf(r) != holdOf(orig)) {
							t.Errorf("key %s replayed %d/%s, first answer %d/%s", key, r.Status, holdOf(r), orig.Status, holdOf(orig))
						}
					}
					if !r.Replayed && r.Status != http.StatusServiceUnavailable && r.Status != http.StatusConflict {
						mu.Lock()
						if _, had := first[key]; !had {
							first[key] = r
						} else {
							t.Errorf("key %s executed twice", key)
						}
						mu.Unlock()
					}
				case p < 30: // age a random live hold into expiry
					agedCount.Add(e.ageRandomHold(rng))
				default: // a new hold
					n := 1 + rng.Intn(4)
					perm := rng.Perm(len(seats))[:n]
					ids := make([]string, n)
					for i, j := range perm {
						ids[i] = seats[j]
					}
					body, _ := json.Marshal(map[string]any{"seat_ids": ids})
					key := fmt.Sprintf("l6-%d-%d", b, op)
					mu.Lock()
					bodies[key] = body
					mu.Unlock()
					mine = append(mine, key)
					r, err := e.svc.Hold(context.Background(), userA, key, eventID, body)
					if err != nil {
						t.Errorf("Hold: %v", err)
						continue
					}
					check(t, r, &deadlocks, &timeouts)
					switch r.Status {
					case http.StatusCreated:
						created.Add(1)
					case http.StatusConflict:
						refused.Add(1)
					}
					if r.Status == http.StatusCreated || r.Status == http.StatusConflict && !isInProgress(r) {
						mu.Lock()
						first[key] = r
						mu.Unlock()
					}
				}
			}
		}(b)
	}
	bwg.Wait()
	close(stopWorkers)
	wwg.Wait()

	// Drain: age every remaining live hold, then let a worker finish.
	if _, err := e.pool.Exec(context.Background(), `UPDATE holds SET expires_at = now() - interval '1 minute' WHERE status = 'ACTIVE'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE seats s SET hold_expires_at = h.expires_at FROM holds h WHERE s.hold_id = h.hold_id AND s.status = 'HELD'`); err != nil {
		t.Fatal(err)
	}
	w, _ := e.worker(Config{})
	for i := 0; i < 10; i++ {
		if r := e.runOnce(w); r.Candidates == 0 {
			break
		}
	}
	if n := e.count(`SELECT count(*) FROM holds WHERE status = 'ACTIVE'`); n != 0 {
		t.Fatalf("%d holds still ACTIVE after draining", n)
	}
	if n := e.count(`SELECT count(*) FROM seats WHERE status = 'HELD'`); n != 0 {
		t.Fatalf("%d seats still HELD after draining", n)
	}

	t.Logf("L6: %d holds, %d refusals, %d replays, %d aged, %d lock timeouts (503), %d worker errors, %d deadlocks",
		created.Load(), refused.Load(), replays.Load(), agedCount.Load(), timeouts.Load(), workerErrs.Load(), deadlocks.Load())
	if deadlocks.Load() != 0 {
		t.Fatalf("%d deadlocks (40P01)", deadlocks.Load())
	}
	if workerErrs.Load() != 0 {
		t.Fatalf("%d worker batches failed", workerErrs.Load())
	}
	if created.Load() == 0 || refused.Load() == 0 || agedCount.Load() == 0 || replays.Load() == 0 {
		t.Fatal("the stress mix did not exercise every path")
	}
	e.audit()
}

func check(t *testing.T, r holds.Response, deadlocks, timeouts *atomic.Int64) {
	t.Helper()
	switch r.Status {
	case http.StatusCreated, http.StatusConflict, http.StatusUnprocessableEntity:
	case http.StatusServiceUnavailable:
		switch sqlState(r.Cause) {
		case "40P01":
			deadlocks.Add(1)
		case "55P03":
			timeouts.Add(1)
		default:
			t.Errorf("503 with cause %v", r.Cause)
		}
	default:
		t.Errorf("unexpected status %d %s", r.Status, r.Body)
	}
}

func isInProgress(r holds.Response) bool {
	var p struct{ Type string }
	_ = json.Unmarshal(r.Body, &p)
	return p.Type == "urn:surgetix:problem:idempotency-in-progress"
}

func holdOf(r holds.Response) string {
	var c holds.Created
	_ = json.Unmarshal(r.Body, &c)
	return c.HoldID
}

// ageRandomHold moves one random ACTIVE hold and its seats' mirror into the
// past in one transaction (rank order: hold row, then its seats).
func (e *env) ageRandomHold(rng *rand.Rand) int64 {
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT hold_id::text FROM holds WHERE status = 'ACTIVE' AND expires_at > now()
		ORDER BY hold_id OFFSET $1 LIMIT 1 FOR UPDATE SKIP LOCKED`, rng.Intn(5)).Scan(&id)
	if err != nil {
		return 0
	}
	if _, err := tx.Exec(ctx, `UPDATE holds SET expires_at = now() - interval '1 minute' WHERE hold_id = $1`, id); err != nil {
		return 0
	}
	if _, err := tx.Exec(ctx, `UPDATE seats SET hold_expires_at = now() - interval '1 minute' WHERE hold_id = $1 AND status = 'HELD'`, id); err != nil {
		return 0
	}
	if tx.Commit(ctx) != nil {
		return 0
	}
	return 1
}

func sqlState(err error) string {
	type coder interface{ SQLState() string }
	for err != nil {
		if c, ok := err.(coder); ok {
			return c.SQLState()
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return ""
		}
		err = u.Unwrap()
	}
	return ""
}

var _ = pgx.ErrNoRows
