// Package config reads jojo's own settings file. Everything in it is optional: jojo runs with no
// file at all, and the repository's jj config still decides everything about jj itself.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is what config.toml carries. Theme names a Catppuccin flavor ("latte" or "light",
// "macchiato" or "dark") and reads the terminal's background when it is empty.
type Config struct {
	// Keys rebinds an action, naming it the way the help legend does: keys.describe = "D". An action
	// the keymap does not know is an error rather than a line quietly doing nothing.
	Keys map[string]string `toml:"keys"`
	// Revsets are the expressions worth keeping, shown in the command palette by name.
	Revsets map[string]string `toml:"revsets"`
	Theme   string            `toml:"theme"`
}

// Path is where jojo looks: $XDG_CONFIG_HOME/jojo/config.toml, or ~/.config/jojo/config.toml. It
// deliberately does not use os.UserConfigDir, which on macOS points at Library/Application Support
// and would put jojo somewhere none of the terminal tools beside it live.
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating the home directory: %w", err)
		}

		dir = filepath.Join(home, ".config")
	}

	return filepath.Join(dir, "jojo", "config.toml"), nil
}

// Load reads the config file. A missing file is the common case and returns the zero Config, while
// a file that exists and does not parse is an error, since silently ignoring it would hide a typo
// in the only place a user states a preference.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	//nolint:gosec // G304: the path is built from the user's own config directory.
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}

	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	return cfg, nil
}
