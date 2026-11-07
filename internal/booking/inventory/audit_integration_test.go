//go:build integration

package inventory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fixture builds hold/booking/ticket states with test-only SQL, as the real
// hold, expiry and confirmation code will in later tasks.
type fixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	event string
	live  time.Time // a hold expiry in the future
	past  time.Time // a hold expiry already passed (awaiting the worker)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := migratedDB(t)
	inv, err := Generate(mixedLayout(ev))
	if err != nil {
		t.Fatal(err)
	}
	if err := Load(context.Background(), pool, inv); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &fixture{t: t, pool: pool, event: ev, live: now.Add(10 * time.Minute), past: now.Add(-time.Minute)}
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func uid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func (f *fixture) hold(id int, kind, status string, expires time.Time) string {
	h := uid(id)
	f.exec(`INSERT INTO holds (hold_id, event_id, user_id, kind, status, expires_at) VALUES ($1, $2, gen_random_uuid(), $3, $4, $5)`,
		h, f.event, kind, status, expires)
	return h
}

// seatHold lists seats in the hold and marks them HELD by it with the exact mirror.
func (f *fixture) seatHold(id int, status string, expires time.Time, seats ...string) string {
	h := f.hold(id, "SEATS", status, expires)
	for _, s := range seats {
		f.exec(`INSERT INTO hold_seats (hold_id, event_id, seat_id) VALUES ($1, $2, $3)`, h, f.event, s)
		f.exec(`UPDATE seats SET status = 'HELD', hold_id = $1, hold_expires_at = $2 WHERE event_id = $3 AND seat_id = $4`, h, expires, f.event, s)
	}
	return h
}

// book converts a hold: booking row, seats BOOKED (hold_id kept, mirror cleared), one ticket per seat.
func (f *fixture) book(id int, ticketStatus string, seats ...string) (hold, booking string) {
	hold = f.hold(id, "SEATS", "CONVERTED", f.live)
	booking = uid(id + 5000)
	f.exec(`INSERT INTO bookings (booking_id, hold_id, order_id, user_id) VALUES ($1, $2, gen_random_uuid(), gen_random_uuid())`, booking, hold)
	for _, s := range seats {
		f.exec(`INSERT INTO hold_seats (hold_id, event_id, seat_id) VALUES ($1, $2, $3)`, hold, f.event, s)
		f.exec(`UPDATE seats SET status = 'BOOKED', hold_id = $1, hold_expires_at = NULL WHERE event_id = $2 AND seat_id = $3`, hold, f.event, s)
		f.ticket(booking, s, ticketStatus)
	}
	return hold, booking
}

func (f *fixture) ticket(booking, seat, status string) {
	checkedIn := any(nil)
	if status == "CHECKED_IN" {
		checkedIn = time.Now()
	}
	f.exec(`INSERT INTO tickets (ticket_id, booking_id, event_id, seat_id, section, owner_user_id, status, qr_secret_enc, checked_in_at)
		VALUES (gen_random_uuid(), $1, $2, $3, 'A', gen_random_uuid(), $4, '\x00', $5)`, booking, f.event, seat, status, checkedIn)
}

// gaAlloc records an allocation and takes the units from the bucket, as a GA hold does.
func (f *fixture) gaAlloc(hold string, bucket, qty int, released bool) {
	var rel any
	if released {
		rel = time.Now()
	}
	f.exec(`INSERT INTO hold_ga_allocations (hold_id, event_id, section, bucket, quantity, released_at) VALUES ($1, $2, 'GA', $3, $4, $5)`, hold, f.event, bucket, qty, rel)
	if !released {
		f.exec(`UPDATE ga_buckets SET remaining = remaining - $1 WHERE event_id = $2 AND section = 'GA' AND bucket = $3`, qty, f.event, bucket)
	}
}

func audit(t *testing.T, pool *pgxpool.Pool) Report {
	t.Helper()
	r, err := Audit(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuditCleanOnFreshInventory(t *testing.T) {
	f := newFixture(t)
	if r := audit(t, f.pool); !r.Clean() {
		t.Fatalf("fresh inventory must audit clean:\n%s", r)
	}
}

// Every documented valid lifecycle state, including expired holds still
// awaiting the expiry worker, must produce no findings (no false positives).
func TestAuditHasNoFalsePositivesForValidStates(t *testing.T) {
	f := newFixture(t)
	// Seats.
	f.seatHold(1, "ACTIVE", f.live, "A-R01-01", "A-R01-02")       // active hold
	f.seatHold(2, "ACTIVE", f.past, "A-R02-01")                   // expired, awaiting the worker
	h3 := f.seatHold(3, "ACTIVE", f.past, "A-R03-01", "A-R03-02") // contended partial reclaim (plan §8.3):
	f.exec(`INSERT INTO hold_seats VALUES ($1, $2, 'A-R03-01')`, f.hold(4, "SEATS", "ACTIVE", f.live), f.event)
	f.exec(`UPDATE seats SET hold_id = $1, hold_expires_at = $2 WHERE event_id = $3 AND seat_id = 'A-R03-01'`, uid(4), f.live, f.event)
	_ = h3                                               //   A-R03-02 still HELD by the expired old hold
	f.seatHold(5, "PAYMENT_PENDING", f.live, "A-R04-01") // payment window
	f.seatHold(6, "PAYMENT_PENDING", f.past, "A-R04-02") // payment window passed, awaiting the worker
	h7 := f.hold(7, "SEATS", "EXPIRED", f.past)          // expired and drained (history rows stay)
	f.exec(`INSERT INTO hold_seats VALUES ($1, $2, 'A-R05-01')`, h7, f.event)
	h8 := f.hold(8, "SEATS", "RELEASED", f.live) // released early
	f.exec(`INSERT INTO hold_seats VALUES ($1, $2, 'A-R05-02')`, h8, f.event)
	f.book(9, "VALID", "A-R06-01")       // confirmed
	f.book(10, "CHECKED_IN", "A-R06-02") // confirmed and scanned
	// Cancelled, then the seat resold to a new hold (PDF p.32):
	_, b11 := f.book(11, "CANCELLED", "A-R07-01")
	_ = b11
	f.exec(`UPDATE seats SET status = 'AVAILABLE', hold_id = NULL WHERE event_id = $1 AND seat_id = 'A-R07-01'`, f.event)
	f.seatHold(12, "ACTIVE", f.live, "A-R07-01")
	f.book(13, "CANCELLED", "A-R07-02") // cancelled, seat back on sale
	f.exec(`UPDATE seats SET status = 'AVAILABLE', hold_id = NULL WHERE event_id = $1 AND seat_id = 'A-R07-02'`, f.event)
	// GA.
	g1 := f.hold(21, "GA", "ACTIVE", f.live) // active, across two buckets
	f.gaAlloc(g1, 0, 2, false)
	f.gaAlloc(g1, 1, 2, false)
	f.gaAlloc(f.hold(22, "GA", "ACTIVE", f.past), 2, 3, false)    // expired, awaiting the worker
	f.gaAlloc(f.hold(23, "GA", "EXPIRED", f.past), 3, 1, true)    // expired, stock restored once
	f.gaAlloc(f.hold(24, "GA", "RELEASED", f.live), 4, 2, true)   // released, stock restored once
	f.gaAlloc(f.hold(25, "GA", "CONVERTED", f.live), 5, 4, false) // sold

	if r := audit(t, f.pool); !r.Clean() {
		t.Fatalf("valid lifecycle states must audit clean:\n%s", r)
	}
}

type corruption struct {
	check   Check
	subject string
	apply   func(f *fixture)
	also    []Check // other checks the same corruption necessarily violates
}

var corruptions = map[string]corruption{
	"two unexpired holds claim one seat": {check: DoubleBookedSeat, subject: "A-R01-01", apply: func(f *fixture) {
		f.seatHold(1, "ACTIVE", f.live, "A-R01-01")
		f.exec(`INSERT INTO hold_seats VALUES ($1, $2, 'A-R01-01')`, f.hold(2, "SEATS", "ACTIVE", f.live), f.event)
	}},
	"sold seat also claimed by an unexpired hold": {check: DoubleBookedSeat, subject: "A-R01-03", apply: func(f *fixture) {
		f.book(3, "VALID", "A-R01-03") // sold: CONVERTED, booking, valid ticket
		f.exec(`INSERT INTO hold_seats VALUES ($1, $2, 'A-R01-03')`, f.hold(4, "SEATS", "ACTIVE", f.live), f.event)
	}},
	"HELD seat points at a missing hold": {check: HeldSeatWithoutHold, subject: "A-R02-01", apply: func(f *fixture) {
		f.exec(`UPDATE seats SET status = 'HELD', hold_id = gen_random_uuid(), hold_expires_at = $1 WHERE event_id = $2 AND seat_id = 'A-R02-01'`, f.live, f.event)
	}},
	"seat mirror differs from canonical expiry": {check: HeldSeatMirrorMismatch, subject: "A-R02-02", apply: func(f *fixture) {
		f.seatHold(6, "ACTIVE", f.live, "A-R02-02")
		f.exec(`UPDATE seats SET hold_expires_at = hold_expires_at + interval '1 second' WHERE event_id = $1 AND seat_id = 'A-R02-02'`, f.event)
	}},
	"seat still HELD by an EXPIRED hold": {check: OrphanHeldSeat, subject: "A-R03-01", apply: func(f *fixture) {
		f.seatHold(7, "ACTIVE", f.past, "A-R03-01")
		f.exec(`UPDATE holds SET status = 'EXPIRED' WHERE hold_id = $1`, uid(7))
	}},
	"seat still HELD by a RELEASED hold": {check: OrphanHeldSeat, subject: "A-R03-02", apply: func(f *fixture) {
		f.seatHold(8, "ACTIVE", f.live, "A-R03-02")
		f.exec(`UPDATE holds SET status = 'RELEASED' WHERE hold_id = $1`, uid(8))
	}},
	"HELD seat missing from its hold's hold_seats": {check: HeldSeatNotInHold, subject: "A-R03-03", apply: func(f *fixture) {
		h := f.hold(9, "SEATS", "ACTIVE", f.live)
		f.exec(`UPDATE seats SET status = 'HELD', hold_id = $1, hold_expires_at = $2 WHERE event_id = $3 AND seat_id = 'A-R03-03'`, h, f.live, f.event)
	}},
	"GA allocations exceed section capacity": {check: OversoldGA, subject: "EMPTY", also: []Check{GAStockMismatch}, apply: func(f *fixture) {
		f.exec(`INSERT INTO hold_ga_allocations (hold_id, event_id, section, bucket, quantity) VALUES ($1, $2, 'EMPTY', 0, 1)`, f.hold(10, "GA", "ACTIVE", f.live), f.event)
	}},
	"GA stock taken without an allocation (deficit)": {check: GAStockMismatch, subject: "GA/0", apply: func(f *fixture) {
		f.exec(`UPDATE ga_buckets SET remaining = remaining - 1 WHERE event_id = $1 AND section = 'GA' AND bucket = 0`, f.event)
	}},
	"GA stock restored twice (surplus)": {check: GAStockMismatch, subject: "GA/1", apply: func(f *fixture) {
		f.gaAlloc(f.hold(11, "GA", "ACTIVE", f.live), 1, 2, false)
		f.exec(`UPDATE ga_buckets SET remaining = remaining + 2 WHERE event_id = $1 AND section = 'GA' AND bucket = 1`, f.event) // restored without stamping
	}},
	"EXPIRED hold keeps unreleased GA stock": {check: OrphanGAAllocation, subject: uid(12) + " GA/2", apply: func(f *fixture) {
		h := f.hold(12, "GA", "ACTIVE", f.past)
		f.gaAlloc(h, 2, 3, false)
		f.exec(`UPDATE holds SET status = 'EXPIRED' WHERE hold_id = $1`, h)
	}},
	"BOOKED seat without a valid ticket": {check: BookedSeatTicketCount, subject: "A-R04-01", apply: func(f *fixture) {
		f.book(13, "CANCELLED", "A-R04-01") // seat stays BOOKED but its only ticket is cancelled
	}},
	"valid ticket on a seat that is not BOOKED": {check: LiveTicketOnUnbookedSeat, apply: func(f *fixture) {
		f.book(14, "VALID", "A-R04-02")
		f.exec(`UPDATE seats SET status = 'AVAILABLE', hold_id = NULL WHERE event_id = $1 AND seat_id = 'A-R04-02'`, f.event)
	}},
}

func TestAuditDetectsEveryCorruption(t *testing.T) {
	for name, c := range corruptions {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			c.apply(f)
			r := audit(t, f.pool)
			want := map[Check]bool{c.check: true}
			for _, a := range c.also {
				want[a] = true
			}
			found := false
			for _, fd := range r.Findings {
				if !want[fd.Check] {
					t.Errorf("unexpected finding: %s", fd)
				}
				if fd.Check == c.check && fd.EventID == f.event && (c.subject == "" || fd.Subject == c.subject) {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s not detected (want subject %q):\n%s", c.check, c.subject, r)
			}
			t.Logf("%s", r)
		})
	}
}

// Every check in Invariants has at least one corruption test.
func TestEveryCheckHasACorruptionTest(t *testing.T) {
	covered := map[Check]bool{}
	for _, c := range corruptions {
		covered[c.check] = true
	}
	var missing []string
	for _, c := range AllChecks() {
		if !covered[c] {
			missing = append(missing, string(c))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("checks without a corruption test: %v", missing)
	}
}

// Findings name the event and records, and carry the invariant text.
func TestFindingsAreActionableAndPerEvent(t *testing.T) {
	f := newFixture(t)
	other := "5e1f0000-0000-4000-8000-0000000000e4"
	inv, _ := Generate(mixedLayout(other))
	if err := Load(context.Background(), f.pool, inv); err != nil {
		t.Fatal(err)
	}
	f.event = other // corrupt only the second event
	f.seatHold(31, "ACTIVE", f.live, "A-R09-09")
	f.exec(`UPDATE holds SET status = 'RELEASED' WHERE hold_id = $1`, uid(31))

	r := audit(t, f.pool)
	if len(r.Findings) != 1 {
		t.Fatalf("want exactly one finding:\n%s", r)
	}
	fd := r.Findings[0]
	s := fd.String()
	if fd.EventID != other || fd.Subject != "A-R09-09" || !strings.Contains(s, uid(31)) || !strings.Contains(s, "RELEASED") ||
		!strings.Contains(s, Invariants[OrphanHeldSeat]) {
		t.Fatalf("finding is not actionable: %s", s)
	}
	if r.Count(OrphanHeldSeat) != 1 || r.Count(DoubleBookedSeat) != 0 {
		t.Fatalf("counts wrong: %s", r)
	}
}

// The audit never mutates: data is identical after auditing a corrupted
// database, and its transaction mode rejects writes outright.
func TestAuditIsReadOnly(t *testing.T) {
	f := newFixture(t)
	corruptions["GA stock restored twice (surplus)"].apply(f)
	corruptions["seat still HELD by an EXPIRED hold"].apply(f)
	ctx := context.Background()
	snapshot := func() string {
		var s string
		err := f.pool.QueryRow(ctx, `SELECT md5(concat_ws('|',
			(SELECT string_agg(t::text, ',' ORDER BY event_id, seat_id) FROM seats t),
			(SELECT string_agg(t::text, ',' ORDER BY event_id, section, bucket) FROM ga_buckets t),
			(SELECT string_agg(t::text, ',' ORDER BY hold_id) FROM holds t),
			(SELECT string_agg(t::text, ',' ORDER BY hold_id, seat_id) FROM hold_seats t),
			(SELECT string_agg(t::text, ',' ORDER BY hold_id, section, bucket) FROM hold_ga_allocations t),
			(SELECT string_agg(t::text, ',' ORDER BY booking_id) FROM bookings t),
			(SELECT string_agg(t::text, ',' ORDER BY ticket_id) FROM tickets t)))`).Scan(&s)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	r := audit(t, f.pool)
	if r.Clean() {
		t.Fatal("precondition: the database is corrupted")
	}
	if after := snapshot(); after != before {
		t.Fatal("audit changed data")
	}
	// The audit's transaction options make any write fail (SQLSTATE 25006).
	tx, err := f.pool.BeginTx(ctx, auditTx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `UPDATE ga_buckets SET remaining = capacity`)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "25006" {
		t.Fatalf("a write inside the audit transaction must fail with 25006, got %v", err)
	}
}
