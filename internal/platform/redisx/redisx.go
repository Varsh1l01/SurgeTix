// Package redisx creates the Redis client used for queue state, caches,
// fencing and rate limits. Redis is never the source of truth for inventory
// or money (ADR-004); Postgres is.
package redisx

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// go-redis writes internal messages (e.g. dial retries) through its own
// printf-style logger. Route them into the structured slog default logger so
// every line a service emits stays JSON.
func init() { redis.SetLogger(slogLogger{}) }

type slogLogger struct{}

func (slogLogger) Printf(ctx context.Context, format string, v ...any) {
	slog.WarnContext(ctx, fmt.Sprintf(format, v...), "component", "go-redis")
}

// New returns a client for addr. An empty password means no AUTH.
// The client connects lazily on first use.
func New(addr, password string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr, Password: password})
}

// Check returns a readiness probe that sends PING.
func Check(c *redis.Client) service.Check {
	return service.Check{Name: "redis", Fn: func(ctx context.Context) error {
		return c.Ping(ctx).Err()
	}}
}
