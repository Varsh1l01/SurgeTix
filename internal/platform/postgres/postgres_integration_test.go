//go:build integration

package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Runs the real Compose init script and checks the ownership boundary:
// each service role reaches its own database and is refused by the others.
func TestServiceDatabasesAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pw := map[string]string{
		"IDENTITY_DB_PASSWORD": "it-identity",
		"CATALOG_DB_PASSWORD":  "it-catalog",
		"BOOKING_DB_PASSWORD":  "it-booking",
		"CHECKOUT_DB_PASSWORD": "it-checkout",
	}
	script := filepath.Join("..", "..", "..", "deploy", "compose", "postgres-init", "01-service-databases.sh")
	ctr, err := tcpostgres.Run(ctx, "postgres:17.11",
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("it-admin"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithInitScripts(script),
		testcontainers.WithEnv(pw),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	dsn := func(role, password, db string) string {
		return "postgres://" + role + ":" + password + "@" + host + ":" + port.Port() + "/" + db + "?sslmode=disable"
	}

	services := []struct{ role, db, password string }{
		{"identity_svc", "identity_db", pw["IDENTITY_DB_PASSWORD"]},
		{"catalog_svc", "catalog_db", pw["CATALOG_DB_PASSWORD"]},
		{"booking_svc", "booking_db", pw["BOOKING_DB_PASSWORD"]},
		{"checkout_svc", "checkout_db", pw["CHECKOUT_DB_PASSWORD"]},
	}
	for _, own := range services {
		for _, target := range services {
			pool, err := Open(ctx, dsn(own.role, own.password, target.db))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			err = Check(pool).Fn(ctx)
			pool.Close()

			if own.db == target.db {
				if err != nil {
					t.Errorf("%s -> %s: expected success, got %v", own.role, target.db, err)
				}
				continue
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" { // insufficient_privilege
				t.Errorf("%s -> %s: expected permission denied (42501), got %v", own.role, target.db, err)
			}
		}
	}
}
