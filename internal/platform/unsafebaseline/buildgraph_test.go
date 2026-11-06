package unsafebaseline

import (
	"os/exec"
	"strings"
	"testing"
)

// The strongest form of "normal builds must not expose baseline endpoints":
// a normal build of Booking or Gateway does not even link the baseline
// packages. Only -tags unsafe_baseline pulls them in.
func TestBaselinePackagesOnlyInTaggedBuilds(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	const mod = "github.com/Varsh1l01/SurgeTix/"
	cases := map[string]string{
		mod + "cmd/booking": mod + "internal/booking/baseline",
		mod + "cmd/gateway": mod + "internal/gateway/baseline",
	}
	deps := func(pkg string, tags ...string) string {
		args := append([]string{"list", "-deps"}, tags...)
		out, err := exec.Command("go", append(args, pkg)...).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		return string(out)
	}
	for cmd, unsafePkg := range cases {
		if strings.Contains(deps(cmd), unsafePkg+"\n") {
			t.Errorf("normal build of %s links %s", cmd, unsafePkg)
		}
		if !strings.Contains(deps(cmd, "-tags", BuildTag), unsafePkg+"\n") {
			t.Errorf("tagged build of %s does not link %s (test is not checking anything)", cmd, unsafePkg)
		}
	}
}
