// Package config reads and writes vidpolish's YouTube upload configuration
// at ~/.vidpolish/config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

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

const template = `[youtube]
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
	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
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

// Save writes cfg back to ~/.vidpolish/config.toml, e.g. after storing a
// refresh token from "vidpolish youtube login".
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
