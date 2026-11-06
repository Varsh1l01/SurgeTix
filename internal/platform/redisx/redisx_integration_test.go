//go:build integration

package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Mirrors the Compose Redis: same image, password required.
func TestCheckWithPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ctr, err := testcontainers.Run(ctx, "redis:7.4.12",
		testcontainers.WithCmd("redis-server", "--requirepass", "it-redis"),
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections")),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	addr, err := ctr.PortEndpoint(ctx, "6379/tcp", "")
	if err != nil {
		t.Fatal(err)
	}

	good := New(addr, "it-redis")
	defer good.Close()
	if err := Check(good).Fn(ctx); err != nil {
		t.Errorf("ping with correct password: %v", err)
	}

	bad := New(addr, "wrong")
	defer bad.Close()
	if err := Check(bad).Fn(ctx); err == nil {
		t.Error("ping with wrong password should fail")
	}
}
