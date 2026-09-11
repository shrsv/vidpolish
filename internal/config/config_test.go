package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestInitCreatesConfigOnce(t *testing.T) {
	home := withTempHome(t)
	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := filepath.Join(home, ".vidpolish", "config.toml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading created config: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("config file is empty")
	}

	// A second Init must not clobber an edited config.
	if err := os.WriteFile(path, []byte("[youtube]\nclient_id = \"abc\"\n"), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	if _, err := Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config after second Init: %v", err)
	}
	if string(data) != "[youtube]\nclient_id = \"abc\"\n" {
		t.Fatalf("Init overwrote existing config: %q", data)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	withTempHome(t)
	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.WriteFile(path, []byte("[youtube]\nclient_id = \"abc\"\n"), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.YouTube.ClientID != "abc" {
		t.Fatalf("ClientID = %q, want abc", cfg.YouTube.ClientID)
	}
	if cfg.YouTube.Privacy != "unlisted" {
		t.Fatalf("Privacy default = %q, want unlisted", cfg.YouTube.Privacy)
	}
	if cfg.YouTube.DefaultLanguage != "en" {
		t.Fatalf("DefaultLanguage default = %q, want en", cfg.YouTube.DefaultLanguage)
	}
}

func TestLoadWithoutConfigErrors(t *testing.T) {
	withTempHome(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error loading config before Init")
	}
}

func TestSaveRoundTrips(t *testing.T) {
	withTempHome(t)
	if _, err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.YouTube.RefreshToken = "refresh-xyz"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if reloaded.YouTube.RefreshToken != "refresh-xyz" {
		t.Fatalf("RefreshToken = %q, want refresh-xyz", reloaded.YouTube.RefreshToken)
	}
}
