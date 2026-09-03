package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load defaults: %v", err)
	}
	if cfg.APIPort != 8080 || cfg.EnginePort != 8081 {
		t.Fatalf("unexpected ports: %+v", cfg)
	}
}

func TestLoadFailsFastOnInvalid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tide.yaml")
	if err := os.WriteFile(p, []byte("apiPort: 99999\nenv: bogus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tide.yaml")
	if err := os.WriteFile(p, []byte("apiPort: 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TIDE_API_PORT", "9091")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIPort != 9091 {
		t.Fatalf("env override failed: got %d", cfg.APIPort)
	}
}

func TestMissingFileFailsFast(t *testing.T) {
	if _, err := Load("/nonexistent/tide.yaml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
