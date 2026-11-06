//go:build !unsafe_baseline

package main

import (
	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

// setupBaseline in a normal build: nothing is registered, and the runtime
// flag alone is a configuration error.
func setupBaseline(env *config.Env, _ *app.Runtime) error {
	_, err := unsafebaseline.Enabled(env)
	return err
}
