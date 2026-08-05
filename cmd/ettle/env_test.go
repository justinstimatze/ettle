package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUserEnvFillsOnlyWhatIsMissing(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if err := os.MkdirAll(filepath.Join(cfg, "ettle"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# keys\n\nLINEAR_API_KEY=lin_from_file\nexport ANTHROPIC_API_KEY=\"sk-from-file\"\nMALFORMED\n"
	if err := os.WriteFile(filepath.Join(cfg, "ettle", "env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// An explicit environment variable must WIN — the file supplies what's missing,
	// it never overrides what a caller set.
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-caller")
	t.Setenv("LINEAR_API_KEY", "")

	loadUserEnv()

	if got := os.Getenv("LINEAR_API_KEY"); got != "lin_from_file" {
		t.Errorf("missing var should come from the file, got %q", got)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "sk-from-caller" {
		t.Errorf("an explicit env var must win, got %q", got)
	}
}

func TestLoadUserEnvIsSilentWithNoFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LINEAR_API_KEY", "")
	loadUserEnv() // must not panic or error — it is a convenience, not a requirement
	if got := os.Getenv("LINEAR_API_KEY"); got != "" {
		t.Errorf("no file should mean no change, got %q", got)
	}
}
