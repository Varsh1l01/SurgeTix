//go:build unsafe_baseline && integration

// Package e2e drives the naive baseline end to end over HTTP:
// client → Gateway (internal/gateway/baseline) → Booking
// (internal/booking/baseline) → PostgreSQL booking_db. It lives outside both
// services so neither imports the other (ADR-033).
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	booking "github.com/Varsh1l01/SurgeTix/internal/booking/baseline"
	gateway "github.com/Varsh1l01/SurgeTix/internal/gateway/baseline"
	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
)

var (
	ctr *tcpostgres.PostgresContainer
	dsn string
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	var err error
	ctr, err = tcpostgres.Run(ctx, "postgres:17.11",
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("it-admin"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithInitScripts(filepath.Join("..", "..", "deploy", "compose", "postgres-init", "01-service-databases.sh")),
		testcontainers.WithEnv(map[string]string{
			"IDENTITY_DB_PASSWORD": "it-identity", "CATALOG_DB_PASSWORD": "it-catalog",
			"BOOKING_DB_PASSWORD": "it-booking", "CHECKOUT_DB_PASSWORD": "it-checkout",
		}),
		tcpostgres.BasicWaitStrategies(),
	)
	if err == nil {
		dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
	}
	if err == nil {
		// Connect as the Booking service's own role, not the admin user.
		dsn = strings.Replace(strings.Replace(dsn, "postgres:it-admin@", "booking_svc:it-booking@", 1), "/postgres?", "/booking_db?", 1)
	}
	cancel()
	code := 1
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
	} else {
		code = m.Run()
	}
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	os.Exit(code)
}

type stack struct {
	gatewayURL string
	bookingURL string
	gwMetrics  string
	bkMetrics  string
	store      *booking.Store
}

// newStack starts Booking and Gateway as they run in the binaries, using the
// same Mount functions. If cp is non-nil, Booking's handler uses a store with
// that checkpoint (the deterministic barrier).
func newStack(t *testing.T, maxConns int, cp booking.Checkpoint) *stack {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = int32(maxConns)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	bkRT := &app.Runtime{Mux: http.NewServeMux(), Metrics: metrics.NewRegistry()}
	if cp == nil {
		if err := booking.Mount(ctx, pool, bkRT); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := booking.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
		s := booking.NewStore(pool)
		bkRT.Handle(booking.Route, booking.NewHandler(s.WithCheckpoint(cp), booking.NewMetrics(bkRT.Metrics, s)))
	}
	store := booking.NewStore(pool)
	if err := store.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	bk := httptest.NewServer(withMetrics(bkRT))
	t.Cleanup(bk.Close)

	up, _ := url.Parse(bk.URL)
	gwRT := &app.Runtime{Mux: http.NewServeMux(), Metrics: metrics.NewRegistry()}
	gateway.Mount(gwRT, up)
	gw := httptest.NewServer(withMetrics(gwRT))
	t.Cleanup(gw.Close)

	return &stack{gatewayURL: gw.URL, bookingURL: bk.URL, gwMetrics: gw.URL + "/metrics", bkMetrics: bk.URL + "/metrics", store: store}
}

func withMetrics(rt *app.Runtime) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler(rt.Metrics))
	mux.Handle("/", rt.Mux)
	return mux
}

func (s *stack) book(t *testing.T, buyer string, seats ...int) (int, map[string]any) {
	t.Helper()
	ids := make([]string, len(seats))
	for i, n := range seats {
		ids[i] = `"` + booking.SeatID(n) + `"`
	}
	req, _ := http.NewRequest(http.MethodPost, s.gatewayURL+"/v1/baseline/events/"+booking.SyntheticEventID+"/holds",
		strings.NewReader(`{"seat_ids":[`+strings.Join(ids, ",")+`]}`))
	if buyer != "" {
		req.Header.Set(booking.BuyerHeader, buyer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("request: %v", err)
		return 0, nil
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func scrape(t *testing.T, u string) string {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestRoutesMatch(t *testing.T) {
	if booking.Route != gateway.Route {
		t.Fatalf("gateway forwards %q but booking serves %q", gateway.Route, booking.Route)
	}
}

func TestEndToEndThroughGateway(t *testing.T) {
	s := newStack(t, 8, nil)

	code, body := s.book(t, "e2e-buyer-1", 1, 2)
	if code != http.StatusCreated {
		t.Fatalf("first booking = %d %v", code, body)
	}
	if body["amount"] != float64(200000) || body["expires_at"] != nil || len(body["seats"].([]any)) != 2 || body["hold_id"] == "" {
		t.Fatalf("201 body = %v", body)
	}

	code, body = s.book(t, "e2e-buyer-2", 2, 3)
	if code != http.StatusConflict {
		t.Fatalf("taken seat = %d %v, want 409", code, body)
	}
	if ids, _ := body["unavailable_seat_ids"].([]any); len(ids) != 1 || ids[0] != booking.SeatID(2) {
		t.Fatalf("409 body = %v", body)
	}

	if code, _ := s.book(t, "", 10); code != http.StatusBadRequest {
		t.Fatalf("missing buyer header = %d, want 400", code)
	}

	a, err := s.store.Audit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Bookings != 1 || a.SeatsBooked != 2 || a.DoubleBookedSeats != 0 {
		t.Fatalf("audit = %+v", a)
	}

	bk := scrape(t, s.bkMetrics)
	for _, want := range []string{
		`holds_total{build="unsafe_baseline",mode="seats",result="booked"} 1`,
		`holds_total{build="unsafe_baseline",mode="seats",result="unavailable"} 1`,
		`seats_remaining{build="unsafe_baseline"} 498`,
		`double_booked_seats{build="unsafe_baseline"} 0`,
		`process_cpu_seconds_total`,
	} {
		if !strings.Contains(bk, want) {
			t.Errorf("booking /metrics missing %s", want)
		}
	}
	gw := scrape(t, s.gwMetrics)
	if !strings.Contains(gw, `http_requests_total{code="201",method="POST",route="`+gateway.Route+`"} 1`) {
		t.Errorf("gateway RED metrics missing the 201:\n%s", gw)
	}
}

// The oversell over the full HTTP path, forced by the barrier: 10 buyers send
// the same seat through Gateway; all pass the CHECK before any BOOK, so all
// receive 201 for the same seat.
func TestOversellThroughGateway(t *testing.T) {
	const buyers = 10
	arrived := make(chan struct{}, buyers)
	release := make(chan struct{})
	var once sync.Once
	cp := func(ctx context.Context, _ pgx.Tx) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(20 * time.Second):
			t.Error("barrier timed out")
		case <-ctx.Done():
		}
	}
	s := newStack(t, buyers+5, cp)
	go func() {
		for range buyers {
			<-arrived
		}
		once.Do(func() { close(release) })
	}()

	var wg sync.WaitGroup
	codes := make([]int, buyers)
	for i := range buyers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = s.book(t, fmt.Sprintf("race-%d", i), 250)
		}()
	}
	wg.Wait()

	for i, c := range codes {
		if c != http.StatusCreated {
			t.Fatalf("buyer %d got %d; every buyer passed the CHECK, so every buyer must book", i, c)
		}
	}
	a, err := s.store.Audit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HTTP oversell: %d buyers x seat 250 -> audit %+v", buyers, a)
	if a.Bookings != buyers || a.SeatsBooked != 1 || a.DoubleBookedSeats != 1 || a.ExcessSeatBookings != buyers-1 {
		t.Fatalf("audit = %+v", a)
	}
	if bk := scrape(t, s.bkMetrics); !strings.Contains(bk, `double_booked_seats{build="unsafe_baseline"} 1`) {
		t.Error("double_booked_seats gauge must report the oversell")
	}
}
