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
	if cfg.YouTube.DescriptionTemplate != "{{.Title}}\n\nvidpolish" {
		t.Fatalf("DescriptionTemplate default = %q", cfg.YouTube.DescriptionTemplate)
	}
	if cfg.Thumbnail.BackgroundColor != "#0f172a" {
		t.Fatalf("BackgroundColor default = %q", cfg.Thumbnail.BackgroundColor)
	}
	if cfg.Thumbnail.AccentColor != "#22d3ee" {
		t.Fatalf("AccentColor default = %q", cfg.Thumbnail.AccentColor)
	}
	if cfg.Thumbnail.TextColor != "#ffffff" {
		t.Fatalf("TextColor default = %q", cfg.Thumbnail.TextColor)
	}
	if !cfg.Thumbnail.Enabled {
		t.Fatal("Thumbnail.Enabled should default to true when [thumbnail] is absent from the config entirely (e.g. an older config predating this feature)")
	}
}

func TestLoadRespectsExplicitThumbnailDisabled(t *testing.T) {
	withTempHome(t)
	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	content := "[thumbnail]\nenabled = false\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Thumbnail.Enabled {
		t.Fatal("Thumbnail.Enabled = true, want false (explicitly disabled in config)")
	}
}

func TestLoadPreservesConfiguredTagsAndThumbnail(t *testing.T) {
	withTempHome(t)
	path, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	content := `[youtube]
default_tags = ["golang", "screencast"]

[thumbnail]
enabled = true
logo_path = "/tmp/logo.png"
background_color = "#111111"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.YouTube.DefaultTags) != 2 || cfg.YouTube.DefaultTags[0] != "golang" {
		t.Fatalf("DefaultTags = %v", cfg.YouTube.DefaultTags)
	}
	if !cfg.Thumbnail.Enabled {
		t.Fatal("Thumbnail.Enabled = false, want true")
	}
	if cfg.Thumbnail.LogoPath != "/tmp/logo.png" {
		t.Fatalf("LogoPath = %q", cfg.Thumbnail.LogoPath)
	}
	if cfg.Thumbnail.BackgroundColor != "#111111" {
		t.Fatalf("BackgroundColor = %q, want #111111 (explicit value should not be overridden)", cfg.Thumbnail.BackgroundColor)
	}
	// AccentColor/TextColor weren't set, so defaults should still apply.
	if cfg.Thumbnail.AccentColor != "#22d3ee" {
		t.Fatalf("AccentColor default = %q", cfg.Thumbnail.AccentColor)
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

func TestValidateRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"bad privacy", Config{YouTube: YouTube{Privacy: "sorta-public"}}},
		{"bad background color", Config{Thumbnail: Thumbnail{BackgroundColor: "navy"}}},
		{"bad accent color", Config{Thumbnail: Thumbnail{AccentColor: "#ggg"}}},
		{"broken template", Config{YouTube: YouTube{DescriptionTemplate: "{{.Title"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(&c.cfg); err == nil {
				t.Fatalf("expected Validate to reject %+v", c.cfg)
			}
		})
	}
}

func TestValidateAcceptsGoodValues(t *testing.T) {
	cfg := Config{
		YouTube:   YouTube{Privacy: "public", DescriptionTemplate: "{{.Title}}"},
		Thumbnail: Thumbnail{BackgroundColor: "#0f172a", AccentColor: "#fff", TextColor: ""},
	}
	if err := Validate(&cfg); err != nil {
		t.Fatalf("Validate rejected a valid config: %v", err)
	}
}

func TestSaveRejectsInvalidConfigWithoutTouchingDisk(t *testing.T) {
	withTempHome(t)
	if _, err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	before, err := os.ReadFile(mustPath(t))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	bad := &Config{YouTube: YouTube{Privacy: "nonsense"}}
	if err := Save(bad); err == nil {
		t.Fatal("expected Save to reject an invalid config")
	}

	after, err := os.ReadFile(mustPath(t))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("Save must not modify the on-disk config when validation fails")
	}
}

func TestSaveIsAtomicNoPartialTempFileLeftBehind(t *testing.T) {
	withTempHome(t)
	if _, err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dir := filepath.Dir(mustPath(t))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("leftover temp file after Save: %s", e.Name())
		}
	}
}

func TestSaveCreatesAndPrunesBackups(t *testing.T) {
	withTempHome(t)
	if _, err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Save more than maxBackups times; each Save backs up whatever was
	// on disk *before* that save, so this produces maxBackups+1 saves
	// worth of prior state to back up.
	for i := 0; i < maxBackups+3; i++ {
		cfg.YouTube.ClientID = "client-" + string(rune('a'+i))
		if err := Save(cfg); err != nil {
			t.Fatalf("Save #%d: %v", i, err)
		}
	}

	backups, err := ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != maxBackups {
		t.Fatalf("len(backups) = %d, want %d (old backups should be pruned)", len(backups), maxBackups)
	}
	// Most recent first.
	for i := 1; i < len(backups); i++ {
		if backups[i-1].Timestamp < backups[i].Timestamp {
			t.Fatalf("backups not sorted most-recent-first: %+v", backups)
		}
	}
}

func TestRestoreBackupRoundTrips(t *testing.T) {
	withTempHome(t)
	if _, err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.YouTube.ClientID = "original-client-id"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	backups, err := ListBackups()
	if err != nil || len(backups) == 0 {
		t.Fatalf("ListBackups: %v, %v", backups, err)
	}
	// The most recent backup holds the config.toml template state (from
	// Init), since that's what was on disk right before the Save above.
	target := backups[0]

	cfg.YouTube.ClientID = "changed-after-backup"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, _ := Load()
	if reloaded.YouTube.ClientID != "changed-after-backup" {
		t.Fatalf("sanity check failed: %q", reloaded.YouTube.ClientID)
	}

	if err := RestoreBackup(target.Timestamp); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	restored, err := Load()
	if err != nil {
		t.Fatalf("Load after restore: %v", err)
	}
	if restored.YouTube.ClientID != "" {
		t.Fatalf("restored ClientID = %q, want empty (the pre-Save template state)", restored.YouTube.ClientID)
	}
}

func mustPath(t *testing.T) string {
	t.Helper()
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	return p
}
