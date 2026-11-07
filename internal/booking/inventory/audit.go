package inventory

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/audit.sql
var auditSQL string

// Check names one audited invariant (Phase 3 plan §13.3).
type Check string

// The audited invariants. Each one names the rule it enforces and where the
// rule comes from. The audit adds no business rule of its own.
const (
	DoubleBookedSeat         Check = "double_booked_seat"
	HeldSeatWithoutHold      Check = "held_seat_without_hold"
	HeldSeatMirrorMismatch   Check = "held_seat_mirror_mismatch"
	OrphanHeldSeat           Check = "orphan_held_seat"
	HeldSeatNotInHold        Check = "held_seat_not_in_hold"
	OversoldGA               Check = "oversold_ga"
	GAStockMismatch          Check = "ga_stock_mismatch"
	OrphanGAAllocation       Check = "orphan_ga_allocation"
	BookedSeatTicketCount    Check = "booked_seat_ticket_count"
	LiveTicketOnUnbookedSeat Check = "live_ticket_on_unbooked_seat"
)

// Invariants maps every check to the invariant it enforces and its source.
var Invariants = map[Check]string{
	DoubleBookedSeat:         "a seat is claimed by at most one hold: an unexpired ACTIVE/PAYMENT_PENDING hold, or a CONVERTED hold whose booking has a valid ticket for it [PDF p.34 double_booked_seats; plan §13.3; p.32 resale after cancellation]",
	HeldSeatWithoutHold:      "a HELD seat's hold_id refers to an existing hold [PDF p.17-18 schema]",
	HeldSeatMirrorMismatch:   "seats.hold_expires_at equals the canonical holds.expires_at [PDF p.21 invariant]",
	OrphanHeldSeat:           "a seat is HELD only by an ACTIVE or PAYMENT_PENDING hold [plan §13.3 no orphans; AMB-50]",
	HeldSeatNotInHold:        "a HELD seat is listed in its hold's hold_seats [PDF p.18 hold_seats = the hold's seats]",
	OversoldGA:               "unreleased GA allocations never exceed a section's capacity [PDF p.34 oversold_ga; plan §13.3]",
	GAStockMismatch:          "per bucket, capacity - remaining = unreleased allocations; a surplus means stock restored twice [plan §13.3]",
	OrphanGAAllocation:       "an EXPIRED or RELEASED hold has restored all its GA stock [plan §8.1]",
	BookedSeatTicketCount:    "every BOOKED seat has exactly one valid ticket [PDF FR-22]",
	LiveTicketOnUnbookedSeat: "a valid (non-cancelled) ticket's seat is BOOKED [PDF p.29-30 transitions]",
}

// AllChecks is every check, in report order.
func AllChecks() []Check {
	out := make([]Check, 0, len(Invariants))
	for c := range Invariants {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Finding is one violation: which invariant, which event, which records.
type Finding struct {
	Check   Check
	EventID string
	Subject string // seat id, "section/bucket", "hold section/bucket" or ticket id
	Detail  string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s event=%s subject=%s: %s. Invariant: %s", f.Check, f.EventID, f.Subject, f.Detail, Invariants[f.Check])
}

// Report is the result of one audit.
type Report struct{ Findings []Finding }

// Clean reports whether no invariant is violated.
func (r Report) Clean() bool { return len(r.Findings) == 0 }

// Count returns the number of findings for one check (e.g. the
// double_booked_seats metric is Count(DoubleBookedSeat)).
func (r Report) Count(c Check) int {
	n := 0
	for _, f := range r.Findings {
		if f.Check == c {
			n++
		}
	}
	return n
}

func (r Report) String() string {
	if r.Clean() {
		return "audit clean: 0 findings"
	}
	lines := make([]string, len(r.Findings))
	for i, f := range r.Findings {
		lines[i] = f.String()
	}
	return fmt.Sprintf("audit: %d finding(s)\n  %s", len(r.Findings), strings.Join(lines, "\n  "))
}

// auditTx is READ ONLY (Postgres rejects any write) and REPEATABLE READ (one
// snapshot and one now() for every check, so changes landing mid-audit cannot
// create false findings).
var auditTx = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// Audit checks every invariant across all events. It never repairs, mutates
// or normalises data. It only reports.
func Audit(ctx context.Context, pool *pgxpool.Pool) (Report, error) {
	tx, err := pool.BeginTx(ctx, auditTx)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // read-only: nothing to commit
	rows, err := tx.Query(ctx, auditSQL)
	if err != nil {
		return Report{}, fmt.Errorf("audit: %w", err)
	}
	defer rows.Close()
	var r Report
	for rows.Next() {
		var f Finding
		var c string
		if err := rows.Scan(&c, &f.EventID, &f.Subject, &f.Detail); err != nil {
			return Report{}, err
		}
		f.Check = Check(c)
		if _, known := Invariants[f.Check]; !known {
			return Report{}, fmt.Errorf("audit: unknown check %q (audit.sql and audit.go out of sync)", c)
		}
		r.Findings = append(r.Findings, f)
	}
	return r, rows.Err()
}
