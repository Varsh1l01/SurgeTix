// Package inventory generates and loads Booking's inventory (seats and GA
// buckets) into booking_db (ADR-036, AMB-45).
//
// PostgreSQL is the authoritative inventory source (CLAUDE.md invariant 3):
// the generator only produces rows, Load writes them to Postgres, and nothing
// here keeps a cache. Catalog's publish step (copying seat_layouts into
// Booking's seats, PDF p.22) replaces the generator in phase 4.
//
// Generation is pure and deterministic: the same Layout always yields the
// same rows in the same order, with stable ids and no duplicates.
package inventory

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// BucketsPerSection is the PDF's GA bucket count (p.17, p.29, ADR-015):
// buckets are numbered 0..31.
const BucketsPerSection = 32

// MaxBucket is the highest valid bucket number. The schema has no CHECK on
// ga_buckets.bucket (PDF DDL kept exact, owner 2026-11-06), so this package
// is what enforces the range.
const MaxBucket = BucketsPerSection - 1

// Layout limits (documented in the Phase 3 plan §3.3). Exceeding any of them
// is a validation error naming the limit.
//   - MaxRows keeps row labels two digits (R01..R99), so text order equals
//     numeric order, which best-available's ORDER BY row_label, seat_number
//     relies on.
//   - MaxSeatsPerRow keeps seat numbers at most three digits (001..999).
//   - MaxSections bounds one event's layout (seated + GA sections together).
//     Seat ids do not depend on it.
const (
	MaxRows        = 99  // row labels R01..R99
	MaxSeatsPerRow = 999 // seat numbers 01..99, or 001..999 for rows of 100+ seats
	MaxSections    = 99
	maxIntColumn   = math.MaxInt32
)

var sectionNameRE = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

// BucketMode selects how a GA section's capacity is spread over buckets.
type BucketMode int

const (
	// EvenBuckets is the PDF layout: 32 buckets (0..31). Capacity C is split
	// as bucket b = C/32 + (1 if b < C mod 32), so the remainder goes one
	// each to the lowest-numbered buckets.
	EvenBuckets BucketMode = iota
	// ExplicitBuckets uses Capacities as given: exactly 32 values, bucket b
	// gets Capacities[b]. They must sum to Capacity. It is used for
	// fragmented-stock layouts (T7).
	ExplicitBuckets
	// SingleBucket puts the whole capacity in bucket 0. BENCHMARK/TEST ONLY:
	// it exists solely for T7's "lock wait with 32 buckets vs 1" measurement
	// (PDF p.35). Production loading (Load) rejects it. Only LoadBenchmark
	// accepts it.
	SingleBucket
)

// SeatSection is a reserved-seating section of Rows × SeatsPerRow seats.
type SeatSection struct {
	Name        string
	Rows        int
	SeatsPerRow int
	PricePaise  int64
}

// GASection is a general-admission section. Generation is independent of
// pricing. The PDF schema has no GA price (ga_buckets has none), and none is
// invented here. The GA price source is an open dependency of GA holds
// (AMB-53, before M3).
type GASection struct {
	Name       string
	Capacity   int
	Mode       BucketMode
	Capacities []int // ExplicitBuckets only
}

// Layout is one event's inventory configuration.
type Layout struct {
	EventID string // UUID, canonical lower-case form
	Seats   []SeatSection
	GA      []GASection
}

// Seat is one row of booking_db.seats as generated (status AVAILABLE).
type Seat struct {
	EventID    string
	SeatID     string
	Section    string
	RowLabel   string
	SeatNumber int
	PricePaise int64
}

// Bucket is one row of booking_db.ga_buckets as generated (remaining = capacity).
type Bucket struct {
	EventID  string
	Section  string
	Number   int
	Capacity int
}

// Inventory is the generated rows of one event.
type Inventory struct {
	EventID string
	Seats   []Seat
	Buckets []Bucket
}

// Capacity is the event's total sellable units: seats plus GA tickets.
func (inv Inventory) Capacity() (seats, ga int) {
	for _, b := range inv.Buckets {
		ga += b.Capacity
	}
	return len(inv.Seats), ga
}

// Capacity is the configured total: seats plus GA tickets.
func (l Layout) Capacity() (seats, ga int) {
	for _, s := range l.Seats {
		seats += s.Rows * s.SeatsPerRow
	}
	for _, g := range l.GA {
		ga += g.Capacity
	}
	return seats, ga
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Validate reports every configuration problem at once.
func (l Layout) Validate() error {
	var errs []error
	if !uuidRE.MatchString(l.EventID) {
		errs = append(errs, fmt.Errorf("event id %q is not a lower-case UUID", l.EventID))
	}
	if len(l.Seats)+len(l.GA) == 0 {
		errs = append(errs, errors.New("layout has no sections"))
	}
	if len(l.Seats)+len(l.GA) > MaxSections {
		errs = append(errs, fmt.Errorf("layout has %d sections, exceeds the limit of %d sections", len(l.Seats)+len(l.GA), MaxSections))
	}
	names := map[string]bool{}
	checkName := func(n string) {
		if !sectionNameRE.MatchString(n) {
			errs = append(errs, fmt.Errorf("section name %q must be 1-16 letters or digits", n))
		}
		if names[n] {
			errs = append(errs, fmt.Errorf("duplicate section name %q", n))
		}
		names[n] = true
	}
	for _, s := range l.Seats {
		checkName(s.Name)
		if s.Rows < 1 || s.Rows > MaxRows {
			errs = append(errs, fmt.Errorf("section %s: %d rows not in 1..%d (row limit)", s.Name, s.Rows, MaxRows))
		}
		if s.SeatsPerRow < 1 || s.SeatsPerRow > MaxSeatsPerRow {
			errs = append(errs, fmt.Errorf("section %s: %d seats per row not in 1..%d (seats-per-row limit)", s.Name, s.SeatsPerRow, MaxSeatsPerRow))
		}
		if s.PricePaise < 0 {
			errs = append(errs, fmt.Errorf("section %s: price %d paise is negative", s.Name, s.PricePaise))
		}
	}
	for _, g := range l.GA {
		checkName(g.Name)
		if g.Capacity < 0 || g.Capacity > maxIntColumn {
			errs = append(errs, fmt.Errorf("GA section %s: capacity %d not in 0..%d", g.Name, g.Capacity, maxIntColumn))
		}
		switch g.Mode {
		case EvenBuckets, SingleBucket:
			if g.Capacities != nil {
				errs = append(errs, fmt.Errorf("GA section %s: Capacities is only for ExplicitBuckets", g.Name))
			}
		case ExplicitBuckets:
			if len(g.Capacities) != BucketsPerSection {
				errs = append(errs, fmt.Errorf("GA section %s: ExplicitBuckets needs exactly %d capacities, got %d", g.Name, BucketsPerSection, len(g.Capacities)))
				break
			}
			sum := 0
			for b, c := range g.Capacities {
				if c < 0 || c > maxIntColumn {
					errs = append(errs, fmt.Errorf("GA section %s: bucket %d capacity %d not in 0..%d", g.Name, b, c, maxIntColumn))
				}
				sum += c
			}
			if sum != g.Capacity {
				errs = append(errs, fmt.Errorf("GA section %s: bucket capacities sum to %d, section capacity is %d", g.Name, sum, g.Capacity))
			}
		default:
			errs = append(errs, fmt.Errorf("GA section %s: unknown bucket mode %d", g.Name, g.Mode))
		}
	}
	return errors.Join(errs...)
}

// SeatID is the deterministic seat id '<section>-R<row>-<number>', zero-padded
// so ids sort in row and seat order (e.g. "A-R01-01"; three digits when a row
// has more than 99 seats).
func SeatID(section string, row, number, seatsPerRow int) string {
	return section + "-" + RowLabel(row) + "-" + fmt.Sprintf("%0*d", seatDigits(seatsPerRow), number)
}

// RowLabel is "R01" … "R99".
func RowLabel(row int) string { return fmt.Sprintf("R%02d", row) }

func seatDigits(seatsPerRow int) int {
	if seatsPerRow > 99 {
		return 3
	}
	return 2
}

// SplitEven is the PDF layout's bucket split of capacity c over 32 buckets.
func SplitEven(c int) []int {
	out := make([]int, BucketsPerSection)
	for b := range out {
		out[b] = c / BucketsPerSection
		if b < c%BucketsPerSection {
			out[b]++
		}
	}
	return out
}

// Generate validates l and returns its rows: seats in (section, row, seat)
// order, buckets in (section, bucket) order.
func Generate(l Layout) (Inventory, error) {
	if err := l.Validate(); err != nil {
		return Inventory{}, fmt.Errorf("invalid layout: %w", err)
	}
	inv := Inventory{EventID: l.EventID}
	for _, s := range l.Seats {
		for r := 1; r <= s.Rows; r++ {
			for n := 1; n <= s.SeatsPerRow; n++ {
				inv.Seats = append(inv.Seats, Seat{
					EventID: l.EventID, SeatID: SeatID(s.Name, r, n, s.SeatsPerRow),
					Section: s.Name, RowLabel: RowLabel(r), SeatNumber: n, PricePaise: s.PricePaise,
				})
			}
		}
	}
	for _, g := range l.GA {
		var caps []int
		switch g.Mode {
		case EvenBuckets:
			caps = SplitEven(g.Capacity)
		case ExplicitBuckets:
			caps = g.Capacities
		case SingleBucket:
			caps = []int{g.Capacity}
		}
		for b, c := range caps {
			inv.Buckets = append(inv.Buckets, Bucket{EventID: l.EventID, Section: g.Name, Number: b, Capacity: c})
		}
	}
	// A layout that asks for SingleBucket generates benchmark inventory; it
	// still gets every structural check, only the 32-bucket rule is relaxed.
	if err := inv.validate(l.usesSingleBucket()); err != nil {
		return Inventory{}, fmt.Errorf("generator produced invalid inventory (bug): %w", err)
	}
	return inv, nil
}

func (l Layout) usesSingleBucket() bool {
	for _, g := range l.GA {
		if g.Mode == SingleBucket {
			return true
		}
	}
	return false
}

// Validate applies the PRODUCTION rules to generated (or hand-built) rows
// before they reach Postgres:
//   - bucket numbers 0..31 (the database has no CHECK on the bucket number);
//   - no duplicate seats or buckets, and valid values;
//   - every GA section has EXACTLY 32 buckets.
//
// A one-bucket (SingleBucket) section is rejected. Load always calls this.
func (inv Inventory) Validate() error { return inv.validate(false) }

// ValidateBenchmark is Validate with one relaxation: a GA section may also
// consist of exactly bucket 0 (the SingleBucket benchmark layout).
// BENCHMARK/TEST ONLY.
func (inv Inventory) ValidateBenchmark() error { return inv.validate(true) }

func (inv Inventory) validate(allowSingleBucket bool) error {
	var errs []error
	seen := map[string]bool{}
	for _, s := range inv.Seats {
		if s.EventID != inv.EventID {
			errs = append(errs, fmt.Errorf("seat %s belongs to event %s, inventory is %s", s.SeatID, s.EventID, inv.EventID))
		}
		if seen[s.SeatID] {
			errs = append(errs, fmt.Errorf("duplicate seat id %s", s.SeatID))
		}
		seen[s.SeatID] = true
		if s.SeatID == "" || s.Section == "" || s.RowLabel == "" || s.SeatNumber < 1 || s.PricePaise < 0 {
			errs = append(errs, fmt.Errorf("seat %q has invalid fields", s.SeatID))
		}
	}
	perSection := map[string]map[int]bool{}
	for _, b := range inv.Buckets {
		if b.EventID != inv.EventID {
			errs = append(errs, fmt.Errorf("bucket %s/%d belongs to event %s, inventory is %s", b.Section, b.Number, b.EventID, inv.EventID))
		}
		if err := ValidateBucketNumber(b.Number); err != nil {
			errs = append(errs, fmt.Errorf("section %s: %w", b.Section, err))
		}
		if b.Capacity < 0 || b.Capacity > maxIntColumn {
			errs = append(errs, fmt.Errorf("section %s bucket %d: capacity %d not in 0..%d", b.Section, b.Number, b.Capacity, maxIntColumn))
		}
		if perSection[b.Section] == nil {
			perSection[b.Section] = map[int]bool{}
		}
		if perSection[b.Section][b.Number] {
			errs = append(errs, fmt.Errorf("section %s: duplicate bucket %d", b.Section, b.Number))
		}
		perSection[b.Section][b.Number] = true
	}
	for sec, nums := range perSection {
		if allowSingleBucket && len(nums) == 1 && nums[0] {
			continue // SingleBucket benchmark layout (T7 comparison)
		}
		if len(nums) != BucketsPerSection {
			hint := ""
			if len(nums) == 1 && nums[0] {
				hint = " (a one-bucket section is a benchmark-only layout; production loading requires 32)"
			}
			errs = append(errs, fmt.Errorf("section %s has %d buckets, want exactly %d (0..%d)%s", sec, len(nums), BucketsPerSection, MaxBucket, hint))
		}
	}
	seatSections := map[string]bool{}
	for _, s := range inv.Seats {
		seatSections[s.Section] = true
	}
	for sec := range seatSections {
		if perSection[sec] != nil {
			errs = append(errs, fmt.Errorf("section %s is both seated and GA", sec))
		}
	}
	return errors.Join(errs...)
}

// ValidateBucketNumber enforces 0..31, the range the database cannot check.
func ValidateBucketNumber(n int) error {
	if n < 0 || n > MaxBucket {
		return fmt.Errorf("bucket number %d not in 0..%d", n, MaxBucket)
	}
	return nil
}

// ComparisonLayout is the 500-seat layout that matches the frozen Phase 2
// baseline: section A, rows R01–R20 × 25 seats, 100,000 paise each. Seat n
// (1..500) of the baseline is SeatID("A", (n-1)/25+1, (n-1)%25+1, 25) here.
func ComparisonLayout(eventID string) Layout {
	return Layout{EventID: strings.ToLower(eventID), Seats: []SeatSection{{Name: "A", Rows: 20, SeatsPerRow: 25, PricePaise: 100000}}}
}
