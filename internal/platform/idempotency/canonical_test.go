package idempotency

import (
	"bytes"
	"errors"
	"testing"
)

func hashOf(t *testing.T, scope, doc string) []byte {
	t.Helper()
	h, err := RequestHash(scope, []byte(doc))
	if err != nil {
		t.Fatalf("%s: %v", doc, err)
	}
	return h
}

func TestCanonicalJSONEquivalentRepresentationsHashEqual(t *testing.T) {
	const scope = "POST /v1/events/{eventID}/holds"
	groups := [][]string{
		{ // whitespace and key order, nested
			`{"seat_ids":["A-R01-01","A-R01-02"],"event_id":"e1"}`,
			`{ "event_id" : "e1",
			   "seat_ids" : [ "A-R01-01" , "A-R01-02" ] }`,
		},
		{ // nested object key order
			`{"best_available":{"section":"A","quantity":2},"event_id":"e1"}`,
			`{"event_id":"e1","best_available":{"quantity":2,"section":"A"}}`,
		},
		{ // number spellings of the same value
			`{"ga":{"section":"GA","quantity":2}}`,
			`{"ga":{"section":"GA","quantity":2.0}}`,
			`{"ga":{"section":"GA","quantity":2e0}}`,
			`{"ga":{"section":"GA","quantity":0.2E1}}`,
			`{"ga":{"section":"GA","quantity":20e-1}}`,
		},
		{ // fractional values
			`{"x":0.5}`, `{"x":5e-1}`, `{"x":0.50}`,
		},
		{ // string escapes
			`{"section":"A"}`, `{"section":"A"}`, `{"section":"A"}`,
		},
		{ // HTML-significant characters are not escaped differently
			`{"note":"<a&b>"}`, `{"note":"<a&b>"}`,
		},
	}
	for _, g := range groups {
		first := hashOf(t, scope, g[0])
		for _, doc := range g[1:] {
			if !bytes.Equal(first, hashOf(t, scope, doc)) {
				c1, _ := CanonicalJSON([]byte(g[0]))
				c2, _ := CanonicalJSON([]byte(doc))
				t.Errorf("equivalent documents hash differently:\n  %s -> %s\n  %s -> %s", g[0], c1, doc, c2)
			}
		}
	}
	if c, _ := CanonicalJSON([]byte(` { "b" : [ 1.0 , {"d":null,"c":true} ] , "a" : "x" } `)); string(c) != `{"a":"x","b":[1,{"c":true,"d":null}]}` {
		t.Fatalf("canonical form = %s", c)
	}
}

func TestCanonicalJSONMeaningfullyDifferentPayloadsHashDifferently(t *testing.T) {
	const scope = "POST /v1/events/{eventID}/holds"
	base := `{"event_id":"e1","seat_ids":["A-R01-01","A-R01-02"]}`
	different := map[string]string{
		"changed value":          `{"event_id":"e1","seat_ids":["A-R01-01","A-R01-03"]}`,
		"array order":            `{"event_id":"e1","seat_ids":["A-R01-02","A-R01-01"]}`,
		"extra field":            `{"event_id":"e1","seat_ids":["A-R01-01","A-R01-02"],"x":1}`,
		"missing field":          `{"event_id":"e1"}`,
		"null instead of absent": `{"event_id":"e1","seat_ids":["A-R01-01","A-R01-02"],"ga":null}`,
		"type change":            `{"event_id":"e1","seat_ids":"A-R01-01,A-R01-02"}`,
		"case of a key":          `{"Event_id":"e1","seat_ids":["A-R01-01","A-R01-02"]}`,
	}
	b := hashOf(t, scope, base)
	for name, doc := range different {
		if bytes.Equal(b, hashOf(t, scope, doc)) {
			t.Errorf("%s: different payload hashed equal", name)
		}
	}
	// Numbers: string vs number, and values that floats would round together.
	pairs := [][2]string{
		{`{"q":1}`, `{"q":"1"}`},
		{`{"q":2}`, `{"q":3}`},
		{`{"q":9007199254740993}`, `{"q":9007199254740992}`}, // equal as float64, different exactly
		{`{"q":0.1}`, `{"q":0.10000000000000001}`},
	}
	for _, p := range pairs {
		if bytes.Equal(hashOf(t, scope, p[0]), hashOf(t, scope, p[1])) {
			t.Errorf("%s and %s hashed equal", p[0], p[1])
		}
	}
	// Same body, different scope.
	if bytes.Equal(hashOf(t, "POST /a", base), hashOf(t, "POST /b", base)) {
		t.Error("same body under different scopes hashed equal")
	}
}

func TestCanonicalJSONRejectsAmbiguousOrInvalidInput(t *testing.T) {
	for _, doc := range []string{
		`{"a":1,"a":2}`,       // duplicate key: meaning ambiguous
		`{"a":{"b":1,"b":1}}`, // duplicate, nested
		`{"a":1`,              // truncated
		`{"a":1} {"b":2}`,     // trailing value
		``,                    // empty
		`{a:1}`,               // not JSON
	} {
		if _, err := RequestHash("POST /x", []byte(doc)); !errors.Is(err, ErrInvalidJSON) {
			t.Errorf("%q: want ErrInvalidJSON, got %v", doc, err)
		}
	}
	if _, err := RequestHash("", []byte(`{}`)); err == nil {
		t.Error("empty scope must be rejected")
	}
}
