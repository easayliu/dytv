package config

import (
	"os"
	"path/filepath"
	"strings"
)

// cookieDir returns the directory for storing cookie files.
func cookieDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dytv")
}

// LoadCookie reads the cookie from ~/.config/dytv/cookie.
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

// SaveCookie writes the cookie to ~/.config/dytv/cookie.
func SaveCookie(cookie string) error {
	dir := cookieDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cookie"), []byte(cookie), 0600)
}
