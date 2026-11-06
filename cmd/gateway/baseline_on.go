//go:build unsafe_baseline

package main

import (
	"log/slog"

	"github.com/Varsh1l01/SurgeTix/internal/gateway/baseline"
	"github.com/Varsh1l01/SurgeTix/internal/platform/app"
	"github.com/Varsh1l01/SurgeTix/internal/platform/config"
	"github.com/Varsh1l01/SurgeTix/internal/platform/unsafebaseline"
)

// setupBaseline forwards the unsafe baseline route to Booking only when the
// runtime flag is set. This file exists only with -tags unsafe_baseline.
func setupBaseline(env *config.Env, rt *app.Runtime) error {
	on, err := unsafebaseline.Enabled(env)
	if err != nil {
		return err
	}
	if !on {
		slog.Info("unsafe baseline compiled in but disabled", "flag", unsafebaseline.EnvFlag)
		return nil
	}
	upstream, err := baseline.ParseUpstream(env.Required("GATEWAY_BOOKING_URL"))
	if err != nil {
		return err
	}
	slog.Warn("UNSAFE NAIVE BASELINE ENABLED: forwarding the baseline route to Booking; testing and benchmarking only (ADR-034)",
		"route", baseline.Route, "upstream", upstream.String(), "deadline", baseline.UpstreamTimeout.String())
	baseline.Mount(rt, upstream)
	return nil
}
