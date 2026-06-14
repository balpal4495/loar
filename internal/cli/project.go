package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/store/postgres"
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
	return cmd
}

// newProjectUseCmd returns `loar project use [name]`.
// If name is omitted, the current directory name is used (Option C).
func newProjectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [name]",
		Short: "Associate the current directory with a project",
		Long: `Associates the current directory with a named project.

If no name is given, the current directory name is used.

A dedicated Postgres database (loar-<name>) is created if it does not already
exist, and .loar/project.toml is written with the full connection string.

Requires ~/.config/loar/config.toml — run ` + "`loar setup`" + ` first.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("project use: %w", err)
			}

			// Option C: default to directory name when no arg is given.
			name := filepath.Base(cwd)
			if len(args) == 1 {
				name = args[0]
			}

			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			// Create the project database (loar-<name>) if it does not exist.
			dbName := "loar-" + name
			fmt.Fprintf(cmd.OutOrStdout(), "Creating database %q... ", dbName)
			if err := postgres.CreateDatabase(ctx, gcfg.AdminDSN(), dbName, gcfg.PostgresUser); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "✗")
				return fmt.Errorf("project use: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓")

			// Connect to the project database and run migrations.
			projectDSN := gcfg.ProjectDSN(name)
			db, err := postgres.New(ctx, projectDSN)
			if err != nil {
				return fmt.Errorf("project use: connect to %s: %w", dbName, err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("project use: migrate: %w", err)
			}

			// Ensure the project record exists inside the project database.
			proj, err := db.GetProjectByName(ctx, name)
			if err != nil {
				proj = &domain.Project{Name: name}
				if err := db.CreateProject(ctx, proj); err != nil {
					return fmt.Errorf("project use: create project record: %w", err)
				}
			}
			_ = proj

			// Write .loar/project.toml with the project name and DSN.
			if err := config.Write(cwd, &config.ProjectConfig{
				Project:     name,
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

// newProjectListCmd returns `loar project list`.
// Lists all loar-* databases on the configured Postgres server.
func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
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

// newProjectDeleteCmd returns `loar project delete <name>`.
// Drops the loar-<name> database and removes .loar/project.toml.
func newProjectDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a project and its database",
		Long: `Drops the loar-<name> Postgres database and removes .loar/project.toml
from the current directory (if it references this project).

This is irreversible.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			gcfg, err := mustGlobalConfig(cmd)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			dbName := "loar-" + name

			fmt.Fprintf(cmd.OutOrStdout(), "Dropping database %q... ", dbName)
			if err := postgres.DropDatabase(ctx, gcfg.AdminDSN(), dbName); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "✗")
				return fmt.Errorf("project delete: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓")

			// Remove .loar/project.toml if it references this project.
			cwd, err := os.Getwd()
			if err == nil {
				tomlPath := filepath.Join(cwd, ".loar", "project.toml")
				if cfg, err := config.Load(cwd); err == nil && cfg.Project == name {
					_ = os.Remove(tomlPath)
					// Remove .loar/ dir if now empty.
					_ = os.Remove(filepath.Join(cwd, ".loar"))
					fmt.Fprintln(cmd.OutOrStdout(), "Removed .loar/project.toml")
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Project %q deleted.\n", name)
			return nil
		},
	}
}
