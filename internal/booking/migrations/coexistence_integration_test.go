//go:build integration && unsafe_baseline

package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/booking/baseline"
)

// The real schema (public, goose_db_version) and the frozen Phase 2 baseline
// (schema baseline, baseline_goose_db_version) share booking_db without
// interfering, whichever migrates first. The baseline code is only called,
// never changed (ADR-034/035).
func TestCoexistsWithFrozenBaseline(t *testing.T) {
	for _, order := range []string{"real-then-baseline", "baseline-then-real"} {
		t.Run(order, func(t *testing.T) {
			pool := newBookingDB(t)
			ctx := context.Background()
			realFirst := order == "real-then-baseline"
			if realFirst {
				up(t, pool)
			}
			if err := baseline.Migrate(ctx, pool); err != nil {
				t.Fatalf("baseline migrate: %v", err)
			}
			if !realFirst {
				up(t, pool)
			}

			var realVersions, baselineVersions int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.goose_db_version WHERE version_id > 0`).Scan(&realVersions); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.baseline_goose_db_version WHERE version_id > 0`).Scan(&baselineVersions); err != nil {
				t.Fatal(err)
			}
			if realVersions != 4 || baselineVersions != 2 {
				t.Fatalf("version histories: real=%d (want 4), baseline=%d (want 2)", realVersions, baselineVersions)
			}

			// The real schema is still exactly the PDF DDL + approved deviation.
			reference := newBookingDB(t)
			if _, err := reference.Exec(ctx, pdfDDL(t)); err != nil {
				t.Fatal(err)
			}
			if _, err := reference.Exec(ctx, approvedDeviation); err != nil {
				t.Fatal(err)
			}
			got, want := catalog(t, pool), catalog(t, reference)
			// catalog() is limited to schema public minus goose_db_version; the
			// baseline adds only its version table there.
			filtered := got[:0:0]
			for _, l := range got {
				if !strings.Contains(l, " baseline_goose_db_version ") {
					filtered = append(filtered, l)
				}
			}
			if len(filtered) != len(want) {
				t.Fatalf("real schema changed by coexistence: %d vs %d catalog entries", len(filtered), len(want))
			}
			for i := range want {
				if filtered[i] != want[i] {
					t.Fatalf("real schema changed by coexistence:\n got  %s\n want %s", filtered[i], want[i])
				}
			}

			// The baseline still works as frozen: its seed and audit are intact.
			a, err := baseline.NewStore(pool).Audit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if a.SeatsTotal != baseline.SeatCount || a.Bookings != 0 || a.DoubleBookedSeats != 0 {
				t.Fatalf("baseline audit after coexistence = %+v", a)
			}
			// And the two seat tables are distinct objects.
			var realSeats, baseSeats int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.seats`).Scan(&realSeats); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM baseline.seats`).Scan(&baseSeats); err != nil {
				t.Fatal(err)
			}
			if realSeats != 0 || baseSeats != baseline.SeatCount {
				t.Fatalf("seat tables: public=%d (want 0), baseline=%d (want %d)", realSeats, baseSeats, baseline.SeatCount)
			}
		})
	}
}
