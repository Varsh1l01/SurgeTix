package kafka

import (
	"context"
	"testing"
	"time"
)

func TestNewClientIsLazyAndCheckFailsWithoutBroker(t *testing.T) {
	client, err := NewClient([]string{"127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewClient should not connect eagerly: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Check(client).Fn(ctx); err == nil {
		t.Error("readiness check should fail when no broker is reachable")
	}
}
