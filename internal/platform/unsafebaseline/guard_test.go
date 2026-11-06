package unsafebaseline

import (
	"errors"
	"testing"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
)

// Runs in both builds: `go test` and `go test -tags unsafe_baseline`.
func TestEnabledRequiresBothKeys(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		want    bool
		wantErr error
	}{
		{"flag unset", map[string]string{}, false, nil},
		{"flag empty", map[string]string{EnvFlag: ""}, false, nil},
		{"flag false", map[string]string{EnvFlag: "false"}, false, nil},
		{"flag true", map[string]string{EnvFlag: "true"}, Compiled, map[bool]error{true: nil, false: ErrNotCompiled}[Compiled]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Enabled(config.FromMap(c.env))
			if got != c.want || !errors.Is(err, c.wantErr) {
				t.Fatalf("Enabled = %v, %v; want %v, %v (Compiled=%v)", got, err, c.want, c.wantErr, Compiled)
			}
		})
	}
}

func TestInvalidFlagIsAConfigError(t *testing.T) {
	env := config.FromMap(map[string]string{EnvFlag: "yes please"})
	if got, _ := Enabled(env); got {
		t.Fatal("an unparsable flag must never enable the baseline")
	}
	if env.Err() == nil {
		t.Fatal("an unparsable flag must be reported as a configuration error")
	}
}
