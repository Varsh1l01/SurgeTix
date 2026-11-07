package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

// Runs without Docker: the embedded set is exactly the four approved
// migrations, in order, and each has an Up and a Down section.
func TestEmbeddedMigrationSet(t *testing.T) {
	want := []string{
		"00001_inventory_and_holds.sql",
		"00002_bookings_and_tickets.sql",
		"00003_shared.sql",
		"00004_idempotency_fencing.sql",
	}
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("embedded migrations = %v, want %v", got, want)
	}
	for _, name := range want {
		b, _ := fs.ReadFile(FS(), name)
		s := string(b)
		if !strings.Contains(s, "-- +goose Up") || !strings.Contains(s, "-- +goose Down") {
			t.Errorf("%s lacks an Up or Down section", name)
		}
	}
	if LockID == 0 {
		t.Fatal("LockID must be a fixed non-zero value")
	}
}
