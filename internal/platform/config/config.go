// Package config reads service configuration from environment variables.
//
// Values are never hardcoded: anything that is a credential or an address of
// shared infrastructure must be supplied by the environment (or a local .env
// file during development). Missing required values are collected so a
// service reports every configuration problem at once and exits.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Env reads typed values from a lookup function and accumulates errors.
type Env struct {
	lookup func(string) (string, bool)
	errs   []error
}

// FromOS returns an Env backed by the process environment.
func FromOS() *Env { return &Env{lookup: os.LookupEnv} }

// FromMap returns an Env backed by a map. It is intended for tests.
func FromMap(m map[string]string) *Env {
	return &Env{lookup: func(k string) (string, bool) { v, ok := m[k]; return v, ok }}
}

// String returns the value of key, or def when it is unset or empty.
func (e *Env) String(key, def string) string {
	if v, ok := e.lookup(key); ok && v != "" {
		return v
	}
	return def
}

// Required returns the value of key and records an error when it is unset or empty.
func (e *Env) Required(key string) string {
	v, ok := e.lookup(key)
	if !ok || v == "" {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

// List returns a required comma-separated list, trimming blanks.
func (e *Env) List(key string) []string {
	raw := e.Required(key)
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if raw != "" && len(out) == 0 {
		e.errs = append(e.errs, fmt.Errorf("%s has no values", key))
	}
	return out
}

// Duration returns key parsed with time.ParseDuration, or def when unset.
func (e *Env) Duration(key string, def time.Duration) time.Duration {
	v, ok := e.lookup(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		e.errs = append(e.errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}

// Err returns all accumulated errors joined, or nil.
func (e *Env) Err() error { return errors.Join(e.errs...) }

// Common holds settings every SurgeTix process needs.
type Common struct {
	Service         string
	HTTPAddr        string
	LogLevel        string
	ShutdownTimeout time.Duration
}

// LoadCommon reads the shared settings. prefix scopes the listen address so
// several services can run from one environment (e.g. BOOKING_HTTP_ADDR).
func LoadCommon(e *Env, service, prefix, defaultAddr string) Common {
	return Common{
		Service:         service,
		HTTPAddr:        e.String(prefix+"_HTTP_ADDR", defaultAddr),
		LogLevel:        e.String("LOG_LEVEL", "info"),
		ShutdownTimeout: e.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}
