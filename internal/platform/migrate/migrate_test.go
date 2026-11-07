package migrate

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpValidatesOptions(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{"00001_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}
	// A lazy pool: no connection is attempted, so validation errors come first.
	pool, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/db")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := Up(ctx, nil, fsys, Options{LockID: 1}); err == nil {
		t.Error("nil pool must be rejected")
	}
	if _, err := Up(ctx, pool, nil, Options{LockID: 1}); err == nil {
		t.Error("nil fsys must be rejected")
	}
	if _, err := Up(ctx, pool, fsys, Options{}); err == nil {
		t.Error("LockID 0 must be rejected (every replica must contend on one fixed lock)")
	}
}
