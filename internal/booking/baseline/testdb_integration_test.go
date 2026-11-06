//go:build unsafe_baseline && integration

package baseline

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// One container per package run: the same PostgreSQL 17.11 image and init
// script as Compose. Tests connect as booking_svc, the Booking service's own
// role, to booking_db (ADR-008, ADR-033).
var (
	dbOnce sync.Once
	dbDSN  string
	dbErr  error
	dbCtr  *tcpostgres.PostgresContainer
)

// TestMain terminates the shared container even when Ryuk is disabled
// (TESTCONTAINERS_RYUK_DISABLED=true on some local Docker Desktop setups).
func TestMain(m *testing.M) {
	code := m.Run()
	if dbCtr != nil {
		_ = dbCtr.Terminate(context.Background())
	}
	os.Exit(code)
}

const testBookingPassword = "it-booking"

func startPostgres(t *testing.T) string {
	t.Helper()
	dbOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		script := filepath.Join("..", "..", "..", "deploy", "compose", "postgres-init", "01-service-databases.sh")
		ctr, err := tcpostgres.Run(ctx, "postgres:17.11",
			tcpostgres.WithUsername("postgres"),
			tcpostgres.WithPassword("it-admin"),
			tcpostgres.WithDatabase("postgres"),
			tcpostgres.WithInitScripts(script),
			testcontainers.WithEnv(map[string]string{
				"IDENTITY_DB_PASSWORD": "it-identity",
				"CATALOG_DB_PASSWORD":  "it-catalog",
				"BOOKING_DB_PASSWORD":  testBookingPassword,
				"CHECKOUT_DB_PASSWORD": "it-checkout",
			}),
			tcpostgres.BasicWaitStrategies(),
		)
		dbCtr = ctr
		if err != nil {
			dbErr = err
			return
		}
		host, err := ctr.Host(ctx)
		if err != nil {
			dbErr = err
			return
		}
		port, err := ctr.MappedPort(ctx, "5432/tcp")
		if err != nil {
			dbErr = err
			return
		}
		dbDSN = "postgres://booking_svc:" + testBookingPassword + "@" + host + ":" + port.Port() + "/booking_db?sslmode=disable"
	})
	if dbErr != nil {
		t.Fatalf("start postgres: %v", dbErr)
	}
	return dbDSN
}

// openStore connects with a pool large enough for maxConns concurrent
// transactions (a barrier of N buyers holds N transactions open at once),
// migrates and resets.
func openStore(t *testing.T, maxConns int) (*Store, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = int32(maxConns)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := NewStore(pool)
	if err := s.Reset(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	return s, pool
}
