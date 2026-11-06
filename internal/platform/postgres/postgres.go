// Package postgres opens a connection pool to the calling service's own database.
//
// Each service owns exactly one database and connects with its own role
// (ADR-008: no service reads another's tables). The DSN therefore always comes
// from a service-scoped variable such as BOOKING_DATABASE_URL, never a shared one.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// Open parses dsn and returns a pool. It does not connect eagerly: a service
// starts even if Postgres is not up yet, and /readyz reports the outage.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx errors can echo the DSN; never log credentials.
		return nil, fmt.Errorf("postgres: invalid connection string")
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Check returns a readiness probe that pings the pool.
func Check(pool *pgxpool.Pool) service.Check {
	return service.Check{Name: "postgres", Fn: pool.Ping}
}

// RegisterPoolMetrics exposes pool statistics on reg: the configured maximum
// (recorded with every benchmark, ADR-034 fairness) and acquire wait, the
// "DB pool wait" metric the PDF names (p.34).
func RegisterPoolMetrics(reg prometheus.Registerer, pool *pgxpool.Pool) {
	stat := func(f func(*pgxpool.Stat) float64) func() float64 {
		return func() float64 { return f(pool.Stat()) }
	}
	reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "pgxpool_max_conns", Help: "Configured maximum pool size."},
			stat(func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) })),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "pgxpool_acquired_conns", Help: "Connections currently in use."},
			stat(func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) })),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "pgxpool_total_conns", Help: "Open connections."},
			stat(func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) })),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "pgxpool_acquire_total", Help: "Successful acquires."},
			stat(func(s *pgxpool.Stat) float64 { return float64(s.AcquireCount()) })),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "pgxpool_empty_acquire_total", Help: "Acquires that had to wait because the pool was empty."},
			stat(func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) })),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "pgxpool_acquire_duration_seconds_total", Help: "Total time spent acquiring connections."},
			stat(func(s *pgxpool.Stat) float64 { return s.AcquireDuration().Seconds() })),
	)
}
