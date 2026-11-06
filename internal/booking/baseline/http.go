//go:build unsafe_baseline

package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
	"github.com/Varsh1l01/SurgeTix/internal/platform/problem"
)

// Route is the dedicated test-only route (AMB-39). It mirrors the real hold
// route, POST /v1/events/{eventID}/holds, under /v1/baseline/ so the two can
// never be confused.
const Route = "POST /v1/baseline/events/{eventID}/holds"

// BuyerHeader carries the generated test identity (AMB-39). The baseline has
// no Identity service, JWT or access token.
const BuyerHeader = "X-Baseline-Buyer-ID"

const (
	maxSeats     = 4 // the hold API allows 1-4 seats (FR-6)
	maxBuyerLen  = 128
	maxBodyBytes = 4 << 10

	scrapeTimeout = 2 * time.Second
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Booker is the store behaviour the handler needs.
type Booker interface {
	Book(ctx context.Context, eventID, buyerID string, seatIDs []string) (Booking, error)
}

// request mirrors the hold API body (docs/05-api/public-rest-api.md). Only the
// chosen-seats mode exists in the baseline.
type request struct {
	SeatIDs       []string        `json:"seat_ids"`
	BestAvailable json.RawMessage `json:"best_available"`
	GA            json.RawMessage `json:"ga"`
}

// response mirrors the hold API's 201 body {hold_id, seats, expires_at, amount}.
// The baseline books immediately, so hold_id carries the booking id and
// expires_at is always null. amount is integer paise (ADR-032).
type response struct {
	HoldID    string  `json:"hold_id"`
	Seats     []Seat  `json:"seats"`
	ExpiresAt *string `json:"expires_at"`
	Amount    int64   `json:"amount"`
}

// Handler serves Route. There is no Idempotency-Key handling (AMB-39).
type Handler struct {
	store Booker
	m     *Metrics
}

// NewHandler returns the baseline handler. m may be nil.
func NewHandler(store Booker, m *Metrics) *Handler { return &Handler{store: store, m: m} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result := h.serve(w, r)
	if h.m != nil {
		h.m.holds.WithLabelValues(result).Inc()
	}
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) string {
	eventID := strings.ToLower(r.PathValue("eventID"))
	if !uuidRE.MatchString(eventID) {
		problem.Write(w, http.StatusUnprocessableEntity, "eventID must be a UUID", nil)
		return "invalid"
	}
	buyer := r.Header.Get(BuyerHeader)
	if buyer == "" || len(buyer) > maxBuyerLen {
		problem.Write(w, http.StatusBadRequest, BuyerHeader+" header is required (1-128 characters)", nil)
		return "invalid"
	}

	var req request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&req); err != nil {
		problem.Write(w, http.StatusUnprocessableEntity, "invalid JSON body", nil)
		return "invalid"
	}
	if req.BestAvailable != nil || req.GA != nil {
		problem.Write(w, http.StatusUnprocessableEntity, "the naive baseline supports only {seat_ids}", nil)
		return "invalid"
	}
	if len(req.SeatIDs) < 1 || len(req.SeatIDs) > maxSeats {
		problem.Write(w, http.StatusUnprocessableEntity, "seat_ids must contain 1 to 4 seats", nil)
		return "invalid"
	}
	seen := make(map[string]bool, len(req.SeatIDs))
	seats := make([]string, 0, len(req.SeatIDs))
	for _, id := range req.SeatIDs {
		id = strings.ToLower(id)
		if !uuidRE.MatchString(id) || seen[id] {
			problem.Write(w, http.StatusUnprocessableEntity, "seat_ids must be distinct UUIDs", nil)
			return "invalid"
		}
		seen[id] = true
		seats = append(seats, id)
	}

	b, err := h.store.Book(r.Context(), eventID, buyer, seats)
	var unavailable *UnavailableError
	switch {
	case err == nil:
		sort.Slice(b.Seats, func(i, j int) bool { return b.Seats[i].ID < b.Seats[j].ID })
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response{HoldID: b.ID, Seats: b.Seats, Amount: b.AmountPaise})
		return "booked"
	case errors.As(err, &unavailable):
		problem.Write(w, http.StatusConflict, "seats not available", map[string]any{"unavailable_seat_ids": unavailable.SeatIDs})
		return "unavailable"
	case errors.Is(err, ErrUnknownSeats):
		problem.Write(w, http.StatusUnprocessableEntity, "unknown seats for this event", nil)
		return "invalid"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		problem.Write(w, http.StatusServiceUnavailable, "request cancelled or timed out", nil)
		return "error"
	default:
		slog.ErrorContext(r.Context(), "baseline booking failed", "err", err)
		problem.Write(w, http.StatusInternalServerError, "", nil)
		return "error"
	}
}

// Metrics are the baseline's domain metrics. Names follow the PDF (p.34),
// with build="unsafe_baseline" so they can never be mistaken for SurgeTix.
type Metrics struct {
	holds *prometheus.CounterVec
}

// NewMetrics registers the counters. If store is non-nil it also registers
// seats_remaining and double_booked_seats, evaluated by SQL at scrape time.
func NewMetrics(reg prometheus.Registerer, store *Store) *Metrics {
	build := prometheus.Labels{"build": "unsafe_baseline"}
	m := &Metrics{holds: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "holds_total",
		Help:        "Booking requests by mode and result (booked, unavailable, invalid, error).",
		ConstLabels: prometheus.Labels{"build": "unsafe_baseline", "mode": "seats"},
	}, []string{"result"})}
	reg.MustRegister(m.holds)
	if store != nil {
		audit := func(pick func(AuditResult) int64) func() float64 {
			return func() float64 {
				ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
				defer cancel()
				a, err := store.Audit(ctx)
				if err != nil {
					return -1 // visible as invalid rather than a plausible 0
				}
				return float64(pick(a))
			}
		}
		reg.MustRegister(
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "seats_remaining", Help: "Seats with status AVAILABLE (scrape-time SQL).", ConstLabels: build,
			}, audit(func(a AuditResult) int64 { return a.SeatsAvailable })),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "double_booked_seats", Help: "Seats with more than one booking (scrape-time SQL). SurgeTix must keep this at 0.", ConstLabels: build,
			}, audit(func(a AuditResult) int64 { return a.DoubleBookedSeats })),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "baseline_excess_seat_bookings", Help: "Seat assignments beyond the seats actually sold (scrape-time SQL).", ConstLabels: build,
			}, audit(func(a AuditResult) int64 { return a.ExcessSeatBookings })),
		)
	}
	return m
}

// Mount migrates the baseline schema and serves Route on rt. Callers must
// have passed the unsafebaseline guard first.
func Mount(ctx context.Context, pool *pgxpool.Pool, rt *app.Runtime) error {
	if err := Migrate(ctx, pool); err != nil {
		return err
	}
	store := NewStore(pool)
	h := NewHandler(store, NewMetrics(rt.Metrics, store))
	rt.Handle(Route, metrics.NewHTTP(rt.Metrics).Wrap(Route, h))
	return nil
}
