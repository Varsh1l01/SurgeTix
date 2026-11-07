//go:build integration

package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Varsh1l01/SurgeTix/internal/platform/migrate"
)

// The reference DDL (pdfDDL, checksum-pinned) and approvedDeviation live in
// pdfddl_test.go, which also runs without Docker.

// catalog is a comparable snapshot of everything that defines the schema:
// tables, columns (type, nullability, default), constraints (definition),
// indexes (definition) and table storage options.
func catalog(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	ctx := context.Background()
	var out []string
	q := func(prefix, sql string) {
		rows, err := pool.Query(ctx, sql)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				t.Fatal(err)
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				if v == nil {
					parts[i] = "<null>"
				} else {
					parts[i] = strings.TrimSpace(strings.ReplaceAll(toString(v), "\n", " "))
				}
			}
			out = append(out, prefix+" "+strings.Join(parts, " | "))
		}
	}
	const notGoose = `NOT IN ('goose_db_version')`
	q("column", `SELECT table_name, column_name, data_type, is_nullable, coalesce(column_default, ''), ordinal_position
		FROM information_schema.columns WHERE table_schema = 'public' AND table_name `+notGoose)
	q("constraint", `SELECT conrelid::regclass::text, conname, contype::text, pg_get_constraintdef(oid)
		FROM pg_constraint WHERE connamespace = 'public'::regnamespace AND conrelid::regclass::text `+notGoose)
	q("index", `SELECT tablename, indexname, indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename `+notGoose)
	q("storage", `SELECT relname, coalesce(array_to_string(reloptions, ','), '') FROM pg_class
		WHERE relnamespace = 'public'::regnamespace AND relkind = 'r' AND relname `+notGoose)
	sort.Strings(out)
	return out
}

// toString renders a catalog value; text columns come back as strings,
// numbers via fmt.
func toString(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

func up(t *testing.T, pool *pgxpool.Pool) []migrate.Result {
	t.Helper()
	res, err := Up(context.Background(), pool, nil)
	if err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return res
}

// Layer 1: the migrated schema equals the PDF DDL plus the approved deviation.
func TestSchemaMatchesPDFDDL(t *testing.T) {
	migrated := newBookingDB(t)
	if res := up(t, migrated); len(res) != 4 {
		t.Fatalf("applied %d migrations, want 4", len(res))
	}

	reference := newBookingDB(t)
	ctx := context.Background()
	if _, err := reference.Exec(ctx, pdfDDL(t)); err != nil {
		t.Fatalf("executing the PDF DDL: %v", err)
	}
	if _, err := reference.Exec(ctx, approvedDeviation); err != nil {
		t.Fatal(err)
	}

	got, want := catalog(t, migrated), catalog(t, reference)
	if len(got) < 60 {
		t.Fatalf("catalog snapshot suspiciously small (%d lines); the comparison would prove nothing", len(got))
	}
	gotSet, wantSet := map[string]bool{}, map[string]bool{}
	for _, l := range got {
		gotSet[l] = true
	}
	for _, l := range want {
		wantSet[l] = true
	}
	var missing, extra []string
	for _, l := range want {
		if !gotSet[l] {
			missing = append(missing, l)
		}
	}
	for _, l := range got {
		if !wantSet[l] {
			extra = append(extra, l)
		}
	}
	if len(missing)+len(extra) > 0 {
		t.Fatalf("schema differs from the PDF DDL:\nmissing (in PDF, not migrated):\n  %s\nextra (migrated, not in PDF):\n  %s",
			strings.Join(missing, "\n  "), strings.Join(extra, "\n  "))
	}
	t.Logf("migrated schema identical to PDF DDL + approved deviation (%d catalog entries compared)", len(got))
}

// Layer 2: the invariants that must never be weakened (CLAUDE.md invariant 4)
// are named explicitly, independent of the docs file, and exercised.
func TestNamedInvariantsPresentAndEnforced(t *testing.T) {
	pool := newBookingDB(t)
	up(t, pool)
	ctx := context.Background()
	cat := strings.Join(catalog(t, pool), "\n")
	for _, must := range []string{
		"CHECK (((remaining >= 0) AND (remaining <= capacity)))",
		"one_live_ticket_per_seat | CREATE UNIQUE INDEX one_live_ticket_per_seat ON public.tickets USING btree (event_id, seat_id) WHERE ((seat_id IS NOT NULL) AND (status <> 'CANCELLED'::text))",
		"bookings_order_id_key | u | UNIQUE (order_id)",
		"bookings_hold_id_key | u | UNIQUE (hold_id)",
		"holds_order_id_key | u | UNIQUE (order_id)",
		"ticket_transfers_ticket_id_key | u | UNIQUE (ticket_id)",
		"CHECK (((transfer_count >= 0) AND (transfer_count <= 1)))",
		"CHECK (((status = 'AVAILABLE'::text) = (hold_id IS NULL)))",
		"CHECK (((status = 'HELD'::text) = (hold_expires_at IS NOT NULL)))",
		"hold_ga_allocations_pkey | p | PRIMARY KEY (hold_id, section, bucket)",
		"idempotency_keys_pkey | p | PRIMARY KEY (scope, user_id, key)",
		"seats_held_expiry | CREATE INDEX seats_held_expiry ON public.seats USING btree (hold_expires_at) WHERE (status = 'HELD'::text)",
		"seats_free_by_row | CREATE INDEX seats_free_by_row ON public.seats USING btree (event_id, section, row_label, seat_number) WHERE (status = 'AVAILABLE'::text)",
		"holds_expiring | CREATE INDEX holds_expiring ON public.holds USING btree (expires_at) WHERE (status = ANY (ARRAY['ACTIVE'::text, 'PAYMENT_PENDING'::text]))",
		"outbox_ready | CREATE INDEX outbox_ready ON public.outbox USING btree (next_attempt_at) WHERE (status = ANY (ARRAY['PENDING'::text, 'IN_FLIGHT'::text]))",
		"storage seats | fillfactor=80",
		"column idempotency_keys | claim_generation | bigint | NO | 1",
		"column idempotency_keys | claimed_at | timestamp with time zone | NO | now()",
	} {
		if !strings.Contains(cat, must) {
			t.Errorf("missing invariant: %s", must)
		}
	}

	// Behaviour: the database itself refuses what the invariants forbid.
	expectCode := func(name, code, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != code {
			t.Errorf("%s: want SQLSTATE %s, got %v", name, code, err)
		}
	}
	const ev = "5e1f0000-0000-4000-8000-0000000000aa"
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO ga_buckets VALUES ($1, 'GA', 0, 10, 10)`, ev)
	expectCode("GA remaining below zero", "23514", `UPDATE ga_buckets SET remaining = -1 WHERE event_id = $1`, ev)
	expectCode("GA remaining above capacity", "23514", `UPDATE ga_buckets SET remaining = 11 WHERE event_id = $1`, ev)

	mustExec(`INSERT INTO seats (event_id, seat_id, section, row_label, seat_number, price_paise) VALUES ($1, 'A-R01-01', 'A', 'R01', 1, 100000)`, ev)
	expectCode("HELD seat without expiry mirror", "23514", `UPDATE seats SET status = 'HELD', hold_id = gen_random_uuid() WHERE event_id = $1`, ev)
	expectCode("AVAILABLE seat with a hold id", "23514", `UPDATE seats SET hold_id = gen_random_uuid() WHERE event_id = $1`, ev)
	expectCode("negative price", "23514", `UPDATE seats SET price_paise = -1 WHERE event_id = $1`, ev)

	mustExec(`INSERT INTO holds VALUES ('00000000-0000-4000-8000-0000000000b1', $1, gen_random_uuid(), 'SEATS', 'CONVERTED', now(), NULL, now())`, ev)
	mustExec(`INSERT INTO bookings VALUES ('00000000-0000-4000-8000-0000000000c1', '00000000-0000-4000-8000-0000000000b1', '00000000-0000-4000-8000-0000000000d1', gen_random_uuid(), now())`)
	ticket := `INSERT INTO tickets (ticket_id, booking_id, event_id, seat_id, section, owner_user_id, status, qr_secret_enc)
		VALUES (gen_random_uuid(), '00000000-0000-4000-8000-0000000000c1', $1, 'A-R01-01', 'A', gen_random_uuid(), 'VALID', '\x00')`
	mustExec(ticket, ev)
	expectCode("second live ticket for the same seat", "23505", ticket, ev)
	expectCode("second booking for the same order", "23505",
		`INSERT INTO bookings VALUES (gen_random_uuid(), gen_random_uuid(), '00000000-0000-4000-8000-0000000000d1', gen_random_uuid(), now())`)
}

// Up -> down -> up on a fresh database: every Down works and Up is repeatable.
func TestUpDownUp(t *testing.T) {
	pool := newBookingDB(t)
	ctx := context.Background()
	up(t, pool)
	first := catalog(t, pool)

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, FS(),
		goose.WithSessionLocker(migrate.NewSessionLocker(LockID)), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	var left []string
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'goose_db_version' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		left = append(left, n)
	}
	rows.Close()
	if len(left) != 0 {
		t.Fatalf("tables left after down to 0: %v", left)
	}

	if res := up(t, pool); len(res) != 4 {
		t.Fatalf("second up applied %d migrations, want 4", len(res))
	}
	if second := catalog(t, pool); strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatal("schema after up/down/up differs from the first up")
	}
	if res := up(t, pool); len(res) != 0 {
		t.Fatalf("third up applied %d migrations, want 0", len(res))
	}
}
