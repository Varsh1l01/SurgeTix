package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn, "error": slog.LevelError,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("expected error for unknown level")
	}
}

func TestNewWritesJSONWithService(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "booking", slog.LevelInfo)
	log.Debug("hidden")
	log.Info("started", "addr", ":8084")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not a single JSON record: %v\n%s", err, buf.String())
	}
	if rec["service"] != "booking" || rec["msg"] != "started" || rec["addr"] != ":8084" {
		t.Errorf("unexpected record %v", rec)
	}
}
