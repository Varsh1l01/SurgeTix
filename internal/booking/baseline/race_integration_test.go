//go:build unsafe_baseline && integration

package baseline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// barrier releases only when n buyers have all finished the CHECK. Every one
// of them has then seen the seats AVAILABLE, so every one of them books: the
// oversell is forced by ordering, not by timing, and the counts are exact.
type barrier struct {
	t       *testing.T
	arrived sync.WaitGroup
	release chan struct{}
	once    sync.Once
}

func newBarrier(t *testing.T, n int) *barrier {
	b := &barrier{t: t, release: make(chan struct{})}
	b.arrived.Add(n)
	go func() { b.arrived.Wait(); b.once.Do(func() { close(b.release) }) }()
	return b
}

func (b *barrier) checkpoint(ctx context.Context, _ pgx.Tx) {
	b.arrived.Done()
	select {
	case <-b.release:
	case <-time.After(30 * time.Second):
		// Fail rather than hang if a buyer never reaches the barrier.
		b.t.Error("barrier timed out: not every buyer passed the CHECK")
		b.once.Do(func() { close(b.release) })
	case <-ctx.Done():
	}
}

type outcome struct {
	booked, unavailable, other atomic.Int64
	errs                       sync.Map
}

func (o *outcome) record(err error) {
	var ue *UnavailableError
	switch {
	case err == nil:
		o.booked.Add(1)
	case errors.As(err, &ue):
		o.unavailable.Add(1)
	default:
		o.other.Add(1)
		o.errs.Store(err.Error(), true)
	}
}

// buyConcurrently runs one Book per entry of requests, all at once.
func buyConcurrently(s *Store, requests [][]string) *outcome {
	var o outcome
	var wg sync.WaitGroup
	for i, seats := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, err := s.Book(ctx, SyntheticEventID, fmt.Sprintf("buyer-%d", i), seats)
			o.record(err)
		}()
	}
	wg.Wait()
	return &o
}

func repeat(seats []string, n int) [][]string {
	out := make([][]string, n)
	for i := range out {
		out[i] = seats
	}
	return out
}

func mustAudit(t *testing.T, s *Store) AuditResult {
	t.Helper()
	a, err := s.Audit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func checkNoOtherErrors(t *testing.T, o *outcome) {
	t.Helper()
	if o.other.Load() != 0 {
		o.errs.Range(func(k, _ any) bool { t.Errorf("unexpected error: %v", k); return true })
		t.FailNow()
	}
}

func TestOversellTwoBuyersOneSeat(t *testing.T) {
	s, _ := openStore(t, 4)
	o := buyConcurrently(s.WithCheckpoint(newBarrier(t, 2).checkpoint), repeat([]string{SeatID(7)}, 2))
	checkNoOtherErrors(t, o)

	a := mustAudit(t, s)
	t.Logf("2 buyers, 1 seat: booked=%d audit=%+v", o.booked.Load(), a)
	if o.booked.Load() != 2 || a.Bookings != 2 || a.SeatsBooked != 1 || a.DoubleBookedSeats != 1 ||
		a.ExcessSeatBookings != 1 || a.MaxBookingsPerSeat != 2 || a.StatusMismatches != 0 {
		t.Fatalf("expected both buyers to get seat 7: booked=%d audit=%+v", o.booked.Load(), a)
	}
}

func TestOversellManyBuyersOneSeat(t *testing.T) {
	const k = 50
	s, _ := openStore(t, k+5)
	o := buyConcurrently(s.WithCheckpoint(newBarrier(t, k).checkpoint), repeat([]string{SeatID(42)}, k))
	checkNoOtherErrors(t, o)

	a := mustAudit(t, s)
	t.Logf("%d buyers, 1 seat: booked=%d audit=%+v", k, o.booked.Load(), a)
	if o.booked.Load() != k || a.Bookings != k || a.SeatsBooked != 1 || a.DoubleBookedSeats != 1 ||
		a.ExcessSeatBookings != k-1 || a.MaxBookingsPerSeat != k {
		t.Fatalf("expected %d bookings for one seat: audit=%+v", k, a)
	}
}

// 200 buyers for 10 seats: 20 buyers per seat, each group forced through the
// race together. Groups run one after another so at most 20 transactions are
// open at once.
func TestOversellDemandExceedsInventory(t *testing.T) {
	const seats, perSeat = 10, 20
	s, _ := openStore(t, perSeat+5)
	var booked int64
	for n := 1; n <= seats; n++ {
		o := buyConcurrently(s.WithCheckpoint(newBarrier(t, perSeat).checkpoint), repeat([]string{SeatID(n)}, perSeat))
		checkNoOtherErrors(t, o)
		booked += o.booked.Load()
	}

	a := mustAudit(t, s)
	t.Logf("%d buyers, %d seats: booked=%d audit=%+v", seats*perSeat, seats, booked, a)
	if booked != seats*perSeat || a.Bookings != seats*perSeat || a.SeatsBooked != seats ||
		a.DoubleBookedSeats != seats || a.ExcessSeatBookings != seats*perSeat-seats ||
		a.SeatsAvailable != SeatCount-seats {
		t.Fatalf("expected every buyer to succeed: audit=%+v", a)
	}
}

// Conflicting multi-seat requests {1,2} and {2,3}: both pass the CHECK, both
// book, and seat 2 is sold twice.
func TestOversellOverlappingMultiSeat(t *testing.T) {
	s, _ := openStore(t, 4)
	o := buyConcurrently(s.WithCheckpoint(newBarrier(t, 2).checkpoint),
		[][]string{{SeatID(1), SeatID(2)}, {SeatID(2), SeatID(3)}})
	checkNoOtherErrors(t, o)

	a := mustAudit(t, s)
	t.Logf("overlapping {1,2} {2,3}: booked=%d audit=%+v", o.booked.Load(), a)
	if o.booked.Load() != 2 || a.BookedSeatRows != 4 || a.DistinctSeatsSold != 3 ||
		a.DoubleBookedSeats != 1 || a.ExcessSeatBookings != 1 {
		t.Fatalf("expected seat 2 double-booked: audit=%+v", a)
	}
}

// Control: the same demand, one buyer at a time. The CHECK then always sees
// the truth, so there is no oversell. The bug needs concurrency.
func TestSequentialControlDoesNotOversell(t *testing.T) {
	const seats, perSeat = 10, 20
	s, _ := openStore(t, 4)
	ctx := context.Background()
	var o outcome
	for n := 1; n <= seats; n++ {
		for i := range perSeat {
			_, err := s.Book(ctx, SyntheticEventID, fmt.Sprintf("seq-%d-%d", n, i), []string{SeatID(n)})
			o.record(err)
		}
	}
	checkNoOtherErrors(t, &o)

	a := mustAudit(t, s)
	t.Logf("sequential %d requests, %d seats: booked=%d unavailable=%d audit=%+v",
		seats*perSeat, seats, o.booked.Load(), o.unavailable.Load(), a)
	if o.booked.Load() != seats || o.unavailable.Load() != seats*perSeat-seats ||
		a.DoubleBookedSeats != 0 || a.ExcessSeatBookings != 0 {
		t.Fatalf("sequential booking must not oversell: audit=%+v", a)
	}
}

// The baseline sets READ COMMITTED explicitly. Even with the database default
// switched to SERIALIZABLE (which would make the second writer fail with
// 40001), the baseline transaction is READ COMMITTED and still oversells.
func TestExplicitReadCommittedIgnoresServerDefault(t *testing.T) {
	_, admin := openStore(t, 4)
	ctx := context.Background()
	if _, err := admin.Exec(ctx, `ALTER DATABASE booking_db SET default_transaction_isolation = 'serializable'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `ALTER DATABASE booking_db RESET default_transaction_isolation`)
	})

	// New connections pick up the new default.
	pool, err := pgxpool.New(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var def string
	if err := pool.QueryRow(ctx, `SHOW default_transaction_isolation`).Scan(&def); err != nil || def != "serializable" {
		t.Fatalf("server default = %q, %v; want serializable", def, err)
	}

	var mu sync.Mutex
	var levels []string
	b := newBarrier(t, 2)
	s := NewStore(pool).WithCheckpoint(func(ctx context.Context, tx pgx.Tx) {
		var lvl string
		if err := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&lvl); err != nil {
			t.Error(err)
		}
		mu.Lock()
		levels = append(levels, lvl)
		mu.Unlock()
		b.checkpoint(ctx, tx)
	})
	o := buyConcurrently(s, repeat([]string{SeatID(99)}, 2))
	checkNoOtherErrors(t, o)

	for _, l := range levels {
		if l != "read committed" {
			t.Fatalf("baseline transaction isolation = %q, want read committed", l)
		}
	}
	a := mustAudit(t, NewStore(pool))
	if len(levels) != 2 || o.booked.Load() != 2 || a.DoubleBookedSeats != 1 {
		t.Fatalf("levels=%v booked=%d audit=%+v", levels, o.booked.Load(), a)
	}
}

// Without a barrier the race depends on real timing. This is reported, not
// asserted: the barrier tests above are the evidence.
func TestNaturalRaceReported(t *testing.T) {
	const buyers = 40
	s, _ := openStore(t, buyers+5)
	o := buyConcurrently(s, repeat([]string{SeatID(300)}, buyers))
	checkNoOtherErrors(t, o)
	a := mustAudit(t, s)
	t.Logf("natural race (no barrier), %d buyers on 1 seat: booked=%d unavailable=%d max_bookings_per_seat=%d",
		buyers, o.booked.Load(), o.unavailable.Load(), a.MaxBookingsPerSeat)
	if o.booked.Load() < 1 || o.booked.Load()+o.unavailable.Load() != buyers {
		t.Fatalf("every request must end booked or unavailable: %+v", a)
	}
}

func TestUnknownSeatIsRejected(t *testing.T) {
	s, _ := openStore(t, 4)
	ctx := context.Background()
	_, err := s.Book(ctx, SyntheticEventID, "b", []string{SeatID(1), "00000000-0000-4000-8000-999999999999"})
	if !errors.Is(err, ErrUnknownSeats) {
		t.Fatalf("err = %v, want ErrUnknownSeats", err)
	}
	_, err = s.Book(ctx, "5e1f0000-0000-4000-8000-000000000002", "b", []string{SeatID(1)})
	if !errors.Is(err, ErrUnknownSeats) {
		t.Fatalf("other event: err = %v, want ErrUnknownSeats", err)
	}
	if a := mustAudit(t, s); a.Bookings != 0 {
		t.Fatalf("rejected requests must not book: %+v", a)
	}
}
