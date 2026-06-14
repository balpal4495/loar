// Package config manages the per-directory Loar configuration stored in
// .loar/project.toml. This file associates the current directory with a
// named project.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	dirName  = ".loar"
	fileName = "project.toml"
)

// ProjectConfig holds the per-directory project association.
type ProjectConfig struct {
	Project     string `toml:"project"`
	DatabaseURL string `toml:"database_url,omitempty"`
}

// configPath returns the absolute path to the project.toml file starting
// from dir and walking up to the filesystem root. It returns the path to use
// when writing a new config at dir (i.e. dir/.loar/project.toml) and
// separately reports whether an existing config was found.
func configPath(dir string) string {
	return filepath.Join(dir, dirName, fileName)
}

// Write writes a ProjectConfig to .loar/project.toml in dir.
func Write(dir string, cfg *ProjectConfig) error {
	if dir == "" {
		return errors.New("config: directory must not be empty")
	}
	if cfg.Project == "" {
		return errors.New("config: project name must not be empty")
	}

	loarDir := filepath.Join(dir, dirName)
	if err := os.MkdirAll(loarDir, 0o755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(loarDir, fileName))
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

// Load reads the project.toml from dir. It returns an error if the file does
// not exist or cannot be parsed.
func Load(dir string) (*ProjectConfig, error) {
	path := configPath(dir)
	var cfg ProjectConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Find walks up from dir towards the filesystem root looking for a
// .loar/project.toml file. It returns the config and the directory where it
// was found, or an error if none was found.
func Find(dir string) (*ProjectConfig, string, error) {
	current := dir
	for {
		cfg, err := Load(current)
		if err == nil {
			return cfg, current, nil
		}
		if !os.IsNotExist(err) && !isNotExistTomlErr(err) {
			return nil, "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil, "", errors.New("config: no .loar/project.toml found; run \"loar project use <name>\" to associate a project")
}

// isNotExistTomlErr reports whether err is a file-not-found error that
// originated during TOML decoding (the underlying os.PathError from
// os.Open is wrapped by the toml package).
func isNotExistTomlErr(err error) bool {
	var pathErr *os.PathError
	return errors.As(err, &pathErr) && os.IsNotExist(pathErr)
}
