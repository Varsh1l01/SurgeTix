package inventory

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// SingleBucket is benchmark/test-only: the production rules (Validate, Load)
// reject it; only ValidateBenchmark / LoadBenchmark accept it.
func TestSingleBucketIsBenchmarkOnly(t *testing.T) {
	inv, err := Generate(Layout{EventID: ev, GA: []GASection{{Name: "SOLO", Capacity: 2000, Mode: SingleBucket}}})
	if err != nil {
		t.Fatalf("Generate must still produce the benchmark layout: %v", err)
	}
	if len(inv.Buckets) != 1 || inv.Buckets[0].Number != 0 || inv.Buckets[0].Capacity != 2000 {
		t.Fatalf("buckets = %+v", inv.Buckets)
	}
	err = inv.Validate()
	if err == nil || !strings.Contains(err.Error(), "benchmark-only") {
		t.Fatalf("production validation must reject a one-bucket section with a clear message, got %v", err)
	}
	if err := Load(context.Background(), nil, inv); err == nil || !strings.Contains(err.Error(), "refusing to load") {
		t.Fatalf("production Load must refuse a SingleBucket inventory, got %v", err)
	}
	if err := inv.ValidateBenchmark(); err != nil {
		t.Fatalf("benchmark validation must accept it: %v", err)
	}
	// The benchmark relaxation is narrow: still exactly bucket 0, and every other rule holds.
	for _, n := range []int{5, 32, -1} {
		bad := Inventory{EventID: ev, Buckets: []Bucket{{EventID: ev, Section: "SOLO", Number: n, Capacity: 2000}}}
		if bad.ValidateBenchmark() == nil {
			t.Fatalf("a lone bucket %d must be rejected even for benchmarks", n)
		}
	}
}

func TestLayoutLimitsAreEnforcedWithClearErrors(t *testing.T) {
	sections := func(n int) []SeatSection {
		var s []SeatSection
		for i := range n {
			s = append(s, SeatSection{Name: fmt.Sprintf("S%d", i), Rows: 1, SeatsPerRow: 1})
		}
		return s
	}
	ok := map[string]Layout{
		"99 rows":               {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 99, SeatsPerRow: 1}}},
		"999 seats per row":     {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 999}}},
		"99 sections":           {EventID: ev, Seats: sections(99)},
		"98 seated + 1 GA (99)": {EventID: ev, Seats: sections(98), GA: []GASection{{Name: "GA", Capacity: 32}}},
	}
	for name, l := range ok {
		if _, err := Generate(l); err != nil {
			t.Errorf("%s must be accepted: %v", name, err)
		}
	}
	bad := map[string]struct {
		l    Layout
		want string
	}{
		"100 rows":               {Layout{EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 100, SeatsPerRow: 1}}}, "100 rows not in 1..99 (row limit)"},
		"1000 seats per row":     {Layout{EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 1000}}}, "1000 seats per row not in 1..999 (seats-per-row limit)"},
		"100 sections":           {Layout{EventID: ev, Seats: sections(100)}, "100 sections, exceeds the limit of 99 sections"},
		"99 seated + 1 GA (100)": {Layout{EventID: ev, Seats: sections(99), GA: []GASection{{Name: "GA", Capacity: 32}}}, "exceeds the limit of 99 sections"},
	}
	for name, c := range bad {
		err := c.l.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}

// Seat ids are unambiguous: section names cannot contain '-', so every id
// splits into exactly (section, row label, number), and that triple maps
// back to one seat only, across sections of different widths.
func TestSeatIDsAreUnambiguous(t *testing.T) {
	l := Layout{EventID: ev, Seats: []SeatSection{
		{Name: "A", Rows: 99, SeatsPerRow: 25},
		{Name: "A1", Rows: 3, SeatsPerRow: 999},
		{Name: "AB", Rows: 1, SeatsPerRow: 100},
		{Name: "1", Rows: 2, SeatsPerRow: 9},
	}}
	inv, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range inv.Seats {
		parts := strings.Split(s.SeatID, "-")
		if len(parts) != 3 || parts[0] != s.Section || parts[1] != s.RowLabel {
			t.Fatalf("id %q does not split into (section, row, number) = (%s, %s, %d)", s.SeatID, s.Section, s.RowLabel, s.SeatNumber)
		}
		var n int
		if _, err := fmt.Sscanf(parts[2], "%d", &n); err != nil || n != s.SeatNumber {
			t.Fatalf("id %q number part %q != seat number %d", s.SeatID, parts[2], s.SeatNumber)
		}
		if seen[s.SeatID] {
			t.Fatalf("duplicate id %s", s.SeatID)
		}
		seen[s.SeatID] = true
	}
	if want := 99*25 + 3*999 + 100 + 18; len(seen) != want {
		t.Fatalf("%d unique ids, want %d", len(seen), want)
	}
	again, _ := Generate(l)
	if !reflect.DeepEqual(inv, again) {
		t.Fatal("ids not deterministic")
	}
}
