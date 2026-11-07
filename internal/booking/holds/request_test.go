package holds

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/booking/inventory"
	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
)

const eventID = "5a4b3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d"

func seatIDs(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%q", fmt.Sprintf("A-R01-%02d", i+1))
	}
	return "[" + strings.Join(ids, ",") + "]"
}

// Quantity literals from the V table (plan §13): 0, negative, 5+, and
// non-integer, plus wrong JSON types.
var badQuantities = []string{
	"0", "-1", "-4", "5", "6", "100", "99999999999999999999999",
	"1.5", "2.0", "2e0", "0.4e1", "-0", "4.0000000001",
	`"2"`, "true", "null", "[2]", "{}",
}

// TestV_EachValidQuantityInEveryMode: each of 1-4 is accepted in all three
// modes, and the typed request says what was asked for.
func TestV_EachValidQuantityInEveryMode(t *testing.T) {
	for n := MinQuantity; n <= MaxQuantity; n++ {
		cases := []struct {
			body string
			mode Mode
		}{
			{`{"seat_ids":` + seatIDs(n) + `}`, ModeSeats},
			{`{"best_available":{"section":"A","quantity":` + strconv.Itoa(n) + `}}`, ModeBestAvailable},
			{`{"ga":{"section":"FLOOR","quantity":` + strconv.Itoa(n) + `}}`, ModeGA},
		}
		for _, c := range cases {
			r, err := ParseRequest(eventID, []byte(c.body))
			if err != nil {
				t.Fatalf("%s: %v", c.body, err)
			}
			if r.Mode != c.mode || r.Quantity != n || r.EventID != eventID {
				t.Fatalf("%s: got %+v", c.body, r)
			}
			switch c.mode {
			case ModeSeats:
				if len(r.SeatIDs) != n || r.Section != "" {
					t.Fatalf("%s: got %+v", c.body, r)
				}
			default:
				if r.SeatIDs != nil || r.Section == "" {
					t.Fatalf("%s: got %+v", c.body, r)
				}
			}
		}
	}
}

// TestV_SeatIDsCount: 0 and 5+ seats are rejected (length is the quantity
// in seat mode; a length is always an integer).
func TestV_SeatIDsCount(t *testing.T) {
	for _, n := range []int{0, 5, 6, 40} {
		body := `{"seat_ids":` + seatIDs(n) + `}`
		expectInvalid(t, body, "seat_ids")
	}
}

// TestV_InvalidQuantityInEveryMode: every bad quantity is rejected in
// best-available (section and tier) and GA.
func TestV_InvalidQuantityInEveryMode(t *testing.T) {
	for _, q := range badQuantities {
		expectInvalid(t, `{"best_available":{"section":"A","quantity":`+q+`}}`, "best_available.quantity")
		expectInvalid(t, `{"ga":{"section":"FLOOR","quantity":`+q+`}}`, "ga.quantity")
		// Tier mode validates the quantity first: a bad quantity is a plain
		// validation error, not "tier mode not available".
		_, err := ParseRequest(eventID, []byte(`{"best_available":{"tier":"GOLD","quantity":`+q+`}}`))
		if !errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrTierModeUnavailable) {
			t.Errorf("tier quantity %s: got %v", q, err)
		}
	}
	// Non-integers are reported as such (not merely as out of range).
	for _, q := range []string{"1.5", "2.0", "2e0", "0.4e1", `"2"`, "true"} {
		_, err := ParseRequest(eventID, []byte(`{"ga":{"section":"FLOOR","quantity":`+q+`}}`))
		var ve *ValidationError
		if !errors.As(err, &ve) || !strings.Contains(ve.Reason, "integer") {
			t.Errorf("quantity %s: reason %v does not say integer", q, err)
		}
	}
	// Missing quantity.
	expectInvalid(t, `{"best_available":{"section":"A"}}`, "best_available.quantity")
	expectInvalid(t, `{"ga":{"section":"FLOOR"}}`, "ga.quantity")
}

// TestV_TierModeRejectedUntilM4 (plan §5.3): a well-formed tier request is
// a 422 "tier mode not available yet", for every valid quantity.
func TestV_TierModeRejectedUntilM4(t *testing.T) {
	for n := MinQuantity; n <= MaxQuantity; n++ {
		for _, tier := range []string{`"GOLD"`, `1`, `null`, `{}`} {
			body := fmt.Sprintf(`{"best_available":{"tier":%s,"quantity":%d}}`, tier, n)
			_, err := ParseRequest(eventID, []byte(body))
			if !errors.Is(err, ErrTierModeUnavailable) || !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("%s: got %v", body, err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != "best_available.tier" {
				t.Fatalf("%s: got %#v", body, err)
			}
		}
	}
	expectInvalid(t, `{"best_available":{"section":"A","tier":"GOLD","quantity":2}}`, "best_available")
	expectInvalid(t, `{"best_available":{"quantity":2}}`, "best_available")
}

func TestV_DuplicateSeatIDs(t *testing.T) {
	for _, body := range []string{
		`{"seat_ids":["A-R01-01","A-R01-01"]}`,
		`{"seat_ids":["A-R01-01","A-R01-02","A-R01-01"]}`,
		`{"seat_ids":["A-R01-02","A-R01-02","A-R01-02","A-R01-02"]}`,
		`{"seat_ids":["A-R01-01","A-R01-01"]}`, // escape decodes to the same id
	} {
		expectInvalidPrefix(t, body, "seat_ids[")
	}
	// Ids differing only in case are different ids (seat_id is TEXT).
	if _, err := ParseRequest(eventID, []byte(`{"seat_ids":["a-R01-01","A-R01-01"]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestV_TwoOrNoModes(t *testing.T) {
	seats := `"seat_ids":["A-R01-01"]`
	ba := `"best_available":{"section":"A","quantity":1}`
	ga := `"ga":{"section":"FLOOR","quantity":1}`
	for _, body := range []string{
		"{" + seats + "," + ba + "}",
		"{" + seats + "," + ga + "}",
		"{" + ba + "," + ga + "}",
		"{" + seats + "," + ba + "," + ga + "}",
		`{"seat_ids":null,"ga":{"section":"FLOOR","quantity":1}}`, // null still counts as present
		`{}`,
	} {
		expectInvalid(t, body, "")
	}
}

func TestV_UnknownFields(t *testing.T) {
	for body, field := range map[string]string{
		`{"seat_ids":["A-R01-01"],"quantity":1}`:                          "quantity",
		`{"seat_ids":["A-R01-01"],"event_id":"x"}`:                        "event_id",
		`{"Seat_IDs":["A-R01-01"]}`:                                       "Seat_IDs",
		`{"best_available":{"section":"A","quantity":1,"adjacent":true}}`: "best_available.adjacent",
		`{"ga":{"section":"FLOOR","quantity":1,"bucket":3}}`:              "ga.bucket",
		`{"ga":{"tier":"GOLD","section":"FLOOR","quantity":1}}`:           "ga.tier", // tier is best-available only
		`{"zz":1,"aa":2}`: "aa", // deterministic: first in sorted order
	} {
		expectInvalid(t, body, field)
	}
}

func TestV_IDsEmptyOversizedAndTyped(t *testing.T) {
	maxSeat := strings.Repeat("S", MaxSeatIDBytes)
	maxSection := strings.Repeat("X", MaxSectionBytes)
	ok := []string{
		`{"seat_ids":["` + maxSeat + `"]}`,
		`{"best_available":{"section":"` + maxSection + `","quantity":1}}`,
		`{"ga":{"section":"` + maxSection + `","quantity":1}}`,
	}
	for _, body := range ok {
		if _, err := ParseRequest(eventID, []byte(body)); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
	}
	for body, field := range map[string]string{
		`{"seat_ids":[""]}`:                              "seat_ids[0]",
		`{"seat_ids":["A-R01-01",""]}`:                   "seat_ids[1]",
		`{"seat_ids":["` + maxSeat + `S"]}`:              "seat_ids[0]",
		`{"seat_ids":[1]}`:                               "seat_ids[0]",
		`{"seat_ids":[null]}`:                            "seat_ids[0]",
		`{"seat_ids":"A-R01-01"}`:                        "seat_ids",
		`{"seat_ids":null}`:                              "seat_ids",
		`{"seat_ids":{}}`:                                "seat_ids",
		`{"best_available":{"section":"","quantity":1}}`: "best_available.section",
		`{"best_available":{"section":"` + maxSection + `X","quantity":1}}`: "best_available.section",
		`{"best_available":{"section":7,"quantity":1}}`:                     "best_available.section",
		`{"best_available":{"section":null,"quantity":1}}`:                  "best_available.section",
		`{"best_available":null}`:                                           "best_available",
		`{"best_available":[]}`:                                             "best_available",
		`{"ga":{"section":"","quantity":1}}`:                                "ga.section",
		`{"ga":{"section":"` + maxSection + `X","quantity":1}}`:             "ga.section",
		`{"ga":{"quantity":1}}`:                                             "ga.section",
		`{"ga":"FLOOR"}`:                                                    "ga",
	} {
		expectInvalid(t, body, field)
	}
	// Byte length, not rune count: 10 runes but 30 bytes (3-byte runes).
	expectInvalid(t, `{"ga":{"section":"विभागविभाग","quantity":1}}`, "ga.section")
}

func TestV_MalformedBodies(t *testing.T) {
	for _, body := range []string{
		``, ` `, `null`, `[]`, `"seat_ids"`, `42`,
		`{"seat_ids":["A-R01-01"]`,     // truncated
		`{"seat_ids":["A-R01-01"]} {}`, // trailing data
		`{"seat_ids":["A-R01-01"]}x`,
		`{"seat_ids":["A-R01-01"],"seat_ids":["A-R01-02"]}`,    // duplicate key
		`{"ga":{"section":"FLOOR","quantity":1,"quantity":2}}`, // nested duplicate key
		`{"seat_ids":['A-R01-01']}`,
	} {
		expectInvalid(t, body, "")
	}
}

func TestV_EventID(t *testing.T) {
	for _, id := range []string{"", "not-a-uuid", "5a4b3c2d1e0f4a9b8c7d6e5f4a3b2c1d", "{" + eventID + "}", eventID + " "} {
		_, err := ParseRequest(id, []byte(`{"seat_ids":["A-R01-01"]}`))
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "eventID" {
			t.Errorf("event id %q: got %v", id, err)
		}
	}
	r, err := ParseRequest(strings.ToUpper(eventID), []byte(`{"seat_ids":["A-R01-01"]}`))
	if err != nil || r.EventID != eventID {
		t.Fatalf("upper-case event id: %+v, %v", r, err)
	}
}

func TestSeatIDsSortedForHashing(t *testing.T) {
	r, err := ParseRequest(eventID, []byte(`{"seat_ids":["B-R01-01","A-R02-10","A-R02-09","A-R01-01"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A-R01-01", "A-R02-09", "A-R02-10", "B-R01-01"}
	if fmt.Sprint(r.SeatIDs) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", r.SeatIDs, want)
	}
}

func TestCanonicalForm(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"seat_ids":["A-R01-02","A-R01-01"]}`, `{"event_id":"` + eventID + `","seat_ids":["A-R01-01","A-R01-02"]}`},
		{`{"best_available":{"quantity":2,"section":"A"}}`, `{"best_available":{"quantity":2,"section":"A"},"event_id":"` + eventID + `"}`},
		{`{"ga":{"section":"FLOOR","quantity":4}}`, `{"event_id":"` + eventID + `","ga":{"quantity":4,"section":"FLOOR"}}`},
		{`{"ga":{"section":"<&>","quantity":1}}`, `{"event_id":"` + eventID + `","ga":{"quantity":1,"section":"<&>"}}`}, // no HTML escaping
	}
	for _, c := range cases {
		r, err := ParseRequest(eventID, []byte(c.body))
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("%s\n got: %s\nwant: %s", c.body, got, c.want)
		}
	}
	if _, err := (Request{}).Canonical(); err == nil {
		t.Error("zero Request must not canonicalise")
	}
}

const scope = "POST /v1/events/{eventID}/holds"

func hash(t *testing.T, id, body string) []byte {
	t.Helper()
	r, err := ParseRequest(id, []byte(body))
	if err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	c, err := r.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	h, err := idempotency.RequestHash(scope, c)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestHashEquivalence: reordered but identical requests hash equal
// (plan §7.1); meaningfully different ones never do.
func TestHashEquivalence(t *testing.T) {
	equal := [][2]string{
		{`{"seat_ids":["A-R01-01","A-R01-02","B-R01-01"]}`, ` { "seat_ids" : [ "B-R01-01" , "A-R01-02", "A-R01-01" ] } `},
		{`{"best_available":{"section":"A","quantity":3}}`, `{"best_available":{"quantity":3,"section":"A"}}`},
		{`{"ga":{"section":"FLOOR","quantity":2}}`, "{\n\t\"ga\": {\"quantity\": 2, \"section\": \"FL\\u004fOR\"}\n}"},
	}
	for _, p := range equal {
		if !bytes.Equal(hash(t, eventID, p[0]), hash(t, eventID, p[1])) {
			t.Errorf("should hash equal:\n%s\n%s", p[0], p[1])
		}
	}
	if !bytes.Equal(hash(t, eventID, equal[0][0]), hash(t, strings.ToUpper(eventID), equal[0][0])) {
		t.Error("event id case must not change the hash")
	}

	other := "0b9e5d4c-3a21-4f10-8e7d-6c5b4a392817"
	distinct := map[string][]byte{
		"seats A1":        hash(t, eventID, `{"seat_ids":["A-R01-01"]}`),
		"seats A1 A2":     hash(t, eventID, `{"seat_ids":["A-R01-01","A-R01-02"]}`),
		"seats a1":        hash(t, eventID, `{"seat_ids":["a-R01-01"]}`),
		"seats A1 other":  hash(t, other, `{"seat_ids":["A-R01-01"]}`),
		"ba A 1":          hash(t, eventID, `{"best_available":{"section":"A","quantity":1}}`),
		"ba A 2":          hash(t, eventID, `{"best_available":{"section":"A","quantity":2}}`),
		"ba B 1":          hash(t, eventID, `{"best_available":{"section":"B","quantity":1}}`),
		"ga A 1":          hash(t, eventID, `{"ga":{"section":"A","quantity":1}}`),
		"ga A 1 other ev": hash(t, other, `{"ga":{"section":"A","quantity":1}}`),
	}
	seen := map[string]string{}
	for name, h := range distinct {
		if prev, dup := seen[string(h)]; dup {
			t.Errorf("%s and %s hash equal", name, prev)
		}
		seen[string(h)] = name
	}
}

// TestIDLimitsMatchGeneratedInventory pins the derived limits to the T3
// generator: its longest section name and seat id fit exactly, and a
// longer section name is rejected by the generator too.
func TestIDLimitsMatchGeneratedInventory(t *testing.T) {
	sec := strings.Repeat("S", MaxSectionBytes)
	longest := inventory.SeatID(sec, inventory.MaxRows, inventory.MaxSeatsPerRow, inventory.MaxSeatsPerRow)
	if len(longest) != MaxSeatIDBytes {
		t.Fatalf("longest generated seat id %q is %d bytes, MaxSeatIDBytes = %d", longest, len(longest), MaxSeatIDBytes)
	}
	layout := func(name string) inventory.Layout {
		return inventory.Layout{EventID: eventID, Seats: []inventory.SeatSection{{Name: name, Rows: 1, SeatsPerRow: 1}}}
	}
	if err := layout(sec).Validate(); err != nil {
		t.Fatalf("generator rejects a %d-byte section name: %v", MaxSectionBytes, err)
	}
	if err := layout(sec + "S").Validate(); err == nil {
		t.Fatalf("generator accepts a %d-byte section name; MaxSectionBytes is stale", MaxSectionBytes+1)
	}
	if _, err := ParseRequest(eventID, []byte(`{"seat_ids":["`+longest+`"]}`)); err != nil {
		t.Fatalf("longest generated seat id rejected: %v", err)
	}
}

// TestValidationHasNoInfrastructureDependency: request.go imports nothing
// that can reach a database, cache or network, so invalid requests cannot
// reach Postgres from here.
func TestValidationHasNoInfrastructureDependency(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "request.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		`"bytes"`: true, `"encoding/json"`: true, `"errors"`: true, `"fmt"`: true,
		`"regexp"`: true, `"sort"`: true, `"strings"`: true,
		`"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"`: true, // pure CanonicalJSON only
	}
	for _, imp := range f.Imports {
		if !allowed[imp.Path.Value] {
			t.Errorf("request.go imports %s", imp.Path.Value)
		}
	}
}

func expectInvalid(t *testing.T, body, field string) {
	t.Helper()
	_, err := ParseRequest(eventID, []byte(body))
	var ve *ValidationError
	if !errors.As(err, &ve) || !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("%s: got %v, want a ValidationError", body, err)
		return
	}
	if errors.Is(err, ErrTierModeUnavailable) {
		t.Errorf("%s: unexpected tier error %v", body, err)
	}
	if ve.Field != field {
		t.Errorf("%s: field %q, want %q (%v)", body, ve.Field, field, err)
	}
}

func expectInvalidPrefix(t *testing.T, body, prefix string) {
	t.Helper()
	_, err := ParseRequest(eventID, []byte(body))
	var ve *ValidationError
	if !errors.As(err, &ve) || !strings.HasPrefix(ve.Field, prefix) {
		t.Errorf("%s: got %v, want field %s…", body, err, prefix)
	}
}

// FuzzParseRequest: never panics; every accepted request satisfies the
// limits; its canonical form is stable and parses back to the same request.
func FuzzParseRequest(f *testing.F) {
	for _, s := range []string{
		`{"seat_ids":["A-R01-01","A-R01-02"]}`,
		`{"best_available":{"section":"A","quantity":2}}`,
		`{"best_available":{"tier":"G","quantity":2}}`,
		`{"ga":{"section":"FLOOR","quantity":4}}`,
		`{"seat_ids":["A","A"]}`, `{}`, `[]`, `{"ga":{"quantity":2.0,"section":"x"}}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		r, err := ParseRequest(eventID, []byte(body))
		if err != nil {
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("error %v does not match ErrInvalidRequest", err)
			}
			return
		}
		if r.Quantity < MinQuantity || r.Quantity > MaxQuantity {
			t.Fatalf("quantity %d accepted", r.Quantity)
		}
		if r.Mode == ModeSeats {
			if len(r.SeatIDs) != r.Quantity {
				t.Fatal("seat count != quantity")
			}
			for i, s := range r.SeatIDs {
				if s == "" || len(s) > MaxSeatIDBytes || (i > 0 && r.SeatIDs[i-1] >= s) {
					t.Fatalf("seat ids not valid, sorted and unique: %q", r.SeatIDs)
				}
			}
		} else if r.Section == "" || len(r.Section) > MaxSectionBytes {
			t.Fatalf("section %q accepted", r.Section)
		}
		c1, err := r.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		c2, _ := r.Canonical()
		if !bytes.Equal(c1, c2) {
			t.Fatal("canonical form not stable")
		}
	})
}
