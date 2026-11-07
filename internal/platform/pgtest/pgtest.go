// Package pgtest gives integration tests a real PostgreSQL 17.11 initialised
// with the Compose init script, so the per-service roles exist (ADR-029,
// ADR-033). It is imported only by tests and is never linked into a service
// binary.
//
// Use it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }
//
// and get a fresh database per test with NewDatabase, connected as the
// service's own role.
package pgtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the same PostgreSQL version as Compose.
const Image = "postgres:17.11"

// Role is a service login role created by the Compose init script.
type Role struct{ Name, Password string }

// Booking is the Booking service's role (owns booking_db in Compose).
var Booking = Role{Name: "booking_svc", Password: "it-booking"}

var (
	adminDSN string
	seq      atomic.Int64
)

// Run starts the container, runs the tests, terminates the container (even
// with TESTCONTAINERS_RYUK_DISABLED=true) and returns the exit code.
func Run(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	ctr, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("it-admin"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithInitScripts(initScript()),
		testcontainers.WithEnv(map[string]string{
			"IDENTITY_DB_PASSWORD": "it-identity",
			"CATALOG_DB_PASSWORD":  "it-catalog",
			"BOOKING_DB_PASSWORD":  Booking.Password,
			"CHECKOUT_DB_PASSWORD": "it-checkout",
		}),
		tcpostgres.BasicWaitStrategies(),
	)
	if err == nil {
		adminDSN, err = ctr.ConnectionString(ctx, "sslmode=disable")
	}
	cancel()
	code := 1
	if err != nil {
		fmt.Fprintln(os.Stderr, "pgtest: start postgres:", err)
	} else {
		code = m.Run()
	}
	if ctr != nil {
		_ = ctr.Terminate(context.Background())
	}
	return code
}

// initScript locates the Compose init script from this file's position, so
// it works from any package directory.
func initScript() string {
	_, self, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(self), "..", "..", "..", "deploy", "compose", "postgres-init", "01-service-databases.sh")
}

// NewDatabase creates an empty database owned by role and returns a pool
// connected as that role. The pool closes when the test ends.
func NewDatabase(t *testing.T, role Role) *pgxpool.Pool {
	t.Helper()
	if adminDSN == "" {
		t.Fatal("pgtest: call pgtest.Run from TestMain first")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), seq.Add(1))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" OWNER "+pgx.Identifier{role.Name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role.Name, role.Password)
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
