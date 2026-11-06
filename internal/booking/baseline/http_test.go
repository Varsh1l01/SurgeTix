//go:build unsafe_baseline

package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
)

type fakeBooker struct {
	calls int
	got   []string
	ret   Booking
	err   error
}

func (f *fakeBooker) Book(_ context.Context, _, _ string, seats []string) (Booking, error) {
	f.calls++
	f.got = seats
	return f.ret, f.err
}

func serve(t *testing.T, h http.Handler, event, buyer, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(Route, h)
	req := httptest.NewRequest(http.MethodPost, "/v1/baseline/events/"+event+"/holds", strings.NewReader(body))
	if buyer != "" {
		req.Header.Set(BuyerHeader, buyer)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandlerValidation(t *testing.T) {
	ok := `{"seat_ids":["` + SeatID(1) + `"]}`
	cases := []struct {
		name, event, buyer, body string
		code                     int
	}{
		{"missing buyer", SyntheticEventID, "", ok, http.StatusBadRequest},
		{"buyer too long", SyntheticEventID, strings.Repeat("x", 129), ok, http.StatusBadRequest},
		{"bad event id", "not-a-uuid", "b", ok, http.StatusUnprocessableEntity},
		{"bad json", SyntheticEventID, "b", `{`, http.StatusUnprocessableEntity},
		{"no seats", SyntheticEventID, "b", `{"seat_ids":[]}`, http.StatusUnprocessableEntity},
		{"five seats", SyntheticEventID, "b", `{"seat_ids":["` + strings.Join([]string{SeatID(1), SeatID(2), SeatID(3), SeatID(4), SeatID(5)}, `","`) + `"]}`, http.StatusUnprocessableEntity},
		{"duplicate seats", SyntheticEventID, "b", `{"seat_ids":["` + SeatID(1) + `","` + strings.ToUpper(SeatID(1)) + `"]}`, http.StatusUnprocessableEntity},
		{"bad seat id", SyntheticEventID, "b", `{"seat_ids":["x"]}`, http.StatusUnprocessableEntity},
		{"best available unsupported", SyntheticEventID, "b", `{"best_available":{"section":"A","quantity":2}}`, http.StatusUnprocessableEntity},
		{"ga unsupported", SyntheticEventID, "b", `{"ga":{"section":"A","quantity":2}}`, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeBooker{}
			rec := serve(t, NewHandler(f, nil), c.event, c.buyer, c.body)
			if rec.Code != c.code {
				t.Fatalf("code = %d, want %d: %s", rec.Code, c.code, rec.Body)
			}
			if f.calls != 0 {
				t.Fatal("invalid requests must not reach the store")
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("content type = %q", ct)
			}
		})
	}
}

func TestHandlerResponses(t *testing.T) {
	booked := Booking{ID: "11111111-1111-4111-8111-111111111111", AmountPaise: 200000, Seats: []Seat{
		{ID: SeatID(2), Section: "A", Row: "R01", Number: 2, PricePaise: 100000},
		{ID: SeatID(1), Section: "A", Row: "R01", Number: 1, PricePaise: 100000},
	}}
	body := `{"seat_ids":["` + strings.ToUpper(SeatID(2)) + `","` + SeatID(1) + `"]}`

	t.Run("201 mirrors the hold response", func(t *testing.T) {
		f := &fakeBooker{ret: booked}
		rec := serve(t, NewHandler(f, nil), SyntheticEventID, "b", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("code = %d: %s", rec.Code, rec.Body)
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"hold_id", "seats", "expires_at", "amount"} {
			if _, ok := got[k]; !ok {
				t.Errorf("missing %q in %v", k, got)
			}
		}
		if got["expires_at"] != nil || got["amount"] != float64(200000) || got["hold_id"] != booked.ID {
			t.Errorf("body = %v", got)
		}
		seats := got["seats"].([]any)
		if seats[0].(map[string]any)["seat_id"] != SeatID(1) {
			t.Errorf("seats not sorted: %v", seats)
		}
		if f.got[0] != SeatID(2) {
			t.Errorf("seat ids must be lower-cased before the store: %v", f.got)
		}
	})

	t.Run("409 lists unavailable seats", func(t *testing.T) {
		f := &fakeBooker{err: &UnavailableError{SeatIDs: []string{SeatID(1)}}}
		rec := serve(t, NewHandler(f, nil), SyntheticEventID, "b", body)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), SeatID(1)) {
			t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
		}
	})

	t.Run("422 unknown seats", func(t *testing.T) {
		rec := serve(t, NewHandler(&fakeBooker{err: ErrUnknownSeats}, nil), SyntheticEventID, "b", body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("code = %d", rec.Code)
		}
	})

	t.Run("500 hides internal errors", func(t *testing.T) {
		rec := serve(t, NewHandler(&fakeBooker{err: errors.New("pq: secret detail")}, nil), SyntheticEventID, "b", body)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
		}
	})
}

func TestHandlerCountsResults(t *testing.T) {
	reg := metrics.NewRegistry()
	m := NewMetrics(reg, nil)
	ok := `{"seat_ids":["` + SeatID(1) + `"]}`
	serve(t, NewHandler(&fakeBooker{}, m), SyntheticEventID, "b", ok)
	serve(t, NewHandler(&fakeBooker{err: &UnavailableError{}}, m), SyntheticEventID, "b", ok)
	serve(t, NewHandler(&fakeBooker{}, m), SyntheticEventID, "", ok)

	rec := httptest.NewRecorder()
	metrics.Handler(reg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	out, _ := io.ReadAll(rec.Body)
	for _, want := range []string{
		`holds_total{build="unsafe_baseline",mode="seats",result="booked"} 1`,
		`holds_total{build="unsafe_baseline",mode="seats",result="unavailable"} 1`,
		`holds_total{build="unsafe_baseline",mode="seats",result="invalid"} 1`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s", want)
		}
	}
}
