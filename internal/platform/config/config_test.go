package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequiredCollectsAllMissing(t *testing.T) {
	e := FromMap(map[string]string{"PRESENT": "x", "EMPTY": ""})
	if got := e.Required("PRESENT"); got != "x" {
		t.Fatalf("Required(PRESENT) = %q", got)
	}
	e.Required("MISSING")
	e.Required("EMPTY")
	err := e.Err()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, k := range []string{"MISSING", "EMPTY"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not mention %s", err, k)
		}
	}
}

func TestStringDefault(t *testing.T) {
	e := FromMap(map[string]string{"SET": "v", "BLANK": ""})
	cases := map[string]string{"SET": "v", "BLANK": "def", "UNSET": "def"}
	for k, want := range cases {
		if got := e.String(k, "def"); got != want {
			t.Errorf("String(%s) = %q, want %q", k, got, want)
		}
	}
	if e.Err() != nil {
		t.Fatalf("String must not record errors: %v", e.Err())
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"single", "localhost:9092", []string{"localhost:9092"}, false},
		{"multiple with spaces", " a:1 , b:2 ,", []string{"a:1", "b:2"}, false},
		{"only commas", ",,", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := FromMap(map[string]string{"K": tt.raw})
			got := e.List("K")
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("List = %v, want %v", got, tt.want)
			}
			if (e.Err() != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", e.Err(), tt.wantErr)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	e := FromMap(map[string]string{"OK": "3s", "BAD": "soon", "NEG": "-1s"})
	if got := e.Duration("OK", time.Second); got != 3*time.Second {
		t.Errorf("OK = %v", got)
	}
	if got := e.Duration("UNSET", time.Second); got != time.Second {
		t.Errorf("UNSET = %v", got)
	}
	e.Duration("BAD", time.Second)
	e.Duration("NEG", time.Second)
	if err := e.Err(); err == nil || !strings.Contains(err.Error(), "BAD") || !strings.Contains(err.Error(), "NEG") {
		t.Errorf("expected errors for BAD and NEG, got %v", err)
	}
}

func TestLoadCommon(t *testing.T) {
	e := FromMap(map[string]string{"BOOKING_HTTP_ADDR": ":9999", "LOG_LEVEL": "debug"})
	c := LoadCommon(e, "booking", "BOOKING", ":8084")
	if c.HTTPAddr != ":9999" || c.LogLevel != "debug" || c.ShutdownTimeout != 10*time.Second || c.Service != "booking" {
		t.Errorf("unexpected %+v", c)
	}
	d := LoadCommon(FromMap(nil), "booking", "BOOKING", ":8084")
	if d.HTTPAddr != ":8084" || d.LogLevel != "info" {
		t.Errorf("unexpected defaults %+v", d)
	}
}

func TestParseDotEnv(t *testing.T) {
	in := "# comment\n\nA=1\nexport B = two \nC=\"quoted value\"\nD='single'\nE=\nF=a=b\n"
	got, err := parseDotEnv(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two", "C": "quoted value", "D": "single", "E": "", "F": "a=b"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, err := parseDotEnv(strings.NewReader("NOEQUALS\n")); err == nil {
		t.Error("expected error for line without '='")
	}
}

func TestLoadDotEnvDoesNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("SURGETIX_TEST_KEEP=file\nSURGETIX_TEST_NEW=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SURGETIX_TEST_KEEP", "process")
	t.Setenv("SURGETIX_TEST_NEW", "")
	os.Unsetenv("SURGETIX_TEST_NEW")

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("SURGETIX_TEST_KEEP"); v != "process" {
		t.Errorf("existing variable overridden: %q", v)
	}
	if v := os.Getenv("SURGETIX_TEST_NEW"); v != "file" {
		t.Errorf("new variable not loaded: %q", v)
	}
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Errorf("missing file should be ignored: %v", err)
	}
}

func TestBool(t *testing.T) {
	cases := []struct {
		val     string
		set     bool
		def     bool
		want    bool
		wantErr bool
	}{
		{set: false, def: false, want: false},
		{set: false, def: true, want: true},
		{val: "", set: true, def: false, want: false},
		{val: "true", set: true, want: true},
		{val: "1", set: true, want: true},
		{val: "false", set: true, def: true, want: false},
		{val: "yes", set: true, def: false, want: false, wantErr: true},
	}
	for _, c := range cases {
		m := map[string]string{}
		if c.set {
			m["FLAG"] = c.val
		}
		e := FromMap(m)
		if got := e.Bool("FLAG", c.def); got != c.want {
			t.Errorf("Bool(%q, def=%v) = %v, want %v", c.val, c.def, got, c.want)
		}
		if (e.Err() != nil) != c.wantErr {
			t.Errorf("Bool(%q): err = %v, wantErr %v", c.val, e.Err(), c.wantErr)
		}
	}
}
