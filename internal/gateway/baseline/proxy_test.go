//go:build unsafe_baseline

package baseline

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func post(t *testing.T, h http.Handler, buyer string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(Route, h)
	req := httptest.NewRequest(http.MethodPost, "/v1/baseline/events/e1/holds", strings.NewReader(`{"seat_ids":["s"]}`))
	req.Header.Set("X-Baseline-Buyer-ID", buyer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestProxyForwardsPathHeadersAndBody(t *testing.T) {
	var gotPath, gotBuyer, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotBuyer = r.URL.Path, r.Header.Get("X-Baseline-Buyer-ID")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"hold_id":"h"}`))
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)

	rec := post(t, NewProxy(u), "buyer-9")
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"hold_id":"h"}` {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
	}
	if gotPath != "/v1/baseline/events/e1/holds" || gotBuyer != "buyer-9" || gotBody != `{"seat_ids":["s"]}` {
		t.Fatalf("upstream saw path=%q buyer=%q body=%q", gotPath, gotBuyer, gotBody)
	}
}

func TestProxyEnforcesGatewayDeadline(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // a slow upstream, standing in for an overloaded Booking
		case <-time.After(UpstreamTimeout + 500*time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)
	start := time.Now()
	rec := post(t, NewProxy(u), "b")
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code = %d, want 504", rec.Code)
	}
	if el := time.Since(start); el > UpstreamTimeout+400*time.Millisecond {
		t.Fatalf("deadline not enforced: took %v", el)
	}
}

func TestProxyUpstreamDown(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens here now
	u, _ := url.Parse("http://" + addr)
	if rec := post(t, NewProxy(u), "b"); rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
}

func TestParseUpstream(t *testing.T) {
	for _, ok := range []string{"http://localhost:18084", "https://booking.internal"} {
		if _, err := ParseUpstream(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "localhost:18084", "ftp://x", "http://"} {
		if _, err := ParseUpstream(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
