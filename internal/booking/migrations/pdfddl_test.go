package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// pdfDDLSHA256 pins the reference DDL used by the schema-fidelity tests.
//
// It is the SHA-256 of canonicalDDL(booking_db section + shared-tables
// section) as transcribed in docs/04-data/postgres-schema.md. When it was
// recorded (2026-11-06), the same value was computed independently from the
// PDF's own text (p.17-21, page footers removed): 3,952 canonical characters,
// identical. So this pins the PDF DDL itself, not just the docs file.
//
// If this test fails, the reference SQL in the docs has changed. Do not just
// update the hash. A change to the PDF schema needs an approved ADR
// (CLAUDE.md P4); a docs typo must be fixed back to the PDF text.
const pdfDDLSHA256 = "acd2e8015d8d3998c2dcdffcefe7a5d5341c0f840f9b9c9d79fe95ed979d0c74"

// approvedDeviation is the ONLY approved change to the PDF DDL (ADR-036
// amendment, AMB-48). It is kept separate from the pinned PDF DDL and written
// out independently of migration 00004, so the reference does not depend on
// the file under test.
const approvedDeviation = `ALTER TABLE idempotency_keys
  ADD COLUMN claim_generation BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN claimed_at TIMESTAMPTZ NOT NULL DEFAULT now();`

// canonicalDDL removes all whitespace. Re-wrapping, re-indenting or changing
// line endings does not change the checksum, but any change to a token does
// (a keyword, name, type, constraint, default, or comment text).
func canonicalDDL(sql string) string { return strings.Join(strings.Fields(sql), "") }

func ddlChecksum(sql string) string {
	sum := sha256.Sum256([]byte(canonicalDDL(sql)))
	return hex.EncodeToString(sum[:])
}

// checkPDFDDL reports whether sql is the pinned PDF DDL.
func checkPDFDDL(sql string) error {
	if got := ddlChecksum(sql); got != pdfDDLSHA256 {
		return fmt.Errorf("reference DDL checksum %s does not match the pinned PDF DDL %s: "+
			"docs/04-data/postgres-schema.md changed (schema changes need an approved ADR)", got, pdfDDLSHA256)
	}
	return nil
}

// extractDocsDDL returns the booking_db and shared-table SQL blocks of
// docs/04-data/postgres-schema.md, without checking them.
func extractDocsDDL() (string, error) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "04-data", "postgres-schema.md"))
	if err != nil {
		return "", err
	}
	// Git may check the docs out with CRLF line endings on Windows.
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	block := func(from, to string) (string, error) {
		i := strings.Index(doc, from)
		if i < 0 {
			return "", fmt.Errorf("section %q not found", from)
		}
		j := strings.Index(doc[i:], to)
		if j < 0 {
			return "", fmt.Errorf("end of section %q not found", from)
		}
		sec := doc[i : i+j]
		s := strings.Index(sec, "```sql\n")
		if s < 0 {
			return "", fmt.Errorf("no sql block in section %q", from)
		}
		e := strings.Index(sec[s+7:], "```")
		if e < 0 {
			return "", fmt.Errorf("unterminated sql block in section %q", from)
		}
		return sec[s+7 : s+7+e], nil
	}
	booking, err := block("## booking_db", "## checkout_db")
	if err != nil {
		return "", err
	}
	shared, err := block("## Shared tables", "## Invariants")
	if err != nil {
		return "", err
	}
	return booking + "\n" + shared, nil
}

// pdfDDL returns the reference DDL for the fidelity tests, failing the test
// unless it matches the pinned PDF DDL checksum.
func pdfDDL(t *testing.T) string {
	t.Helper()
	sql, err := extractDocsDDL()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPDFDDL(sql); err != nil {
		t.Fatal(err)
	}
	return sql
}

func TestReferenceDDLMatchesPinnedPDFChecksum(t *testing.T) {
	sql, err := extractDocsDDL()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPDFDDL(sql); err != nil {
		t.Fatal(err)
	}
	// Characters, not bytes: the DDL contains one multi-byte rune ("→" in a comment).
	if n := utf8.RuneCountInString(canonicalDDL(sql)); n != 3952 {
		t.Fatalf("canonical reference DDL is %d characters, recorded 3952", n)
	}
}

func TestChecksumRejectsModifiedReferenceSQL(t *testing.T) {
	sql, err := extractDocsDDL()
	if err != nil {
		t.Fatal(err)
	}
	if checkPDFDDL(sql) != nil {
		t.Fatal("precondition: the unmodified reference must pass")
	}
	mutations := []struct{ name, old, new string }{
		{"weaken GA stock CHECK", "CHECK (remaining >= 0 AND remaining <= capacity)", "CHECK (remaining <= capacity)"},
		{"drop live-ticket partial unique index", "CREATE UNIQUE INDEX one_live_ticket_per_seat", "CREATE INDEX one_live_ticket_per_seat"},
		{"remove a UNIQUE", "order_id     UUID NOT NULL UNIQUE,", "order_id     UUID NOT NULL,"},
		{"change a type", "price_paise     BIGINT NOT NULL", "price_paise     INT NOT NULL"},
		{"add a column", "  section         TEXT   NOT NULL,", "  section         TEXT   NOT NULL,\n  sneaky          INT,"},
		{"remove a column", "  qr_secret_enc  BYTEA NOT NULL,\n", ""},
		{"change fillfactor", "fillfactor = 80", "fillfactor = 90"},
		{"fold the approved deviation into the reference", "  expires_at    TIMESTAMPTZ NOT NULL,\n  PRIMARY KEY (scope, user_id, key)",
			"  expires_at    TIMESTAMPTZ NOT NULL,\n  claim_generation BIGINT NOT NULL DEFAULT 1,\n  PRIMARY KEY (scope, user_id, key)"},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(sql, m.old) {
				t.Fatalf("mutation target %q not found; update the test, not the hash", m.old)
			}
			if err := checkPDFDDL(strings.Replace(sql, m.old, m.new, 1)); err == nil {
				t.Fatal("modified reference SQL was accepted")
			}
		})
	}
}

func TestChecksumIgnoresWhitespaceOnly(t *testing.T) {
	sql, err := extractDocsDDL()
	if err != nil {
		t.Fatal(err)
	}
	reformatted := strings.NewReplacer("\n", "\r\n", "  ", " \t ", "(", "(\n ").Replace(sql)
	if reformatted == sql {
		t.Fatal("precondition: the reformatting must change the text")
	}
	if err := checkPDFDDL(reformatted); err != nil {
		t.Fatalf("whitespace-only reformatting must be accepted: %v", err)
	}
}

func TestApprovedDeviationKeptSeparate(t *testing.T) {
	sql, err := extractDocsDDL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "claim_generation") || strings.Contains(sql, "claimed_at") {
		t.Fatal("the pinned PDF DDL must not contain the approved deviation's columns")
	}
	if !strings.Contains(approvedDeviation, "claim_generation") || !strings.Contains(approvedDeviation, "claimed_at") {
		t.Fatal("approvedDeviation must carry both fencing columns")
	}
	if checkPDFDDL(sql+"\n"+approvedDeviation) == nil {
		t.Fatal("the PDF DDL plus the deviation must not pass as the PDF DDL")
	}
}
