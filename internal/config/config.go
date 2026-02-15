package config

import (
	"os"
	"path/filepath"
	"strings"
)

// cookieDir returns the directory for storing cookie files.
// Priority: DYTV_CONFIG_DIR > XDG_CONFIG_HOME/dytv > ~/.config/dytv
func cookieDir() string {
	if dir := os.Getenv("DYTV_CONFIG_DIR"); dir != "" {
		return dir
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "dytv")
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dytv")
}

// LoadCookie reads the cookie from the config directory.
// Returns empty string (not error) if the file does not exist.
func LoadCookie() (string, error) {
	data, err := os.ReadFile(filepath.Join(cookieDir(), "cookie"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveCookie writes the cookie to the config directory.
func SaveCookie(cookie string) error {
	dir := cookieDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cookie"), []byte(cookie), 0600)
}
