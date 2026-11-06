package redisx

import (
	"context"
	"testing"
	"time"
)

func TestCheckFailsWhenUnreachable(t *testing.T) {
	c := New("127.0.0.1:1", "")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	chk := Check(c)
	if chk.Name != "redis" {
		t.Errorf("name = %q", chk.Name)
	}
	if err := chk.Fn(ctx); err == nil {
		t.Error("readiness check should fail when Redis is unreachable")
	}
}
