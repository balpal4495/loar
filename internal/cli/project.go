package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/store/postgres"
	sqstore "github.com/balpal4495/loar/internal/store/sqlite"
	"github.com/spf13/cobra"
)

// NewProjectCmd returns the `loar project` command group.
func NewProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
		Long:  "Projects are knowledge boundaries. Each directory can be associated with one project.",
	}
	cmd.AddCommand(newProjectUseCmd())
	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectDeleteCmd())
	cmd.AddCommand(newProjectCleanCmd())
	return cmd
}

// newProjectUseCmd returns `loar project use [name]`.
func newProjectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [name]",
		Short: "Associate the current directory with a project",
		Long: `Associates the current directory with a named project.

If no name is given, the current directory name is used.

In Postgres mode: creates a dedicated loar-<name> database.
In local mode (after loar setup --local): creates .loar/loar.db (SQLite).

Requires ~/.config/loar/config.toml — run ` + "`loar setup`" + ` first.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("project use: %w", err)
			}

			name := filepath.Base(cwd)
			if len(args) == 1 {
				name = args[0]
			}

			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			// Local (SQLite) mode — no Postgres needed.
			if gcfg.Backend == "local" {
				loarDir := filepath.Join(cwd, ".loar")
				if err := os.MkdirAll(loarDir, 0o755); err != nil {
					return fmt.Errorf("project use: create .loar dir: %w", err)
				}
				dbPath := filepath.Join(loarDir, "loar.db")
				fmt.Fprintf(cmd.OutOrStdout(), "Initialising local database at %s... ", dbPath)
				db, err := sqstore.New(dbPath)
				if err != nil {
					fmt.Fprintln(cmd.OutOrStdout(), "✗")
					return fmt.Errorf("project use: %w", err)
				}
				defer db.Close()
				if err := db.Migrate(ctx); err != nil {
					fmt.Fprintln(cmd.OutOrStdout(), "✗")
					return fmt.Errorf("project use: migrate: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓")

				proj, err := db.GetProjectByName(ctx, name)
				if err != nil {
					proj = &domain.Project{Name: name}
					if err := db.CreateProject(ctx, proj); err != nil {
						return fmt.Errorf("project use: create project record: %w", err)
					}
				}
				_ = proj

				if err := config.Write(cwd, &config.ProjectConfig{
					Project:     name,
					Backend:     "local",
					DatabaseURL: dbPath,
				}); err != nil {
					return fmt.Errorf("project use: write config: %w", err)
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Project set to %q (local mode)\n", name)
				fmt.Fprintln(cmd.OutOrStdout(), "Created .loar/project.toml")
				return nil
			}

			// Postgres mode.
			dbName := "loar-" + name
			fmt.Fprintf(cmd.OutOrStdout(), "Creating database %q... ", dbName)
			if err := postgres.CreateDatabase(ctx, gcfg.AdminDSN(), dbName, gcfg.PostgresUser); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "✗")
				return fmt.Errorf("project use: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓")

			projectDSN := gcfg.ProjectDSN(name)
			db, err := postgres.New(ctx, projectDSN)
			if err != nil {
				return fmt.Errorf("project use: connect to %s: %w", dbName, err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("project use: migrate: %w", err)
			}

			proj, err := db.GetProjectByName(ctx, name)
			if err != nil {
				proj = &domain.Project{Name: name}
				if err := db.CreateProject(ctx, proj); err != nil {
					return fmt.Errorf("project use: create project record: %w", err)
				}
			}
			_ = proj

			if err := config.Write(cwd, &config.ProjectConfig{
				Project:     name,
				Backend:     "postgres",
				DatabaseURL: projectDSN,
			}); err != nil {
				return fmt.Errorf("project use: write config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Project set to %q\n", name)
			fmt.Fprintln(cmd.OutOrStdout(), "Created .loar/project.toml")
			return nil
		},
	}
}

// newProjectCleanCmd returns `loar project clean`.
func newProjectCleanCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Wipe all observations and entities from the current project",
		Long: `Deletes all observations, entities, and entity links from the current project
database. The database and schema are preserved; only the data is removed.

Use --force to skip the confirmation prompt.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			db, cfg, err := openStore(cmd)
			if err != nil {
				return fmt.Errorf("project clean: %w", err)
			}
			defer db.Close()

			if !force {
				fmt.Fprintf(cmd.OutOrStdout(),
					"This will delete all observations and entities for project %q. Continue? [y/N] ",
					cfg.Project)
				var answer string
				fmt.Fscan(cmd.InOrStdin(), &answer)
				if answer != "y" && answer != "Y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			// CleanProject is on the concrete types (Postgres and SQLite) but not on
			// store.Store (it's an admin operation, not a query operation).
			// Open the concrete backend directly.
			if cfg.Backend == "local" {
				sqDB, err := sqstore.New(cfg.DatabaseURL)
				if err != nil {
					return fmt.Errorf("project clean: %w", err)
				}
				defer sqDB.Close()
				if err := sqDB.CleanProject(ctx); err != nil {
					return fmt.Errorf("project clean: %w", err)
				}
			} else {
				dsn := cfg.DatabaseURL
				if envDSN := os.Getenv("LOAR_DATABASE_URL"); envDSN != "" {
					dsn = envDSN
				}
				pgDB, err := postgres.New(ctx, dsn)
				if err != nil {
					return fmt.Errorf("project clean: connect: %w", err)
				}
				defer pgDB.Close()
				if err := pgDB.CleanProject(ctx); err != nil {
					return fmt.Errorf("project clean: %w", err)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Project %q cleaned. Re-run \"loar ingest\" to repopulate.\n", cfg.Project)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

// newProjectListCmd lists all loar-* databases (Postgres mode) or prints a
// notice in local mode.
func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
			}

			if gcfg.Backend == "local" {
				fmt.Fprintln(cmd.OutOrStdout(), "Local mode: projects are per-directory (.loar/loar.db).")
				fmt.Fprintln(cmd.OutOrStdout(), "No global project registry in local mode.")
				return nil
			}

			ctx := cmd.Context()
			names, err := postgres.ListLoarDatabases(ctx, gcfg.AdminDSN())
			if err != nil {
				return fmt.Errorf("project list: %w", err)
			}

			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No projects found.")
				return nil
			}
			for _, n := range names {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		},
	}
}

// newProjectDeleteCmd drops the loar-<name> database (Postgres only).
func newProjectDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a project and its database",
		Long: `Drops the loar-<name> Postgres database and removes .loar/project.toml.
In local mode, remove .loar/loar.db manually.

This is irreversible.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
			}

			if gcfg.Backend == "local" {
				fmt.Fprintln(cmd.OutOrStdout(), "Local mode: delete the .loar/loar.db file manually.")
				return nil
			}

			ctx := cmd.Context()
			dbName := "loar-" + name

			fmt.Fprintf(cmd.OutOrStdout(), "Dropping database %q... ", dbName)
			if err := postgres.DropDatabase(ctx, gcfg.AdminDSN(), dbName); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "✗")
				return fmt.Errorf("project delete: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓")

			cwd, err := os.Getwd()
			if err == nil {
				tomlPath := filepath.Join(cwd, ".loar", "project.toml")
				if cfg, err := config.Load(cwd); err == nil && cfg.Project == name {
					_ = os.Remove(tomlPath)
					_ = os.Remove(filepath.Join(cwd, ".loar"))
					fmt.Fprintln(cmd.OutOrStdout(), "Removed .loar/project.toml")
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Project %q deleted.\n", name)
			return nil
		},
	}
}


// NewProjectCmd returns the `loar project` command group.
