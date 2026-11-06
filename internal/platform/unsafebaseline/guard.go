// Package unsafebaseline is the two-key safety guard for the Phase 2 naive
// baseline (ADR-034, AMB-41).
//
// The baseline deliberately oversells. It is reachable only when BOTH keys
// are turned:
//
//  1. the binary is built with the Go build tag "unsafe_baseline", so a normal
//     build does not even contain the baseline code; and
//  2. SURGETIX_ENABLE_UNSAFE_BASELINE=true at runtime (default false).
//
// Setting the flag on a binary built without the tag is a configuration
// error: the service refuses to start rather than silently ignoring it.
package unsafebaseline

import (
	"errors"

	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
)

// EnvFlag is the runtime key.
const EnvFlag = "SURGETIX_ENABLE_UNSAFE_BASELINE"

// BuildTag is the compile-time key.
const BuildTag = "unsafe_baseline"

// ErrNotCompiled is returned when the flag is set on a normal build.
var ErrNotCompiled = errors.New(EnvFlag + "=true, but this binary was built without -tags " + BuildTag + "; the unsafe baseline is not available")

// Enabled reports whether the baseline may run. Errors are recorded on env.
func Enabled(env *config.Env) (bool, error) {
	if !env.Bool(EnvFlag, false) {
		return false, nil
	}
	if !Compiled {
		return false, ErrNotCompiled
	}
	return true, nil
}
