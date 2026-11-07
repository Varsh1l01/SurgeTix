package inventory

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const ev = "5e1f0000-0000-4000-8000-0000000000e1"

func TestSplitEven(t *testing.T) {
	for _, c := range []int{0, 1, 31, 32, 33, 63, 64, 2000, 2001, 2_147_483_647} {
		caps := SplitEven(c)
		if len(caps) != BucketsPerSection {
			t.Fatalf("C=%d: %d buckets, want 32", c, len(caps))
		}
		sum, lo, hi := 0, caps[0], caps[0]
		for b, x := range caps {
			sum += x
			lo, hi = min(lo, x), max(hi, x)
			if b > 0 && x > caps[b-1] {
				t.Fatalf("C=%d: remainder not on the lowest buckets: %v", c, caps)
			}
			if want := c/32 + map[bool]int{true: 1, false: 0}[b < c%32]; x != want {
				t.Fatalf("C=%d bucket %d = %d, want %d (PDF split rule)", c, b, x, want)
			}
		}
		if sum != c || hi-lo > 1 {
			t.Fatalf("C=%d: sum %d, spread %d (want sum C, spread <= 1)", c, sum, hi-lo)
		}
	}
	// Spot values named in the plan.
	// 2001 = 32*62 + 17: buckets 0..16 get 63, buckets 17..31 get 62.
	if got := SplitEven(2001); got[0] != 63 || got[16] != 63 || got[17] != 62 || got[31] != 62 {
		t.Fatalf("C=2001 split = %v", got)
	}
	if got := SplitEven(31); got[30] != 1 || got[31] != 0 {
		t.Fatalf("C=31 split = %v", got)
	}
}

func TestEvenGASectionHasExactly32BucketsNumbered0To31(t *testing.T) {
	inv, err := Generate(Layout{EventID: ev, GA: []GASection{{Name: "GA", Capacity: 2000}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Buckets) != 32 {
		t.Fatalf("%d buckets, want 32", len(inv.Buckets))
	}
	for i, b := range inv.Buckets {
		if b.Number != i || b.Section != "GA" {
			t.Fatalf("bucket %d = %+v", i, b)
		}
	}
	if _, ga := inv.Capacity(); ga != 2000 {
		t.Fatalf("GA capacity %d, want 2000", ga)
	}
}

func TestBucketNumberRange(t *testing.T) {
	for _, n := range []int{0, 1, 15, 30, 31} {
		if err := ValidateBucketNumber(n); err != nil {
			t.Errorf("bucket %d must be valid: %v", n, err)
		}
	}
	for _, n := range []int{-1, 32, 33, 100, -32768, 32767} {
		if err := ValidateBucketNumber(n); err == nil {
			t.Errorf("bucket %d must be rejected", n)
		}
	}
}

// Hand-built inventories, bypassing Generate, must be rejected before they
// reach Postgres, which has no CHECK on the bucket number.
func TestInventoryValidateRejectsInvalidRows(t *testing.T) {
	full := func(section string) []Bucket {
		var bs []Bucket
		for b := 0; b < 32; b++ {
			bs = append(bs, Bucket{EventID: ev, Section: section, Number: b, Capacity: 1})
		}
		return bs
	}
	with := func(mod func([]Bucket) []Bucket) Inventory {
		return Inventory{EventID: ev, Buckets: mod(full("GA"))}
	}
	seat := Seat{EventID: ev, SeatID: "A-R01-01", Section: "A", RowLabel: "R01", SeatNumber: 1}
	cases := map[string]Inventory{
		"bucket -1":                          {EventID: ev, Buckets: append(full("GA")[1:], Bucket{EventID: ev, Section: "GA", Number: -1, Capacity: 1})},
		"bucket 32":                          {EventID: ev, Buckets: append(full("GA")[1:], Bucket{EventID: ev, Section: "GA", Number: 32, Capacity: 1})},
		"duplicate bucket":                   with(func(b []Bucket) []Bucket { b[5].Number = 4; return b }),
		"31 buckets":                         with(func(b []Bucket) []Bucket { return b[:31] }),
		"33 buckets":                         with(func(b []Bucket) []Bucket { return append(b, Bucket{EventID: ev, Section: "GA", Number: 32}) }),
		"negative capacity":                  with(func(b []Bucket) []Bucket { b[0].Capacity = -1; return b }),
		"bucket of another event":            with(func(b []Bucket) []Bucket { b[0].EventID = "5e1f0000-0000-4000-8000-0000000000e2"; return b }),
		"single non-zero bucket":             {EventID: ev, Buckets: []Bucket{{EventID: ev, Section: "GA", Number: 7, Capacity: 10}}},
		"single bucket 0 (benchmark layout)": {EventID: ev, Buckets: []Bucket{{EventID: ev, Section: "GA", Number: 0, Capacity: 10}}},
		"duplicate seat":                     {EventID: ev, Seats: []Seat{seat, seat}},
		"seat of another event":              {EventID: ev, Seats: []Seat{{EventID: "5e1f0000-0000-4000-8000-0000000000e2", SeatID: "A-R01-01", Section: "A", RowLabel: "R01", SeatNumber: 1}}},
		"seat number 0":                      {EventID: ev, Seats: []Seat{{EventID: ev, SeatID: "A-R01-00", Section: "A", RowLabel: "R01"}}},
		"seated and GA same name":            {EventID: ev, Seats: []Seat{{EventID: ev, SeatID: "GA-R01-01", Section: "GA", RowLabel: "R01", SeatNumber: 1}}, Buckets: full("GA")},
	}
	for name, inv := range cases {
		t.Run(name, func(t *testing.T) {
			if err := inv.Validate(); err == nil {
				t.Fatal("invalid inventory accepted")
			}
			// Load refuses before touching the database (nil pool is never used).
			if err := Load(context.Background(), nil, inv); err == nil || !strings.Contains(err.Error(), "refusing to load") {
				t.Fatalf("Load must refuse invalid inventory, got %v", err)
			}
		})
	}
}

func TestLayoutValidateRejectsInvalidConfigurations(t *testing.T) {
	okSeats := []SeatSection{{Name: "A", Rows: 2, SeatsPerRow: 3, PricePaise: 100}}
	thirtyTwo := func(v int) []int {
		c := make([]int, 32)
		for i := range c {
			c[i] = v
		}
		return c
	}
	cases := map[string]Layout{
		"bad event id":            {EventID: "not-a-uuid", Seats: okSeats},
		"upper-case event id":     {EventID: strings.ToUpper(ev), Seats: okSeats},
		"no sections":             {EventID: ev},
		"duplicate section":       {EventID: ev, Seats: okSeats, GA: []GASection{{Name: "A", Capacity: 10}}},
		"section name with dash":  {EventID: ev, Seats: []SeatSection{{Name: "A-1", Rows: 1, SeatsPerRow: 1}}},
		"empty section name":      {EventID: ev, Seats: []SeatSection{{Name: "", Rows: 1, SeatsPerRow: 1}}},
		"zero rows":               {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 0, SeatsPerRow: 1}}},
		"100 rows":                {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 100, SeatsPerRow: 1}}},
		"zero seats per row":      {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 0}}},
		"1000 seats per row":      {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 1000}}},
		"negative price":          {EventID: ev, Seats: []SeatSection{{Name: "A", Rows: 1, SeatsPerRow: 1, PricePaise: -1}}},
		"negative GA capacity":    {EventID: ev, GA: []GASection{{Name: "GA", Capacity: -1}}},
		"GA capacity over INT":    {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 1 << 31}}},
		"explicit: 31 capacities": {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 31, Mode: ExplicitBuckets, Capacities: thirtyTwo(1)[:31]}}},
		"explicit: 33 capacities": {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 33, Mode: ExplicitBuckets, Capacities: append(thirtyTwo(1), 1)}}},
		"explicit: sum mismatch":  {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 100, Mode: ExplicitBuckets, Capacities: thirtyTwo(3)}}},
		"explicit: negative":      {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 92, Mode: ExplicitBuckets, Capacities: append(thirtyTwo(3)[:31], -1)}}},
		"capacities with even":    {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 96, Capacities: thirtyTwo(3)}}},
		"unknown bucket mode":     {EventID: ev, GA: []GASection{{Name: "GA", Capacity: 1, Mode: BucketMode(9)}}},
	}
	for name, l := range cases {
		t.Run(name, func(t *testing.T) {
			if err := l.Validate(); err == nil {
				t.Fatal("invalid layout accepted")
			}
			if _, err := Generate(l); err == nil {
				t.Fatal("Generate accepted an invalid layout")
			}
		})
	}
}

func TestGenerateIsDeterministicWithStableUniqueIDs(t *testing.T) {
	l := Layout{EventID: ev,
		Seats: []SeatSection{{Name: "A", Rows: 20, SeatsPerRow: 25, PricePaise: 100000}, {Name: "B", Rows: 3, SeatsPerRow: 120, PricePaise: 50000}},
		GA:    []GASection{{Name: "GA", Capacity: 2001}, {Name: "FLOOR", Capacity: 96, Mode: ExplicitBuckets, Capacities: SplitEven(96)}, {Name: "SOLO", Capacity: 500, Mode: SingleBucket}},
	}
	a, err := Generate(l)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate(l)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same layout generated different inventory")
	}

	ids := map[string]bool{}
	for _, s := range a.Seats {
		if ids[s.SeatID] {
			t.Fatalf("duplicate seat id %s", s.SeatID)
		}
		ids[s.SeatID] = true
	}
	if a.Seats[0].SeatID != "A-R01-01" || a.Seats[499].SeatID != "A-R20-25" || a.Seats[500].SeatID != "B-R01-001" || a.Seats[len(a.Seats)-1].SeatID != "B-R03-120" {
		t.Fatalf("ids: first %s, A last %s, B first %s, last %s", a.Seats[0].SeatID, a.Seats[499].SeatID, a.Seats[500].SeatID, a.Seats[len(a.Seats)-1].SeatID)
	}
	// Zero padding keeps text order equal to (row, seat) order within a section.
	for _, sec := range []string{"A", "B"} {
		var gen, sorted []string
		for _, s := range a.Seats {
			if s.Section == sec {
				gen = append(gen, s.SeatID)
			}
		}
		sorted = append(sorted, gen...)
		sort.Strings(sorted)
		if !reflect.DeepEqual(gen, sorted) {
			t.Fatalf("section %s: seat ids do not sort in row/seat order", sec)
		}
	}

	// Configured capacity equals generated capacity.
	ls, lg := l.Capacity()
	is, ig := a.Capacity()
	if ls != is || lg != ig || ls != 500+360 || lg != 2001+96+500 {
		t.Fatalf("capacity layout (%d,%d) vs inventory (%d,%d)", ls, lg, is, ig)
	}
	// Bucket shapes per mode.
	per := map[string][]Bucket{}
	for _, bk := range a.Buckets {
		per[bk.Section] = append(per[bk.Section], bk)
	}
	if len(per["GA"]) != 32 || len(per["FLOOR"]) != 32 || len(per["SOLO"]) != 1 || per["SOLO"][0].Number != 0 || per["SOLO"][0].Capacity != 500 {
		t.Fatalf("bucket shapes: GA %d, FLOOR %d, SOLO %+v", len(per["GA"]), len(per["FLOOR"]), per["SOLO"])
	}
	for _, bk := range per["FLOOR"] {
		if bk.Capacity >= 4 { // fragmented: no single bucket can fill a 4-ticket order (T7)
			t.Fatalf("fragmented bucket %d has %d", bk.Number, bk.Capacity)
		}
	}
}

func TestComparisonLayoutMatchesBaselineNumbering(t *testing.T) {
	inv, err := Generate(ComparisonLayout(strings.ToUpper(ev)))
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Seats) != 500 || len(inv.Buckets) != 0 || inv.EventID != ev {
		t.Fatalf("comparison layout: %d seats, %d buckets, event %s", len(inv.Seats), len(inv.Buckets), inv.EventID)
	}
	for n := 1; n <= 500; n++ { // baseline seat n -> same row/number here
		s := inv.Seats[n-1]
		if want := SeatID("A", (n-1)/25+1, (n-1)%25+1, 25); s.SeatID != want || s.PricePaise != 100000 || s.Section != "A" {
			t.Fatalf("seat %d = %+v, want id %s at 100000 paise", n, s, want)
		}
	}
}
