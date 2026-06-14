// Package cli assembles the loar command tree and shared configuration.
package cli

import (
	"fmt"
	"os"

	"github.com/balpal4495/loar/internal/config"
	"github.com/spf13/cobra"
)

// mustProjectDSN returns the Postgres DSN for the current project.
// Precedence:
//  1. LOAR_DATABASE_URL environment variable (CI / advanced override)
//  2. database_url field in .loar/project.toml (set by `loar project use`)
//
// Exits with a helpful error if neither is available.
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
