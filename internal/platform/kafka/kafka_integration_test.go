//go:build integration

package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Starts the same Kafka image as Compose in its default single-node KRaft
// mode and checks that the client reaches the broker.
func TestCheckAgainstKRaftBroker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ctr, err := testcontainers.Run(ctx, "apache/kafka:4.4.0",
		testcontainers.WithExposedPorts("9092/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Kafka Server started").WithStartupTimeout(2*time.Minute)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start kafka: %v", err)
	}
	addr, err := ctr.PortEndpoint(ctx, "9092/tcp", "")
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewClient([]string{addr})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := Check(client).Fn(ctx); err != nil {
		t.Errorf("broker ping: %v", err)
	}
}
