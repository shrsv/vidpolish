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
}

// Config is the top-level vidpolish configuration.
type Config struct {
	YouTube YouTube `toml:"youtube"`
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
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
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
