package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/balpal4495/loar/internal/config"
)

func TestWriteAndLoad(t *testing.T) {
	dir := t.TempDir()

	if err := config.Write(dir, &config.ProjectConfig{Project: "tierone"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The file must exist at .loar/project.toml.
	path := filepath.Join(dir, ".loar", "project.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("project.toml not created: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project != "tierone" {
		t.Errorf("Project: want tierone, got %s", cfg.Project)
	}
}

func TestWriteEmptyName(t *testing.T) {
	dir := t.TempDir()
	if err := config.Write(dir, &config.ProjectConfig{Project: ""}); err == nil {
		t.Error("Write with empty name should return an error")
	}
}

func TestWriteEmptyDir(t *testing.T) {
	if err := config.Write("", &config.ProjectConfig{Project: "myproject"}); err == nil {
		t.Error("Write with empty dir should return an error")
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write config in root.
	if err := config.Write(root, &config.ProjectConfig{Project: "myproject"}); err != nil {
		t.Fatal(err)
	}

	// Find from a deeply nested subdirectory.
	cfg, foundDir, err := config.Find(subdir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cfg.Project != "myproject" {
		t.Errorf("Project: want myproject, got %s", cfg.Project)
	}
	if foundDir != root {
		t.Errorf("foundDir: want %s, got %s", root, foundDir)
	}
}

func TestFindNotFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := config.Find(dir)
	if err == nil {
		t.Error("Find should return an error when no config exists")
	}
}

func TestOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := config.Write(dir, &config.ProjectConfig{Project: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := config.Write(dir, &config.ProjectConfig{Project: "second"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "second" {
		t.Errorf("Project: want second, got %s", cfg.Project)
	}
}
