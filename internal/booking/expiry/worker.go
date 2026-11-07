// Package expiry is Booking's hold-expiry worker (PDF p.30; plan §8.1;
// M1-T11). Each batch is one transaction running holds.ExpireBatch: lock up
// to BatchSize expired holds (SKIP LOCKED), return the seats they still own
// (SKIP LOCKED), expire the drained ones, write the outbox events, commit.
//
// Pacing (plan §8.1): while batches come back full, the next batch runs at
// once; otherwise the worker waits Interval (2 s). Shutdown: when the run
// context ends, no new batch starts, and an in-flight batch is allowed up
// to ShutdownTimeout to commit before it is cancelled (and rolled back).
//
// Several workers may run at once (replicas): SKIP LOCKED gives each batch
// a disjoint set of holds and seats, and every change is conditional, so
// nothing is released or expired twice.
package expiry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Varsh1l01/SurgeTix/internal/booking/holds"
)

// PDF p.30: every 2 s, batches of 1,000.
const (
	DefaultBatchSize = 1000
	DefaultInterval  = 2 * time.Second
	// DefaultShutdownTimeout matches the services' SHUTDOWN_TIMEOUT default.
	DefaultShutdownTimeout = 10 * time.Second
)

// Config tunes the worker. Zero values take the defaults.
type Config struct {
	BatchSize       int
	Interval        time.Duration
	ShutdownTimeout time.Duration
	Logger          *slog.Logger
}

// Worker runs expiry batches against Booking's database.
type Worker struct {
	pool *pgxpool.Pool
	cfg  Config
	m    metrics

	// beforeCommit, if set, runs after a batch's work and before its
	// COMMIT. Tests use it to stall or kill a batch; nil in production.
	beforeCommit func(ctx context.Context, tx pgx.Tx) error
}

// Metrics of plan §12 (Expiry row).
type metrics struct {
	batches prometheus.Counter
	expired prometheus.Counter
	partial prometheus.Counter
	lag     prometheus.Gauge
}

// New returns a worker. reg receives its metrics; nil leaves them
// unregistered (they still count, e.g. for tests).
func New(pool *pgxpool.Pool, reg prometheus.Registerer, cfg Config) (*Worker, error) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	m := metrics{
		batches: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "booking_expiry_batches_total", Help: "Expiry batches committed."}),
		expired: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "booking_holds_expired_total", Help: "Holds marked EXPIRED by the expiry worker."}),
		partial: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "booking_expiry_partial_total", Help: "Expired holds a batch locked but could not drain (a seat was locked elsewhere); retried by a later batch."}),
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "booking_expiry_lag_seconds", Help: "Age of the oldest expired, unprocessed hold (0 if none)."}),
	}
	if reg != nil {
		for _, c := range []prometheus.Collector{m.batches, m.expired, m.partial, m.lag} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("expiry: register metrics: %w", err)
			}
		}
	}
	return &Worker{pool: pool, cfg: cfg, m: m}, nil
}

// RunOnce runs one batch in one transaction and commits it. On any error
// the batch rolls back completely, and a rerun is safe (plan §10.5).
func (w *Worker) RunOnce(ctx context.Context) (holds.ExpireResult, error) {
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return holds.ExpireResult{}, fmt.Errorf("expiry: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }() // no-op after Commit
	res, err := holds.ExpireBatch(ctx, tx, w.cfg.BatchSize)
	if err != nil {
		return holds.ExpireResult{}, err
	}
	if w.beforeCommit != nil {
		if err := w.beforeCommit(ctx, tx); err != nil {
			return holds.ExpireResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return holds.ExpireResult{}, fmt.Errorf("expiry: commit: %w", err)
	}
	w.m.batches.Inc()
	w.m.expired.Add(float64(res.Expired))
	w.m.partial.Add(float64(res.Partial))
	if err := w.updateLag(ctx); err != nil {
		w.cfg.Logger.Warn("expiry: lag metric not updated", "err", err)
	}
	return res, nil
}

func (w *Worker) updateLag(ctx context.Context) error {
	var lag float64
	err := w.pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM now() - min(expires_at)), 0)::float8 FROM holds
		 WHERE status IN ('ACTIVE','PAYMENT_PENDING') AND expires_at < now() AND kind = 'SEATS'`).Scan(&lag)
	if err != nil {
		return err
	}
	w.m.lag.Set(lag)
	return nil
}

// Run runs batches until ctx ends, paced as described in the package
// comment. It returns nil on a normal stop. A failed batch is logged and
// retried after Interval.
func (w *Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		res, err := w.runGuarded(ctx)
		switch {
		case err != nil:
			if ctx.Err() == nil {
				w.cfg.Logger.Error("expiry: batch failed", "err", err)
			}
		case res.Candidates >= w.cfg.BatchSize:
			continue // a full batch: more may be waiting, run again now
		}
		t := time.NewTimer(w.cfg.Interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

// runGuarded runs one batch on a context that survives the stop signal for
// up to ShutdownTimeout, so an in-flight batch can still commit.
func (w *Worker) runGuarded(ctx context.Context) (holds.ExpireResult, error) {
	bctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			t := time.NewTimer(w.cfg.ShutdownTimeout)
			defer t.Stop()
			select {
			case <-done:
			case <-t.C:
				cancel()
			}
		}
	}()
	res, err := w.RunOnce(bctx)
	if err != nil && errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return res, fmt.Errorf("expiry: batch cancelled after the %v shutdown timeout: %w", w.cfg.ShutdownTimeout, err)
	}
	return res, err
}
