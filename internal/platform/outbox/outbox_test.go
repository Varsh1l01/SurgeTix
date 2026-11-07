package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

const (
	testID    = "8f7c1e2a-4b3d-4c5e-9f60-718293a4b5c6"
	holdID    = "0b9e5d4c-3a21-4f10-8e7d-6c5b4a392817"
	eventUUID = "5a4b3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d"
	userID    = "11111111-1111-4111-8111-111111111111"
)

// A non-UTC time with a whole-microsecond fraction; must print as UTC Z.
var testTime = time.Date(2026, 11, 6, 14, 0, 0, 123456000, time.FixedZone("IST", 19800))

// Representative data shaped like plan §9's v1 fields. The package treats
// data as an opaque JSON object; these pin the envelope around it.
var goldenCases = []struct {
	name string
	ev   Event
	want string
}{
	{
		name: HoldCreated,
		ev:   Event{Type: HoldCreated, Key: holdID, Data: json.RawMessage(`{"hold_id":"` + holdID + `","event_id":"` + eventUUID + `","user_id":"` + userID + `","kind":"SEATS","expires_at":"2026-11-06T08:40:00.000000Z","amount_paise":250000,"seats":["A-1","A-2"]}`)},
		want: `{"event_id":"` + testID + `","event_type":"HoldCreated","schema_version":1,"occurred_at":"2026-11-06T08:30:00.123456Z","producer":"booking","aggregate":{"type":"hold","id":"` + holdID + `"},"data":{"hold_id":"` + holdID + `","event_id":"` + eventUUID + `","user_id":"` + userID + `","kind":"SEATS","expires_at":"2026-11-06T08:40:00.000000Z","amount_paise":250000,"seats":["A-1","A-2"]}}`,
	},
	{
		name: HoldReleased,
		ev:   Event{Type: HoldReleased, Key: holdID, Data: map[string]string{"reason": "released", "hold_id": holdID, "event_id": eventUUID}},
		want: `{"event_id":"` + testID + `","event_type":"HoldReleased","schema_version":1,"occurred_at":"2026-11-06T08:30:00.123456Z","producer":"booking","aggregate":{"type":"hold","id":"` + holdID + `"},"data":{"event_id":"` + eventUUID + `","hold_id":"` + holdID + `","reason":"released"}}`,
	},
	{
		name: HoldExpired,
		ev:   Event{Type: HoldExpired, Key: holdID, Data: json.RawMessage(`{"hold_id":"` + holdID + `","event_id":"` + eventUUID + `","reason":"expired"}`)},
		want: `{"event_id":"` + testID + `","event_type":"HoldExpired","schema_version":1,"occurred_at":"2026-11-06T08:30:00.123456Z","producer":"booking","aggregate":{"type":"hold","id":"` + holdID + `"},"data":{"hold_id":"` + holdID + `","event_id":"` + eventUUID + `","reason":"expired"}}`,
	},
	{
		name: InventoryChanged,
		ev:   Event{Type: InventoryChanged, Key: eventUUID + ":A", Data: json.RawMessage(`{"event_id":"` + eventUUID + `","section":"A","seats":[{"seat_id":"A-1","status":"HELD"},{"seat_id":"A-2","status":"HELD"}]}`)},
		want: `{"event_id":"` + testID + `","event_type":"InventoryChanged","schema_version":1,"occurred_at":"2026-11-06T08:30:00.123456Z","producer":"booking","aggregate":{"type":"section","id":"` + eventUUID + `:A"},"data":{"event_id":"` + eventUUID + `","section":"A","seats":[{"seat_id":"A-1","status":"HELD"},{"seat_id":"A-2","status":"HELD"}]}}`,
	},
	{
		name: InventoryReturned,
		ev:   Event{Type: InventoryReturned, Key: eventUUID + ":GA-Floor", Data: json.RawMessage(`{"event_id":"` + eventUUID + `","section":"GA-Floor","source":"expired","quantity":3}`)},
		want: `{"event_id":"` + testID + `","event_type":"InventoryReturned","schema_version":1,"occurred_at":"2026-11-06T08:30:00.123456Z","producer":"booking","aggregate":{"type":"section","id":"` + eventUUID + `:GA-Floor"},"data":{"event_id":"` + eventUUID + `","section":"GA-Floor","source":"expired","quantity":3}}`,
	},
}

func TestGoldenEnvelopes(t *testing.T) {
	if len(goldenCases) != len(registry) {
		t.Fatalf("golden cases cover %d types, registry has %d", len(goldenCases), len(registry))
	}
	for _, c := range goldenCases {
		t.Run(c.name, func(t *testing.T) {
			s, data, err := validate(c.ev)
			if err != nil {
				t.Fatal(err)
			}
			got, err := encode(testID, testTime, c.ev.Type, s, c.ev.Key, data)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("envelope mismatch\n got: %s\nwant: %s", got, c.want)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	want := map[string][2]string{
		HoldCreated:       {TopicHolds, "hold"},
		HoldReleased:      {TopicHolds, "hold"},
		HoldExpired:       {TopicHolds, "hold"},
		InventoryChanged:  {TopicInventory, "section"},
		InventoryReturned: {TopicInventory, "section"},
	}
	if len(registry) != len(want) {
		t.Fatalf("registry has %d types, want %d", len(registry), len(want))
	}
	for typ, w := range want {
		s := registry[typ]
		if s.topic != w[0] || s.aggregateType != w[1] || s.producer != "booking" {
			t.Errorf("%s: got %+v, want topic %s aggregate %s producer booking", typ, s, w[0], w[1])
		}
	}
	if TopicHolds != "booking.holds" || TopicInventory != "booking.inventory" {
		t.Error("topic names must be the AMB-03 approved names")
	}
}

func TestOccurredAtFormat(t *testing.T) {
	cases := map[time.Time]string{
		time.Date(2026, 11, 6, 8, 30, 0, 0, time.UTC):         "2026-11-06T08:30:00.000000Z",
		time.Date(2026, 11, 6, 8, 30, 0, 100000, time.UTC):    "2026-11-06T08:30:00.000100Z",
		time.Date(2026, 11, 6, 8, 30, 0, 999999000, time.UTC): "2026-11-06T08:30:00.999999Z",
		testTime: "2026-11-06T08:30:00.123456Z",
	}
	for in, want := range cases {
		if got := in.UTC().Format(OccurredAtLayout); got != want {
			t.Errorf("%v: got %s, want %s", in, got, want)
		}
	}
}

type badMarshaler struct{}

func (badMarshaler) MarshalJSON() ([]byte, error) { return nil, errors.New("boom") }

func TestValidateRejects(t *testing.T) {
	obj := map[string]int{"x": 1}
	var nilMap map[string]int
	var nilPtr *struct{ A int }
	cases := []struct {
		name string
		ev   Event
	}{
		{"empty type", Event{Type: "", Key: holdID, Data: obj}},
		{"unknown type", Event{Type: "HoldConfirmed", Key: holdID, Data: obj}},
		{"SoldOut pending AMB-52", Event{Type: "SoldOut", Key: eventUUID, Data: obj}},
		{"type wrong case", Event{Type: "holdcreated", Key: holdID, Data: obj}},
		{"hold key empty", Event{Type: HoldCreated, Key: "", Data: obj}},
		{"hold key not uuid", Event{Type: HoldCreated, Key: "hold-1", Data: obj}},
		{"hold key upper case", Event{Type: HoldCreated, Key: "0B9E5D4C-3A21-4F10-8E7D-6C5B4A392817", Data: obj}},
		{"hold key braces", Event{Type: HoldCreated, Key: "{" + holdID + "}", Data: obj}},
		{"hold key with section", Event{Type: HoldExpired, Key: holdID + ":A", Data: obj}},
		{"section key without section", Event{Type: InventoryChanged, Key: eventUUID, Data: obj}},
		{"section key empty section", Event{Type: InventoryChanged, Key: eventUUID + ":", Data: obj}},
		{"section key bad uuid", Event{Type: InventoryReturned, Key: "event:A", Data: obj}},
		{"section key upper-case uuid", Event{Type: InventoryReturned, Key: "5A4B3C2D-1E0F-4A9B-8C7D-6E5F4A3B2C1D:A", Data: obj}},
		{"section key invalid utf8", Event{Type: InventoryChanged, Key: eventUUID + ":\xff", Data: obj}},
		{"nil data", Event{Type: HoldCreated, Key: holdID, Data: nil}},
		{"nil map data", Event{Type: HoldCreated, Key: holdID, Data: nilMap}},
		{"nil pointer data", Event{Type: HoldCreated, Key: holdID, Data: nilPtr}},
		{"null raw data", Event{Type: HoldCreated, Key: holdID, Data: json.RawMessage(`null`)}},
		{"array data", Event{Type: HoldCreated, Key: holdID, Data: []int{1}}},
		{"string data", Event{Type: HoldCreated, Key: holdID, Data: "x"}},
		{"number data", Event{Type: HoldCreated, Key: holdID, Data: 1}},
		{"malformed raw data", Event{Type: HoldCreated, Key: holdID, Data: json.RawMessage(`{"a":`)}},
		{"unmarshalable data", Event{Type: HoldCreated, Key: holdID, Data: map[string]float64{"x": math.NaN()}}},
		{"marshaler error", Event{Type: HoldCreated, Key: holdID, Data: badMarshaler{}}},
		{"channel data", Event{Type: HoldCreated, Key: holdID, Data: make(chan int)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := validate(c.ev); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("got %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	obj := struct{}{}
	for _, ev := range []Event{
		{Type: HoldCreated, Key: holdID, Data: obj},
		{Type: InventoryChanged, Key: eventUUID + ":Balcony East", Data: map[string]any{}},
		{Type: InventoryChanged, Key: eventUUID + ":A:B", Data: json.RawMessage(` { "a" : 1 } `)},
		{Type: InventoryReturned, Key: eventUUID + ":विभाग", Data: obj},
	} {
		if _, _, err := validate(ev); err != nil {
			t.Errorf("%+v: %v", ev, err)
		}
	}
}

func TestDataIsCompacted(t *testing.T) {
	_, data, err := validate(Event{Type: HoldCreated, Key: holdID, Data: json.RawMessage(" {\n \"a\" : [ 1 , 2 ] }\n")})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":[1,2]}` {
		t.Fatalf("got %s", data)
	}
}

func TestInsertNilTx(t *testing.T) {
	if _, err := Insert(context.Background(), nil, Event{Type: HoldCreated, Key: holdID, Data: struct{}{}}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("got %v, want ErrInvalidEvent", err)
	}
}
