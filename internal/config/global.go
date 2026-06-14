package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const globalFileName = "config.toml"

// GlobalConfig holds machine-level connection details written by `loar setup`.
// It is stored at ~/.config/loar/config.toml.
type GlobalConfig struct {
	PostgresHost     string `toml:"postgres_host"`
	PostgresPort     int    `toml:"postgres_port"`
	PostgresUser     string `toml:"postgres_user"`
	PostgresPassword string `toml:"postgres_password"`
	// Backend is "postgres" (default) or "local" (SQLite, set by loar setup --local).
	Backend string `toml:"backend,omitempty"`
}

// DefaultGlobalConfig returns a GlobalConfig with sane local defaults.
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		PostgresHost:     "localhost",
		PostgresPort:     5432,
		PostgresUser:     "loar",
		PostgresPassword: "loar",
	}
}

// AdminDSN returns a DSN connecting to the "postgres" maintenance database
// using the global config credentials. Used for creating/dropping project
// databases and managing users.
func (g GlobalConfig) AdminDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/postgres?sslmode=disable",
		g.PostgresUser, g.PostgresPassword, g.PostgresHost, g.PostgresPort,
	)
}

// ProjectDSN returns a DSN for the named project database (loar-<name>).
func (g GlobalConfig) ProjectDSN(projectName string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/loar-%s?sslmode=disable",
		g.PostgresUser, g.PostgresPassword, g.PostgresHost, g.PostgresPort, projectName,
	)
}

// GlobalConfigDir returns the directory for the global config (~/.config/loar).
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "loar"), nil
}

// GlobalConfigPath returns the absolute path to ~/.config/loar/config.toml.
func GlobalConfigPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalFileName), nil
}

// GlobalConfigExists reports whether ~/.config/loar/config.toml already exists.
func GlobalConfigExists() (bool, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// LoadGlobal reads the global config from ~/.config/loar/config.toml.
func LoadGlobal() (*GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}
	var cfg GlobalConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("config: global config not found at %s; run \"loar setup\" first", path)
	}
	return &cfg, nil
}

// WriteGlobal writes cfg to ~/.config/loar/config.toml with mode 0600.
func WriteGlobal(cfg GlobalConfig) error {
	dir, err := GlobalConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: create config dir: %w", err)
	}
	path := filepath.Join(dir, globalFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("config: write global config: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
