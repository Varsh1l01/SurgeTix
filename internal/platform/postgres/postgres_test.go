package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestOpenRejectsInvalidDSNWithoutEchoingIt(t *testing.T) {
	_, err := Open(context.Background(), "postgres://user:s3cret@host:notaport/db")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %q", err)
	}
}

func TestOpenIsLazy(t *testing.T) {
	// Nothing listens on port 1. Open must still succeed so the service can
	// start and report the outage through /readyz instead of crashing.
	pool, err := Open(context.Background(), "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("Open should not connect eagerly: %v", err)
	}
	defer pool.Close()
	if err := Check(pool).Fn(context.Background()); err == nil {
		t.Error("readiness check should fail when Postgres is unreachable")
	}
}

func TestRegisterPoolMetrics(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://u:p@127.0.0.1:1/db?pool_max_conns=7")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	reg := prometheus.NewRegistry()
	RegisterPoolMetrics(reg, pool)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]float64{}
	for _, mf := range mfs {
		m := mf.GetMetric()[0]
		if g := m.GetGauge(); g != nil {
			found[mf.GetName()] = g.GetValue()
		} else if c := m.GetCounter(); c != nil {
			found[mf.GetName()] = c.GetValue()
		}
	}
	if found["pgxpool_max_conns"] != 7 || len(found) != 6 {
		t.Fatalf("metrics = %v", found)
	}
}
