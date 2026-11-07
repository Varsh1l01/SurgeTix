//go:build integration

package migrations

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// One PostgreSQL 17.11 container per package run, initialised with the real
// Compose script so the booking_svc role exists. Every test gets its own
// database OWNED BY booking_svc and connects as booking_svc, the Booking
// service's own role (ADR-033).
var (
	ctr      *tcpostgres.PostgresContainer
	adminDSN string
	dbSeq    atomic.Int64
)

const bookingPassword = "it-booking"

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	var err error
	ctr, err = tcpostgres.Run(ctx, "postgres:17.11",
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("it-admin"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithInitScripts(filepath.Join("..", "..", "..", "deploy", "compose", "postgres-init", "01-service-databases.sh")),
		testcontainers.WithEnv(map[string]string{
			"IDENTITY_DB_PASSWORD": "it-identity", "CATALOG_DB_PASSWORD": "it-catalog",
			"BOOKING_DB_PASSWORD": bookingPassword, "CHECKOUT_DB_PASSWORD": "it-checkout",
		}),
		tcpostgres.BasicWaitStrategies(),
	)
	if err == nil {
		adminDSN, err = ctr.ConnectionString(ctx, "sslmode=disable")
	}
	cancel()
	code := 1
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
	} else {
		code = m.Run()
	}
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	os.Exit(code)
}

// newBookingDB creates an empty database owned by booking_svc and returns a
// booking_svc pool on it.
func newBookingDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("bk_%d_%d", time.Now().UnixNano(), dbSeq.Add(1))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" OWNER booking_svc"); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("booking_svc", bookingPassword)
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
