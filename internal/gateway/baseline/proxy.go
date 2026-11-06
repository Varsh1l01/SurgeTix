//go:build unsafe_baseline

// Package baseline is the Gateway side of the Phase 2 UNSAFE NAIVE BASELINE
// (ADR-034). It forwards exactly one route to Booking over internal HTTP
// (AMB-40). gRPC replaces this in phase 4.
//
// Compiled only with -tags unsafe_baseline and served only when
// SURGETIX_ENABLE_UNSAFE_BASELINE=true.
package baseline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/metrics"
	"github.com/Varsh1l01/SurgeTix/internal/platform/problem"
)

// Route must equal the Booking baseline route (asserted by the e2e test).
const Route = "POST /v1/baseline/events/{eventID}/holds"

// UpstreamTimeout is the gateway deadline the PDF sets for internal calls
// (p.27: "propagate the 1 s gateway deadline").
const UpstreamTimeout = time.Second

// NewProxy forwards requests to upstream (Booking's base URL).
//
// Fairness (ADR-034): the transport keeps up to 1024 idle connections to
// Booking. Go's default of 2 per host would throttle the baseline with
// connection churn that has nothing to do with check-then-book.
func NewProxy(upstream *url.URL) http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.SetXForwarded()
		},
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: UpstreamTimeout, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:        1024,
			MaxIdleConnsPerHost: 1024,
			IdleConnTimeout:     90 * time.Second,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.DeadlineExceeded) {
				problem.Write(w, http.StatusGatewayTimeout, "booking did not answer within the gateway deadline", nil)
				return
			}
			if errors.Is(err, context.Canceled) {
				return // client went away; nothing to answer
			}
			slog.WarnContext(r.Context(), "baseline upstream failed", "err", err)
			problem.Write(w, http.StatusBadGateway, "booking unavailable", nil)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), UpstreamTimeout)
		defer cancel()
		rp.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ParseUpstream validates the Booking base URL.
func ParseUpstream(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("GATEWAY_BOOKING_URL must be an http(s) base URL, got %q", raw)
	}
	return u, nil
}

// Mount serves Route on rt, forwarding to upstream.
func Mount(rt *app.Runtime, upstream *url.URL) {
	rt.Handle(Route, metrics.NewHTTP(rt.Metrics).Wrap(Route, NewProxy(upstream)))
}
