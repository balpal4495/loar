// Package cli assembles the loar command tree and shared configuration.
package cli

import (
	"fmt"
	"os"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/store"
	"github.com/balpal4495/loar/internal/store/postgres"
	"github.com/spf13/cobra"
)

// openStore opens a Postgres store for the project associated with the
// current working directory. Returns the store, the project config, and
// an error. The caller is responsible for calling store.Close() when done.
func openStore(cmd *cobra.Command) (store.Store, *config.ProjectConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("could not determine working directory: %w", err)
	}

	cfg, _, err := config.Find(cwd)
	if err != nil {
		return nil, nil, fmt.Errorf("no project configured; run \"loar project use\" to initialise one")
	}

	dsn := cfg.DatabaseURL
	if envDSN := os.Getenv("LOAR_DATABASE_URL"); envDSN != "" {
		dsn = envDSN
	}
	if dsn == "" {
		return nil, nil, fmt.Errorf("project.toml has no database_url; run \"loar project use\" to reinitialise")
	}
	db, err := postgres.New(cmd.Context(), dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: connect: %w", err)
	}
	return db, cfg, nil
}

// mustProjectDSN returns the Postgres DSN for the current project.
// Kept for commands (project clean, etc.) that still open Postgres directly.
// Prefer openStore for new code.
func mustProjectDSN(cmd *cobra.Command) string {
	if dsn := os.Getenv("LOAR_DATABASE_URL"); dsn != "" {
		return dsn
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: could not determine working directory:", err)
		os.Exit(1)
	}

	cfg, _, err := config.Find(cwd)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: no project configured; run \"loar project use\" to initialise one")
		os.Exit(1)
	}

	if cfg.DatabaseURL == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: project.toml has no database_url; run \"loar project use\" to reinitialise")
		os.Exit(1)
	}

	return cfg.DatabaseURL
}

// mustGlobalConfig loads ~/.config/loar/config.toml and returns it.
// Exits with a helpful error if the file is missing.
func mustGlobalConfig(cmd *cobra.Command) (*config.GlobalConfig, error) {
	gcfg, err := config.LoadGlobal()
	if err != nil {
		return nil, fmt.Errorf("%w\nRun \"loar setup\" first", err)
	}
	return gcfg, nil
}
