//go:build integration

package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/booking/migrations"
	"github.com/Varsh1l01/SurgeTix/internal/platform/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

// newDB: a fresh booking_db with the approved schema (T2 migrations), so the
// tests write to the real outbox table. Test-only import; production code in
// this package never depends on Booking.
func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := pgtest.NewDatabase(t, pgtest.Booking)
	if _, err := migrations.Up(context.Background(), pool, nil); err != nil {
		t.Fatal(err)
	}
	return pool
}

// begin starts a transaction that is always rolled back at test end (a no-op
// after Commit), so a failing assertion never leaves a connection busy.
func begin(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// holdSeat is the business state change: a real seats row, written in tx.
func holdSeat(t *testing.T, tx pgx.Tx, seatID string) {
	t.Helper()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO seats (event_id, seat_id, section, row_label, seat_number, price_paise, status, hold_id, hold_expires_at)
		 VALUES ($1, $2, 'A', 'A', 1, 125000, 'HELD', $3, now() + interval '10 minutes')`,
		eventUUID, seatID, holdID)
	if err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	// table is a constant chosen by the test, never input.
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func holdCreated() Event {
	return Event{Type: HoldCreated, Key: holdID, Data: map[string]any{"hold_id": holdID, "seats": []string{"A-1"}}}
}

func inventoryChanged() Event {
	return Event{Type: InventoryChanged, Key: eventUUID + ":A", Data: map[string]any{"event_id": eventUUID, "section": "A"}}
}

type row struct {
	id, topic, key, status   string
	payload                  []byte
	attempts                 int
	lockedBy                 *string
	lockedUntil, sentAt      *time.Time
	nextAttemptAt, createdAt time.Time
}

func readRow(t *testing.T, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) row {
	t.Helper()
	var r row
	err := q.QueryRow(context.Background(),
		`SELECT id::text, topic, key, status, payload, attempts, locked_by, locked_until, sent_at, next_attempt_at, created_at
		 FROM outbox WHERE id = $1::uuid`, id).
		Scan(&r.id, &r.topic, &r.key, &r.status, &r.payload, &r.attempts, &r.lockedBy, &r.lockedUntil, &r.sentAt, &r.nextAttemptAt, &r.createdAt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestCommitPersistsStateAndEventsAtomically: the state change and its events
// are invisible until COMMIT, then all visible, with the exact envelope.
func TestCommitPersistsStateAndEventsAtomically(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	tx := begin(t, pool)

	var txNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&txNow); err != nil {
		t.Fatal(err)
	}
	holdSeat(t, tx, "A-1")
	id1, err := Insert(ctx, tx, holdCreated())
	if err != nil {
		t.Fatal(err)
	}
	// Wall-clock time passes inside the transaction; occurred_at must not move.
	if _, err := tx.Exec(ctx, `SELECT pg_sleep(0.05)`); err != nil {
		t.Fatal(err)
	}
	id2, err := Insert(ctx, tx, inventoryChanged())
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatal("two events got the same id")
	}

	// Before commit, another session sees neither the seat nor the events.
	if n := count(t, pool, "outbox"); n != 0 {
		t.Fatalf("outbox visible before commit: %d rows", n)
	}
	if n := count(t, pool, "seats"); n != 0 {
		t.Fatalf("seat visible before commit: %d rows", n)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "outbox"); n != 2 {
		t.Fatalf("outbox rows after commit = %d, want 2", n)
	}
	if n := count(t, pool, "seats"); n != 1 {
		t.Fatalf("seats after commit = %d, want 1", n)
	}

	for _, c := range []struct {
		id    string
		ev    Event
		topic string
	}{{id1, holdCreated(), TopicHolds}, {id2, inventoryChanged(), TopicInventory}} {
		r := readRow(t, pool, c.id)
		if r.topic != c.topic || r.key != c.ev.Key {
			t.Errorf("%s: topic/key = %s/%s, want %s/%s", c.ev.Type, r.topic, r.key, c.topic, c.ev.Key)
		}
		// Left for the phase 4 relay exactly as the schema defaults say.
		if r.status != "PENDING" || r.attempts != 0 || r.lockedBy != nil || r.lockedUntil != nil || r.sentAt != nil {
			t.Errorf("%s: row not untouched PENDING: %+v", c.ev.Type, r)
		}
		if !r.nextAttemptAt.Equal(txNow) || !r.createdAt.Equal(txNow) {
			t.Errorf("%s: next_attempt_at/created_at = %v/%v, want tx now %v", c.ev.Type, r.nextAttemptAt, r.createdAt, txNow)
		}
		// Payload is byte-identical to envelope v1 for (id, tx now()).
		s, data, err := validate(c.ev)
		if err != nil {
			t.Fatal(err)
		}
		want, err := encode(c.id, txNow, c.ev.Type, s, c.ev.Key, data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(r.payload, want) {
			t.Errorf("%s: payload\n got: %s\nwant: %s", c.ev.Type, r.payload, want)
		}
		var env struct {
			EventID    string `json:"event_id"`
			OccurredAt string `json:"occurred_at"`
		}
		if err := json.Unmarshal(r.payload, &env); err != nil {
			t.Fatal(err)
		}
		if env.EventID != c.id || env.EventID != r.id {
			t.Errorf("event_id %s, returned %s, row id %s: must all be equal", env.EventID, c.id, r.id)
		}
		if env.OccurredAt != txNow.UTC().Format(OccurredAtLayout) {
			t.Errorf("occurred_at %s, want tx now %s", env.OccurredAt, txNow.UTC().Format(OccurredAtLayout))
		}
	}
}

func TestRollbackLeavesNothing(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	tx := begin(t, pool)
	holdSeat(t, tx, "A-1")
	if _, err := Insert(ctx, tx, holdCreated()); err != nil {
		t.Fatal(err)
	}
	if _, err := Insert(ctx, tx, inventoryChanged()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "outbox"); n != 0 {
		t.Fatalf("outbox rows after rollback = %d", n)
	}
	if n := count(t, pool, "seats"); n != 0 {
		t.Fatalf("seats after rollback = %d", n)
	}
}

// TestFailureAfterInsertLeavesNothing: a business statement failing after the
// events were written aborts the transaction; COMMIT cannot save the events.
func TestFailureAfterInsertLeavesNothing(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	tx := begin(t, pool)
	holdSeat(t, tx, "A-1")
	if _, err := Insert(ctx, tx, holdCreated()); err != nil {
		t.Fatal(err)
	}
	// Same primary key: the business step fails after the outbox insert.
	_, err := tx.Exec(ctx,
		`INSERT INTO seats (event_id, seat_id, section, row_label, seat_number, price_paise) VALUES ($1, 'A-1', 'A', 'A', 1, 1)`,
		eventUUID)
	if err == nil {
		t.Fatal("duplicate seat insert succeeded")
	}
	// An aborted transaction also refuses further outbox writes.
	if _, err := Insert(ctx, tx, inventoryChanged()); err == nil || errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("Insert in aborted tx: got %v, want a database error", err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, pgx.ErrTxCommitRollback) {
		t.Fatalf("commit of aborted tx: got %v, want ErrTxCommitRollback", err)
	}
	if n := count(t, pool, "outbox"); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
	if n := count(t, pool, "seats"); n != 0 {
		t.Fatalf("seats = %d, want 0", n)
	}
}

// TestRejectedEventWritesNothingAndTxStaysUsable: validation runs before any
// SQL, so a rejected event neither writes a row nor aborts the caller's tx.
func TestRejectedEventWritesNothingAndTxStaysUsable(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	tx := begin(t, pool)
	holdSeat(t, tx, "A-1")
	for _, bad := range []Event{
		{Type: "SoldOut", Key: eventUUID, Data: map[string]any{}},
		{Type: "Bogus", Key: holdID, Data: map[string]any{}},
		{Type: HoldCreated, Key: "not-a-uuid", Data: map[string]any{}},
		{Type: InventoryChanged, Key: eventUUID, Data: map[string]any{}},
		{Type: HoldCreated, Key: holdID, Data: nil},
		{Type: HoldCreated, Key: holdID, Data: []int{1}},
	} {
		if _, err := Insert(ctx, tx, bad); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("%+v: got %v, want ErrInvalidEvent", bad, err)
		}
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n); err != nil {
		t.Fatalf("tx unusable after rejections: %v", err)
	}
	if n != 0 {
		t.Fatalf("rejected events wrote %d rows", n)
	}
	id, err := Insert(ctx, tx, holdCreated())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "outbox"); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
	if r := readRow(t, pool, id); r.key != holdID {
		t.Fatalf("unexpected row %+v", r)
	}
}

// TestEventIDsAreUniqueAndStable: every event gets its own id; the stored
// payload never changes when re-read; ids are what Insert returned.
func TestEventIDsAreUniqueAndStable(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	const n = 200
	ids := make(map[string]bool, n)
	var order []string
	for batch := 0; batch < 2; batch++ { // two transactions, two now() values
		tx := begin(t, pool)
		for i := 0; i < n/2; i++ {
			ev := holdCreated()
			if i%2 == 1 {
				ev = inventoryChanged()
			}
			id, err := Insert(ctx, tx, ev)
			if err != nil {
				t.Fatal(err)
			}
			if !uuidRE.MatchString(id) {
				t.Fatalf("id %q is not a lower-case UUID", id)
			}
			if ids[id] {
				t.Fatalf("duplicate id %s", id)
			}
			ids[id] = true
			order = append(order, id)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if c := count(t, pool, "outbox"); c != n {
		t.Fatalf("rows = %d, want %d", c, n)
	}
	var distinctIDs, distinctEventIDs int
	if err := pool.QueryRow(ctx,
		`SELECT count(DISTINCT id), count(DISTINCT convert_from(payload, 'UTF8')::jsonb->>'event_id') FROM outbox
		 WHERE (convert_from(payload, 'UTF8')::jsonb->>'event_id')::uuid = id`).
		Scan(&distinctIDs, &distinctEventIDs); err != nil {
		t.Fatal(err)
	}
	if distinctIDs != n || distinctEventIDs != n {
		t.Fatalf("rows whose payload event_id = id: %d ids, %d event_ids; want %d", distinctIDs, distinctEventIDs, n)
	}
	for _, id := range order[:5] {
		a, b := readRow(t, pool, id), readRow(t, pool, id)
		if !bytes.Equal(a.payload, b.payload) {
			t.Fatalf("payload of %s changed between reads", id)
		}
	}
}

// TestEventsShareTransactionTimeAcrossInserts: occurred_at is the
// transaction's now(); different transactions get their own.
func TestEventsShareTransactionTimeAcrossInserts(t *testing.T) {
	ctx := context.Background()
	pool := newDB(t)
	times := map[string]bool{}
	for i := 0; i < 2; i++ {
		tx := begin(t, pool)
		var a, b string
		id1, err := Insert(ctx, tx, holdCreated())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT pg_sleep(0.02)`); err != nil {
			t.Fatal(err)
		}
		id2, err := Insert(ctx, tx, inventoryChanged())
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		q := `SELECT convert_from(payload, 'UTF8')::jsonb->>'occurred_at' FROM outbox WHERE id = $1::uuid`
		if err := pool.QueryRow(ctx, q, id1).Scan(&a); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, q, id2).Scan(&b); err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("events in one tx: occurred_at %s vs %s", a, b)
		}
		if _, err := time.Parse(OccurredAtLayout, a); err != nil || len(a) != len("2026-11-06T08:30:00.123456Z") {
			t.Fatalf("occurred_at %q not in RFC 3339 UTC 6-digit form", a)
		}
		times[a] = true
	}
	if len(times) != 2 {
		t.Fatalf("two transactions produced %d distinct occurred_at values, want 2", len(times))
	}
}
