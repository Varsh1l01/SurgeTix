//go:build !unsafe_baseline

package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

// setupBaseline in a normal build: the baseline code is not compiled in. It
// registers nothing, and refuses to start if the runtime flag asks for it.
func setupBaseline(_ context.Context, env *config.Env, _ *app.Runtime, _ *pgxpool.Pool) error {
	_, err := unsafebaseline.Enabled(env)
	return err
}
