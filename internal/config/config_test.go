package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyleking/jojo/internal/config"
)

const fileMode = 0o600

// writeConfig points the user config directory at a temp dir holding body, or holding nothing when
// body is empty. It sets environment variables, which is why none of these tests run in parallel.
func writeConfig(t *testing.T, body string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	if body == "" {
		return
	}

	dir := filepath.Join(home, "jojo")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), fileMode); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// Running with no config file is the common case and has to stay silent.
//
//nolint:paralleltest // t.Setenv forbids it.
func TestLoadWithoutAFileIsNotAnError(t *testing.T) {
	writeConfig(t, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Theme != "" || cfg.Keys != nil || cfg.Revsets != nil {
		t.Errorf("Load() = %#v, want the zero config", cfg)
	}
}

//nolint:paralleltest // t.Setenv forbids it.
func TestLoadReadsEverySetting(t *testing.T) {
	writeConfig(t, "theme = \"latte\"\n\n[keys]\ndescribe = \"M\"\n\n[revsets]\nmine = \"mine()\"\n")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Theme != "latte" {
		t.Errorf("Theme = %q, want latte", cfg.Theme)
	}

	if cfg.Keys["describe"] != "M" {
		t.Errorf("Keys = %v, want describe on M", cfg.Keys)
	}

	if cfg.Revsets["mine"] != "mine()" {
		t.Errorf("Revsets = %v, want mine", cfg.Revsets)
	}
}

// A file that does not parse is the one place a typo hides, so it stops startup rather than leaving
// every setting silently unapplied.
//
//nolint:paralleltest // t.Setenv forbids it.
func TestLoadRefusesAFileItCannotParse(t *testing.T) {
	writeConfig(t, "theme = [unclosed\n")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() on a broken file = nil, want an error naming the file")
	}
}
