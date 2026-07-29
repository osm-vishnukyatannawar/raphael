// Package config resolves the on-disk locations Raphael uses for user data.
//
// Nothing is written next to the binary: on Linux this resolves under
// ~/.config/raphael, on Windows under %AppData%\raphael.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDirName is the per-user directory Raphael owns inside the OS config dir.
const appDirName = "raphael"

// Dir returns the application's config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create config dir %q: %w", dir, err)
	}

	return dir, nil
}

// DatabasePath returns the path to the SQLite database file.
func DatabasePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "raphael.db"), nil
}
