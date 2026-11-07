package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIdleInTransactionTimeoutIsTheApprovedValue(t *testing.T) {
	if IdleInTransactionTimeout != 5*time.Second {
		t.Fatalf("IdleInTransactionTimeout = %v, want the approved 5s", IdleInTransactionTimeout)
	}
}

func TestRuntimePoolConfigSetsOnlyTheTimeout(t *testing.T) {
	const dsn = "postgres://booking_svc:pw@localhost:15432/booking_db?application_name=booking"
	rt, err := runtimePoolConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"]; got != "5000ms" {
		t.Fatalf("runtime param %q, want 5000ms", got)
	}
	plain, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, set := plain.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"]; set {
		t.Fatal("the plain pool config must not carry the timeout")
	}
	// Everything else matches the plain configuration.
	if rt.MaxConns != plain.MaxConns || rt.ConnConfig.Host != plain.ConnConfig.Host ||
		rt.ConnConfig.Database != plain.ConnConfig.Database || rt.ConnConfig.User != plain.ConnConfig.User ||
		rt.ConnConfig.RuntimeParams["application_name"] != "booking" || len(rt.ConnConfig.RuntimeParams) != len(plain.ConnConfig.RuntimeParams)+1 {
		t.Fatalf("runtime config differs beyond the timeout: %+v", rt.ConnConfig.RuntimeParams)
	}
}

func TestRuntimePoolConfigNeverEchoesTheDSN(t *testing.T) {
	_, err := runtimePoolConfig("postgres://u:s3cret@%zz")
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("got %v", err)
	}
}
