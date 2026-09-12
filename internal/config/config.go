// Package config reads and writes vidpolish's YouTube upload configuration
// at ~/.vidpolish/config.toml.
package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
)

// YouTube holds YouTube API credentials and upload defaults.
type YouTube struct {
	ClientID        string `toml:"client_id"`
	ClientSecret    string `toml:"client_secret"`
	RefreshToken    string `toml:"refresh_token"`
	Privacy         string `toml:"privacy"`
	DefaultLanguage string `toml:"default_language"`

	// DefaultTags are merged with the fixed "vidpolish" tag on every
	// upload.
	DefaultTags []string `toml:"default_tags"`

	// DescriptionTemplate is rendered with Go's text/template, given
	// {{.Title}}. If the rendered result doesn't contain the literal
	// word "vidpolish", it is appended automatically.
	DescriptionTemplate string `toml:"description_template"`
}

// Thumbnail configures auto-generated YouTube thumbnails.
type Thumbnail struct {
	Enabled         bool   `toml:"enabled"`
	LogoPath        string `toml:"logo_path"`
	BackgroundColor string `toml:"background_color"`
	AccentColor     string `toml:"accent_color"`
	TextColor       string `toml:"text_color"`
}

// Config is the top-level vidpolish configuration.
type Config struct {
	YouTube   YouTube   `toml:"youtube"`
	Thumbnail Thumbnail `toml:"thumbnail"`
}

const configTemplate = `[youtube]
# From a Google Cloud Console OAuth client of type "Desktop app" with the
# YouTube Data API v3 enabled. See the README's YouTube upload section.
client_id     = ""
client_secret = ""

# Filled in automatically by "vidpolish youtube login". Leave blank.
refresh_token = ""

# Default privacy for uploads: public | unlisted | private
privacy = "unlisted"

# Fallback for the uploaded video's snippet.defaultLanguage and
# snippet.defaultAudioLanguage (BCP-47 code, e.g. "en", "en-IN").
default_language = "en"

# Tags merged with the fixed "vidpolish" tag on every upload.
default_tags = []

# Rendered with Go's text/template; {{.Title}} is available. The word
# "vidpolish" is always appended if missing from the rendered result.
description_template = "{{.Title}}\n\nvidpolish"

[thumbnail]
# Auto-generate a thumbnail (brand background + optional logo + title)
# and set it on every upload.
enabled = true

# Local PNG/JPG/SVG logo file to composite onto the thumbnail. Empty = no
# logo, title text only.
logo_path = ""

background_color = "#0f172a"
accent_color     = "#22d3ee"
text_color       = "#ffffff"
`

// Path returns the path to ~/.vidpolish/config.toml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".vidpolish", "config.toml"), nil
}

// Init creates ~/.vidpolish/config.toml with a commented template if it
// does not already exist. It never overwrites an existing config file.
func Init() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking config path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(configTemplate), 0o600); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}
	return path, nil
}

// Load reads ~/.vidpolish/config.toml, applying defaults for unset
// optional fields. It returns an error if the file does not exist; run
// "vidpolish config init" first.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	var cfg Config
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config at %s, run \"vidpolish config init\" first", path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if cfg.YouTube.Privacy == "" {
		cfg.YouTube.Privacy = "unlisted"
	}
	if cfg.YouTube.DefaultLanguage == "" {
		cfg.YouTube.DefaultLanguage = "en"
	}
	if cfg.YouTube.DescriptionTemplate == "" {
		cfg.YouTube.DescriptionTemplate = "{{.Title}}\n\nvidpolish"
	}
	// Thumbnail generation defaults to on. A plain bool zero-value can't
	// tell "the config never mentioned [thumbnail].enabled" apart from
	// "explicitly set to false", so check the decode metadata: only a
	// config that actually spells out `enabled = false` turns it off.
	if !meta.IsDefined("thumbnail", "enabled") {
		cfg.Thumbnail.Enabled = true
	}
	if cfg.Thumbnail.BackgroundColor == "" {
		cfg.Thumbnail.BackgroundColor = "#0f172a"
	}
	if cfg.Thumbnail.AccentColor == "" {
		cfg.Thumbnail.AccentColor = "#22d3ee"
	}
	if cfg.Thumbnail.TextColor == "" {
		cfg.Thumbnail.TextColor = "#ffffff"
	}
	return &cfg, nil
}

// maxBackups is how many timestamped backups BackupDir retains; older
// ones are pruned on each Save.
const maxBackups = 10

var (
	hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	privacies  = map[string]bool{"public": true, "unlisted": true, "private": true}
)

// Validate rejects a Config that would be unsafe or broken to save:
// an unrecognized privacy value, a thumbnail color that isn't a valid
// #rgb/#rrggbb hex code, or a description_template that fails to parse.
// The CLI and the local UI's config API both call this before Save so
// neither path can persist a config that quietly breaks uploads.
func Validate(cfg *Config) error {
	if cfg.YouTube.Privacy != "" && !privacies[cfg.YouTube.Privacy] {
		return fmt.Errorf("youtube.privacy must be public, unlisted, or private, got %q", cfg.YouTube.Privacy)
	}
	for name, v := range map[string]string{
		"background_color": cfg.Thumbnail.BackgroundColor,
		"accent_color":     cfg.Thumbnail.AccentColor,
		"text_color":       cfg.Thumbnail.TextColor,
	} {
		if v != "" && !hexColorRe.MatchString(v) {
			return fmt.Errorf("thumbnail.%s must be a #rgb or #rrggbb hex color, got %q", name, v)
		}
	}
	if cfg.YouTube.DescriptionTemplate != "" {
		if _, err := template.New("description").Parse(cfg.YouTube.DescriptionTemplate); err != nil {
			return fmt.Errorf("youtube.description_template is not a valid template: %w", err)
		}
	}
	return nil
}

// BackupDir returns ~/.vidpolish/config-backups, creating it if needed.
func BackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	dir := filepath.Join(home, ".vidpolish", "config-backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating config backup dir: %w", err)
	}
	return dir, nil
}

// Backup is one saved snapshot of a prior config.toml.
type Backup struct {
	Timestamp int64 // unix nanoseconds; a unique, sortable id used by ListBackups/RestoreBackup
	Path      string
}

// ListBackups returns saved config backups, most recent first.
func ListBackups() ([]Backup, error) {
	dir, err := BackupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing config backups: %w", err)
	}
	var out []Backup
	for _, e := range entries {
		ts, ok := parseBackupFilename(e.Name())
		if !ok {
			continue
		}
		out = append(out, Backup{Timestamp: ts, Path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

// RestoreBackup writes the backup taken at timestamp back to
// ~/.vidpolish/config.toml (itself going through Save's atomic-write and
// fresh-backup path, so a restore can always be undone too).
func RestoreBackup(timestamp int64) error {
	dir, err := BackupDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, backupFilename(timestamp)))
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return fmt.Errorf("parsing backup: %w", err)
	}
	return Save(&cfg)
}

func backupFilename(ts int64) string {
	return "config-" + strconv.FormatInt(ts, 10) + ".toml"
}

func parseBackupFilename(name string) (int64, bool) {
	if !strings.HasPrefix(name, "config-") || !strings.HasSuffix(name, ".toml") {
		return 0, false
	}
	ts, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(name, "config-"), ".toml"), 10, 64)
	if err != nil {
		return 0, false
	}
	return ts, true
}

// Save validates cfg, then writes it back to ~/.vidpolish/config.toml
// atomically (write to a temp file, then rename over the real path, so a
// crash mid-write can never leave a half-written config), backing up
// whatever was there before under ~/.vidpolish/config-backups first.
func Save(cfg *Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	if err := backupCurrent(path); err != nil {
		return fmt.Errorf("backing up config before save: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed away

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// backupCurrent copies whatever is currently on disk at path into
// ~/.vidpolish/config-backups before it gets overwritten, then prunes
// old backups beyond maxBackups. A missing current file is not an error
// (nothing to back up yet).
func backupCurrent(path string) error {
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()

	dir, err := BackupDir()
	if err != nil {
		return err
	}
	dest, err := os.OpenFile(filepath.Join(dir, backupFilename(time.Now().UnixNano())), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer dest.Close()
	if _, err := io.Copy(dest, src); err != nil {
		return err
	}

	backups, err := ListBackups()
	if err != nil {
		return err
	}
	for _, b := range backups[min(len(backups), maxBackups):] {
		os.Remove(b.Path)
	}
	return nil
}
