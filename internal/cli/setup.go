package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/setup"
	"github.com/balpal4495/loar/internal/store/postgres"
	"github.com/spf13/cobra"
)

// NewSetupCmd returns the `loar setup` command.
func NewSetupCmd() *cobra.Command {
	var reset bool
	var local bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure Loar for first use",
		Long: `Detects a local PostgreSQL instance, creates the Loar database user,
and writes the global configuration to ~/.config/loar/config.toml.

Use --local to skip Postgres entirely and use a local SQLite database instead.
Local mode requires no Docker and no Postgres — the database lives in .loar/
inside each project directory.

Run once per machine before using any other loar command.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := cmd.OutOrStdout()

			// Check whether setup has already been run.
			exists, err := config.GlobalConfigExists()
			if err != nil {
				return err
			}
			if exists && !reset {
				path, _ := config.GlobalConfigPath()
				fmt.Fprintf(w, "Global config already exists at %s\n", path)
				fmt.Fprintln(w, "Run `loar setup --reset` to reconfigure.")
				return nil
			}

			fmt.Fprintln(w, "Loar Setup")
			fmt.Fprintln(w, "──────────────────────────────────────────────────────")

			if local {
				return setupLocal(cmd, w)
			}

			// Auto-suggest local mode when Postgres is unavailable.
			fmt.Fprint(w, "Checking for PostgreSQL on localhost:5432... ")
			if setup.DetectPostgres() != setup.PostgresRunning {
				fmt.Fprintln(w, "✗ not found")
				fmt.Fprintln(w)
				fmt.Fprintln(w, "PostgreSQL is not running.")
				fmt.Fprintln(w, "Use `loar setup --local` for a zero-dependency SQLite backend.")
				fmt.Fprintln(w, "Or install and start PostgreSQL, then re-run `loar setup`.")
				fmt.Fprintln(w, setup.InstallInstructions())
				return fmt.Errorf("setup: PostgreSQL not available on localhost:5432")
			}
			fmt.Fprintln(w, "✓")

			defaults := config.DefaultGlobalConfig()
			scanner := bufio.NewReader(cmd.InOrStdin())

			fmt.Fprintf(w, "\nPostgres admin username [postgres]: ")
			adminUser := readLine(scanner)
			if adminUser == "" {
				adminUser = "postgres"
			}

			fmt.Fprintf(w, "Postgres admin password [postgres]: ")
			adminPassword := readLine(scanner)
			if adminPassword == "" {
				adminPassword = "postgres"
			}

			adminDSN := fmt.Sprintf(
				"postgres://%s:%s@%s:%d/postgres?sslmode=disable",
				adminUser, adminPassword, defaults.PostgresHost, defaults.PostgresPort,
			)

			fmt.Fprintf(w, "Loar database user     [%s]: ", defaults.PostgresUser)
			loarUser := readLine(scanner)
			if loarUser == "" {
				loarUser = defaults.PostgresUser
			}

			fmt.Fprintf(w, "Loar database password [%s]: ", defaults.PostgresPassword)
			loarPassword := readLine(scanner)
			if loarPassword == "" {
				loarPassword = defaults.PostgresPassword
			}

			fmt.Fprintf(w, "\nCreating Postgres user %q... ", loarUser)
			ctx := cmd.Context()
			if err := postgres.EnsureUser(ctx, adminDSN, loarUser, loarPassword); err != nil {
				fmt.Fprintln(w, "✗")
				return fmt.Errorf("setup: %w", err)
			}
			fmt.Fprintln(w, "✓")

			cfg := config.GlobalConfig{
				PostgresHost:     defaults.PostgresHost,
				PostgresPort:     defaults.PostgresPort,
				PostgresUser:     loarUser,
				PostgresPassword: loarPassword,
				Backend:          "postgres",
			}
			path, _ := config.GlobalConfigPath()
			fmt.Fprintf(w, "Writing %s... ", path)
			if err := config.WriteGlobal(cfg); err != nil {
				fmt.Fprintln(w, "✗")
				return fmt.Errorf("setup: %w", err)
			}
			fmt.Fprintln(w, "✓")

			fmt.Fprintln(w)
			fmt.Fprintln(w, "Setup complete.")
			fmt.Fprintln(w, "Run `loar project use` in any directory to start a new project.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false, "Overwrite existing global config")
	cmd.Flags().BoolVar(&local, "local", false, "Use local SQLite backend (no Postgres required)")
	return cmd
}

// setupLocal writes a local-mode global config without requiring Postgres.
func setupLocal(cmd *cobra.Command, w interface{ Write([]byte) (int, error) }) error {
	fmt.Fprintln(w, "Mode: local (SQLite — no Postgres required)")
	cfg := config.GlobalConfig{
		Backend: "local",
	}
	path, _ := config.GlobalConfigPath()
	fmt.Fprintf(w, "Writing %s... ", path)
	if err := config.WriteGlobal(cfg); err != nil {
		fmt.Fprintln(w, "✗")
		return fmt.Errorf("setup: %w", err)
	}
	fmt.Fprintln(w, "✓")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Setup complete (local mode).")
	fmt.Fprintln(w, "Run `loar project use` in any directory to start a new project.")
	fmt.Fprintln(w, "The SQLite database will be created at .loar/loar.db inside each project.")
	return nil
}

// readLine reads a single line from r, trimming whitespace.
func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}


