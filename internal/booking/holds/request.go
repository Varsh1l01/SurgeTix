// Package holds holds Booking's seat-hold logic. This file is M1-T7: strict,
// server-side validation of the POST /v1/events/{eventID}/holds body for all
// three modes (PDF p.25, p.28-29; FR-6; AMB-10 ✅), and the canonical form
// used as the idempotency hashing input (plan §7.1, AMB-48 ✅).
//
// Body shapes (exactly one mode per request):
//
//	{"seat_ids": ["A-R01-01", ...]}                         chosen seats
//	{"best_available": {"section": "A", "quantity": 2}}     best-available
//	{"best_available": {"tier": ..., "quantity": 2}}        tier mode (M4; rejected until then)
//	{"ga": {"section": "FLOOR", "quantity": 2}}             general admission
//
// Validation is pure: it touches no database, cache or network, so an
// invalid request can never reach Postgres.
package holds

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
)

// Per-hold limits (FR-6; AMB-10 ✅: 1-4 in every mode, server-side).
const (
	MinQuantity = 1
	MaxQuantity = 4
)

// Identifier length limits, in bytes.
//
// Derived (plan §4.2 "empty or oversized ids"): Phase 3 inventory comes only
// from the T3 generator (AMB-45 ✅), whose section names are 1-16 letters
// or digits and whose seat ids are '<section>-R<row>-<number>' with at most
// two row digits and three seat digits. No id longer than these can exist,
// so longer input is rejected before any lookup. Tests pin these values to
// the generator's real output.
const (
	MaxSectionBytes = 16
	MaxSeatIDBytes  = MaxSectionBytes + len("-R99-999")
)

// Mode is the hold mode a request selects.
type Mode int

const (
	ModeSeats         Mode = iota + 1 // {"seat_ids": [...]}
	ModeBestAvailable                 // {"best_available": {"section", "quantity"}}
	ModeGA                            // {"ga": {"section", "quantity"}}
)

func (m Mode) String() string {
	switch m {
	case ModeSeats:
		return "seat_ids"
	case ModeBestAvailable:
		return "best_available"
	case ModeGA:
		return "ga"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// Request is a validated hold request. Build it only with ParseRequest.
type Request struct {
	EventID  string   // lower-case canonical UUID (the path parameter)
	Mode     Mode     //
	SeatIDs  []string // ModeSeats: 1-4 distinct ids, sorted bytewise
	Section  string   // ModeBestAvailable, ModeGA
	Quantity int      // seats requested, 1-4 in every mode (= len(SeatIDs) for ModeSeats)
}

// ErrInvalidRequest matches every validation failure (HTTP 422, plan §11.2).
var ErrInvalidRequest = errors.New("holds: invalid hold request")

// ErrTierModeUnavailable matches a well-formed best-available request in
// tier mode, which is not available until M4 (plan §5.3; AMB-09). It also
// matches ErrInvalidRequest, so it maps to 422 as well.
var ErrTierModeUnavailable = errors.New("holds: tier mode not available yet")

// ValidationError says which field failed and why. Field is a JSON path
// such as "seat_ids[2]" or "ga.quantity"; it is empty for the body as a
// whole, and "eventID" for the path parameter.
type ValidationError struct {
	Field  string
	Reason string
	tier   bool
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return "invalid hold request: " + e.Reason
	}
	return "invalid hold request: " + e.Field + ": " + e.Reason
}

// Is makes errors.Is(err, ErrInvalidRequest) true for every ValidationError,
// and errors.Is(err, ErrTierModeUnavailable) true for the tier rejection.
func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidRequest || (e.tier && target == ErrTierModeUnavailable)
}

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

var (
	uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// A JSON integer literal: no fraction, no exponent (so 2.0 and 2e0 are
	// rejected as non-integer, like 1.5).
	intLiteralRE = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
)

// ParseRequest validates eventID (the path parameter) and body, and returns
// the typed request. Every failure is a *ValidationError matching
// ErrInvalidRequest. Nothing outside this process is consulted.
func ParseRequest(eventID string, body []byte) (Request, error) {
	if !uuidRE.MatchString(eventID) {
		return Request{}, invalid("eventID", "must be a UUID")
	}
	// Malformed JSON, trailing data and duplicate keys (at any depth) are
	// rejected by the same rules that the request hash uses (T5).
	if _, err := idempotency.CanonicalJSON(body); err != nil {
		return Request{}, invalid("", "body is not valid JSON (malformed, trailing data or duplicate keys)")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return Request{}, invalid("", "body is not valid JSON")
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return Request{}, invalid("", "body must be a JSON object")
	}

	var present []string
	for _, k := range sortedKeys(top) {
		switch k {
		case "seat_ids", "best_available", "ga":
			present = append(present, k)
		default:
			return Request{}, invalid(k, "unknown field")
		}
	}
	switch len(present) {
	case 0:
		return Request{}, invalid("", "exactly one of seat_ids, best_available or ga is required")
	case 1:
	default:
		return Request{}, invalid("", "exactly one mode is allowed, got %s", strings.Join(present, " and "))
	}

	r := Request{EventID: strings.ToLower(eventID)}
	var err error
	switch present[0] {
	case "seat_ids":
		r.Mode = ModeSeats
		r.SeatIDs, err = parseSeatIDs(top["seat_ids"])
		r.Quantity = len(r.SeatIDs)
	case "best_available":
		r.Mode = ModeBestAvailable
		r.Section, r.Quantity, err = parseSectionQuantity("best_available", top["best_available"], true)
	case "ga":
		r.Mode = ModeGA
		r.Section, r.Quantity, err = parseSectionQuantity("ga", top["ga"], false)
	}
	if err != nil {
		return Request{}, err
	}
	return r, nil
}

func parseSeatIDs(v any) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, invalid("seat_ids", "must be an array of seat ids")
	}
	if len(arr) < MinQuantity || len(arr) > MaxQuantity {
		return nil, invalid("seat_ids", "must contain %d-%d seat ids, got %d", MinQuantity, MaxQuantity, len(arr))
	}
	ids := make([]string, len(arr))
	seen := make(map[string]int, len(arr))
	for i, e := range arr {
		field := fmt.Sprintf("seat_ids[%d]", i)
		s, ok := e.(string)
		if !ok {
			return nil, invalid(field, "must be a string")
		}
		if err := checkID(field, s, MaxSeatIDBytes); err != nil {
			return nil, err
		}
		if j, dup := seen[s]; dup {
			return nil, invalid(field, "duplicate of seat_ids[%d]", j)
		}
		seen[s] = i
		ids[i] = s
	}
	sort.Strings(ids)
	return ids, nil
}

// parseSectionQuantity validates {"section", "quantity"} (GA) or
// {"section" | "tier", "quantity"} (best-available, allowTier).
func parseSectionQuantity(mode string, v any, allowTier bool) (string, int, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return "", 0, invalid(mode, "must be an object")
	}
	for _, k := range sortedKeys(obj) {
		switch {
		case k == "section", k == "quantity", allowTier && k == "tier":
		default:
			return "", 0, invalid(mode+"."+k, "unknown field")
		}
	}
	_, hasSection := obj["section"]
	_, hasTier := obj["tier"]
	switch {
	case hasSection && hasTier:
		return "", 0, invalid(mode, "exactly one of section or tier is allowed")
	case !hasSection && !hasTier && allowTier:
		return "", 0, invalid(mode, "section or tier is required")
	case !hasSection && !hasTier:
		return "", 0, invalid(mode+".section", "is required")
	}
	q, err := parseQuantity(mode+".quantity", obj["quantity"], obj)
	if err != nil {
		return "", 0, err
	}
	if hasTier {
		// The tier value is not validated: its data model is open (AMB-09).
		return "", 0, &ValidationError{Field: mode + ".tier", Reason: "tier mode not available yet", tier: true}
	}
	s, ok := obj["section"].(string)
	if !ok {
		return "", 0, invalid(mode+".section", "must be a string")
	}
	if err := checkID(mode+".section", s, MaxSectionBytes); err != nil {
		return "", 0, err
	}
	return s, q, nil
}

func parseQuantity(field string, v any, obj map[string]any) (int, error) {
	if _, present := obj["quantity"]; !present {
		return 0, invalid(field, "is required")
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, invalid(field, "must be an integer %d-%d", MinQuantity, MaxQuantity)
	}
	lit := n.String()
	if !intLiteralRE.MatchString(lit) {
		return 0, invalid(field, "must be an integer %d-%d, got %s", MinQuantity, MaxQuantity, lit)
	}
	// Anything longer than one digit (or negative) is out of range; this
	// also avoids integer overflow on huge literals.
	if len(lit) != 1 || lit[0] < '0'+MinQuantity || lit[0] > '0'+MaxQuantity {
		return 0, invalid(field, "must be %d-%d, got %s", MinQuantity, MaxQuantity, lit)
	}
	return int(lit[0] - '0'), nil
}

// sortedKeys gives a deterministic iteration order, so the same bad body
// always yields the same error.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func checkID(field, s string, max int) error {
	switch {
	case s == "":
		return invalid(field, "must not be empty")
	case len(s) > max:
		return invalid(field, "longer than %d bytes", max)
	}
	return nil
}

// Canonical returns the idempotency hashing input for r (plan §7.1):
// canonical JSON with sorted keys, no whitespace, seat_ids sorted, and the
// event id from the path included. Pass it to idempotency.RequestHash.
// Equivalent requests (reordered seat ids, keys or whitespace, or a
// different case of the event UUID) produce identical bytes.
func (r Request) Canonical() ([]byte, error) {
	doc := map[string]any{"event_id": r.EventID}
	switch r.Mode {
	case ModeSeats:
		doc["seat_ids"] = r.SeatIDs
	case ModeBestAvailable:
		doc["best_available"] = map[string]any{"section": r.Section, "quantity": r.Quantity}
	case ModeGA:
		doc["ga"] = map[string]any{"section": r.Section, "quantity": r.Quantity}
	default:
		return nil, errors.New("holds: Canonical on a request not built by ParseRequest")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("holds: canonical form: %w", err)
	}
	return idempotency.CanonicalJSON(raw)
}
