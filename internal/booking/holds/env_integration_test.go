//go:build integration

package holds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/booking/inventory"
	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

const (
	holdScope = "POST /v1/events/{eventID}/holds"
	userA     = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	userB     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

// Section prices (paise) of the default test layout.
const (
	priceA = 125000
	priceB = 90000
)

var keySeq atomic.Int64

type env struct {
	t       *testing.T
	pool    *pgxpool.Pool // large enough for the concurrency tests
	store   *idempotency.Store
	eventID string
}

// newEnv: a fresh booking_db with the approved schema and a generated
// inventory (T3) for one event: sections A (2x10 seats) and B (1x10),
// plus any extra sections given.
func newEnv(t *testing.T, extra ...inventory.SeatSection) *env {
	t.Helper()
	ctx := context.Background()
	small := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(ctx, small, nil); err != nil {
		t.Fatal(err)
	}
	cfg := small.Config().Copy()
	cfg.MaxConns = 64
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	layout := inventory.Layout{
		EventID: eventID,
		Seats: append([]inventory.SeatSection{
			{Name: "A", Rows: 2, SeatsPerRow: 10, PricePaise: priceA},
			{Name: "B", Rows: 1, SeatsPerRow: 10, PricePaise: priceB},
		}, extra...),
	}
	inv, err := inventory.Generate(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Load(ctx, pool, inv); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, pool: pool, store: idempotency.New(pool), eventID: eventID}
}

// withLockTimeout lengthens the hold transaction's lock timeout for a test
// that builds a forced lock schedule (production keeps LockTimeout).
func withLockTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := lockTimeout
	lockTimeout = d
	t.Cleanup(func() { lockTimeout = prev })
}

func seatsRequest(t testing.TB, ids ...string) Request {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"seat_ids": ids})
	r, err := ParseRequest(eventID, b)
	if err != nil {
		t.Fatalf("request %v: %v", ids, err)
	}
	return r
}

// claim obtains a fresh idempotency claim for req, as T10 will.
func (e *env) claim(user string, req Request) idempotency.Claim {
	e.t.Helper()
	c, err := req.Canonical()
	if err != nil {
		e.t.Fatal(err)
	}
	h, err := idempotency.RequestHash(holdScope, c)
	if err != nil {
		e.t.Fatal(err)
	}
	k := idempotency.Key{Scope: holdScope, UserID: user, Key: fmt.Sprintf("k-%d", keySeq.Add(1))}
	res, err := e.store.Claim(context.Background(), k, h)
	if err != nil || res.Outcome != idempotency.Proceed {
		e.t.Fatalf("claim: %v %v", res.Outcome, err)
	}
	return res.Claim
}

// hold runs HoldSeats for user with a fresh claim. Safe from goroutines
// only through holdErr (no t.Fatal).
func (e *env) hold(user string, ids ...string) SeatsResult {
	e.t.Helper()
	res, err := HoldSeats(context.Background(), e.pool, e.claim(user, seatsRequest(e.t, ids...)), seatsRequest(e.t, ids...))
	if err != nil {
		e.t.Fatalf("HoldSeats %v: %v", ids, err)
	}
	return res
}

func (e *env) mustHold(user string, ids ...string) string {
	e.t.Helper()
	r := e.hold(user, ids...)
	if r.Outcome != Held {
		e.t.Fatalf("HoldSeats %v: outcome %v, body %s", ids, r.Outcome, r.Response.Body)
	}
	return r.HoldID
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

type seatRow struct {
	status string
	holdID *string
	expiry *time.Time
}

func (e *env) seat(id string) seatRow {
	e.t.Helper()
	var s seatRow
	if err := e.pool.QueryRow(context.Background(),
		`SELECT status, hold_id::text, hold_expires_at FROM seats WHERE event_id = $1 AND seat_id = $2`,
		e.eventID, id).Scan(&s.status, &s.holdID, &s.expiry); err != nil {
		e.t.Fatalf("seat %s: %v", id, err)
	}
	return s
}

func (e *env) expectSeat(id, status string, holdID string) {
	e.t.Helper()
	s := e.seat(id)
	got := ""
	if s.holdID != nil {
		got = *s.holdID
	}
	if s.status != status || got != holdID {
		e.t.Fatalf("seat %s: %s/%q, want %s/%q", id, s.status, got, status, holdID)
	}
}

type holdRow struct {
	status, kind, user string
	expiresAt          time.Time
	createdAt          time.Time
}

func (e *env) holdRow(id string) holdRow {
	e.t.Helper()
	var h holdRow
	if err := e.pool.QueryRow(context.Background(),
		`SELECT status, kind, user_id::text, expires_at, created_at FROM holds WHERE hold_id = $1`, id).
		Scan(&h.status, &h.kind, &h.user, &h.expiresAt, &h.createdAt); err != nil {
		e.t.Fatalf("hold %s: %v", id, err)
	}
	return h
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// expire moves a hold's canonical expiry and its seats' mirror into the
// past together (keeping invariant 9), as if 10 minutes had passed.
func (e *env) expire(holdID string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `UPDATE holds SET expires_at = now() - interval '1 minute' WHERE hold_id = $1`, holdID); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE seats s SET hold_expires_at = h.expires_at FROM holds h
		WHERE h.hold_id = $1 AND s.hold_id = h.hold_id AND s.status = 'HELD'`, holdID); err != nil {
		e.t.Fatal(err)
	}
}

type event struct {
	Type      string              `json:"event_type"`
	EventID   string              `json:"event_id"`
	Aggregate struct{ ID string } `json:"aggregate"`
	Data      json.RawMessage     `json:"data"`
	topic     string
	key       string
}

func (e *env) events() []event {
	e.t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT topic, key, payload, status FROM outbox ORDER BY created_at, payload`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		var topic, key, status string
		var payload []byte
		if err := rows.Scan(&topic, &key, &payload, &status); err != nil {
			e.t.Fatal(err)
		}
		if status != "PENDING" {
			e.t.Fatalf("outbox row %s not PENDING: %s", key, status)
		}
		var ev event
		if err := json.Unmarshal(payload, &ev); err != nil {
			e.t.Fatal(err)
		}
		ev.topic, ev.key = topic, key
		out = append(out, ev)
	}
	return out
}

func eventsOf(evs []event, typ string) []event {
	var out []event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func (e *env) idemRow(c idempotency.Claim) (status string, code *int, body []byte) {
	e.t.Helper()
	if err := e.pool.QueryRow(context.Background(),
		`SELECT status, response_code, response_body FROM idempotency_keys WHERE scope = $1 AND user_id = $2 AND key = $3`,
		c.Scope, c.UserID, c.Key.Key).Scan(&status, &code, &body); err != nil {
		e.t.Fatal(err)
	}
	return
}

// waitBlocked waits until at least n sessions of this database wait on a
// lock (pg_stat_activity), to force a lock schedule deterministically.
func (e *env) waitBlocked(n int) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var got int
		if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&got); err != nil {
			e.t.Fatal(err)
		}
		if got >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %d blocked sessions", n)
}

// lockSeats opens a transaction holding row locks on the given seats, as
// an in-flight transaction (or the expiry worker) would. Release with the
// returned function (rollback).
func (e *env) lockSeats(ids ...string) func() {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	sort.Strings(ids)
	n := 0
	rows, err := tx.Query(ctx, `SELECT seat_id FROM seats WHERE event_id = $1 AND seat_id = ANY($2) ORDER BY seat_id FOR UPDATE`, e.eventID, ids)
	if err != nil {
		e.t.Fatal(err)
	}
	for rows.Next() {
		n++
	}
	rows.Close()
	if n != len(ids) {
		e.t.Fatalf("locked %d of %d seats", n, len(ids))
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

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func seatID(section string, row, n int) string { return fmt.Sprintf("%s-R%02d-%02d", section, row, n) }

// seatID3 is the id in a section with more than 99 seats per row (T3).
func seatID3(section string, row, n int) string { return fmt.Sprintf("%s-R%02d-%03d", section, row, n) }

func decodeCreated(t *testing.T, r SeatsResult) Created {
	t.Helper()
	var c Created
	if err := json.Unmarshal(r.Response.Body, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func joinIDs(ids []string) string { return strings.Join(ids, ",") }

type claimT = idempotency.Claim

// resetSeats frees seats for reuse between rounds of one test. The holds
// owning them are first marked RELEASED (test-only transition), so no live
// hold keeps listing a seat in hold_seats that another hold then takes.
func (e *env) resetSeats(ids ...string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `UPDATE holds SET status = 'RELEASED'
		WHERE status IN ('ACTIVE','PAYMENT_PENDING')
		  AND hold_id IN (SELECT hold_id FROM hold_seats WHERE event_id = $1 AND seat_id = ANY($2))`, e.eventID, ids); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE seats SET status = 'AVAILABLE', hold_id = NULL, hold_expires_at = NULL
		WHERE event_id = $1 AND seat_id = ANY($2)`, e.eventID, ids); err != nil {
		e.t.Fatal(err)
	}
}

var _ = pgx.ErrNoRows
