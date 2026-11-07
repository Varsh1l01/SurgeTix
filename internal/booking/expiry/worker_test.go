package expiry

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestDefaultsFollowThePDF(t *testing.T) {
	w, err := New(nil, nil, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if w.cfg.BatchSize != 1000 || w.cfg.Interval != 2*time.Second || w.cfg.ShutdownTimeout != 10*time.Second || w.cfg.Logger == nil {
		t.Fatalf("defaults %+v", w.cfg)
	}
	w, _ = New(nil, nil, Config{BatchSize: 5, Interval: time.Second, ShutdownTimeout: time.Minute})
	if w.cfg.BatchSize != 5 || w.cfg.Interval != time.Second || w.cfg.ShutdownTimeout != time.Minute {
		t.Fatalf("overrides %+v", w.cfg)
	}
}

func TestMetricsRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	if _, err := New(nil, reg, Config{}); err != nil {
		t.Fatal(err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"booking_expiry_batches_total": true, "booking_holds_expired_total": true,
		"booking_expiry_partial_total": true, "booking_expiry_lag_seconds": true,
	}
	for _, mf := range mfs {
		delete(want, mf.GetName())
	}
	if len(want) != 0 {
		t.Fatalf("not registered: %v", want)
	}
	// A second worker on the same registry is refused, not silently merged.
	if _, err := New(nil, reg, Config{}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
}
