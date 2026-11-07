package inventory

import (
	"regexp"
	"strings"
	"testing"
)

// Runs without Docker: audit.sql and audit.go name the same checks, every
// check documents its invariant and source, and the SQL contains only reads.
func TestAuditSQLAndInvariantsInSync(t *testing.T) {
	inSQL := map[Check]bool{}
	for _, m := range regexp.MustCompile(`(?m)^SELECT '([a-z_]+)'`).FindAllStringSubmatch(auditSQL, -1) {
		inSQL[Check(m[1])] = true
	}
	for c := range inSQL {
		if _, ok := Invariants[c]; !ok {
			t.Errorf("audit.sql emits %q, which has no invariant in audit.go", c)
		}
	}
	for c, desc := range Invariants {
		if !inSQL[c] {
			t.Errorf("check %q is documented but not in audit.sql", c)
		}
		if !strings.Contains(desc, "[") { // every invariant cites its source
			t.Errorf("check %q does not cite a source: %q", c, desc)
		}
	}
	if len(inSQL) != len(Invariants) || len(AllChecks()) != 10 {
		t.Fatalf("checks: sql=%d invariants=%d all=%d, want 10", len(inSQL), len(Invariants), len(AllChecks()))
	}
	// Read-only by construction (the READ ONLY transaction enforces it too).
	upper := strings.ToUpper(regexp.MustCompile(`--[^\n]*`).ReplaceAllString(auditSQL, ""))
	for _, w := range []string{"INSERT ", "UPDATE ", "DELETE ", "TRUNCATE ", "ALTER ", "DROP ", "CREATE ", "MERGE ", " INTO "} {
		if strings.Contains(upper, w) {
			t.Errorf("audit.sql contains %q; the audit must only read", strings.TrimSpace(w))
		}
	}
}
