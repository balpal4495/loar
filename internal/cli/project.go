package cli

import (
	"fmt"
	"os"

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
	return cmd
}

// newProjectUseCmd returns `loar project use <name>`.
func newProjectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Associate the current directory with a project",
		Long: `Associates the current directory with a named project by writing
.loar/project.toml. If the project does not exist in the database it is
created automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dsn := mustDSN(cmd)

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("project use: %w", err)
			}

			ctx := cmd.Context()
			db, err := postgres.New(ctx, dsn)
			if err != nil {
				return fmt.Errorf("project use: connect: %w", err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("project use: migrate: %w", err)
			}

			// Ensure the project exists in the database.
			proj, err := db.GetProjectByName(ctx, name)
			if err != nil {
				// Project not found – create it.
				proj = &domain.Project{Name: name}
				if err := db.CreateProject(ctx, proj); err != nil {
					return fmt.Errorf("project use: create project: %w", err)
				}
				// Re-fetch to get the generated ID.
				proj, err = db.GetProjectByName(ctx, name)
				if err != nil {
					return fmt.Errorf("project use: fetch project: %w", err)
				}
			}
			_ = proj

			if err := config.Write(cwd, name); err != nil {
				return fmt.Errorf("project use: write config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Project set to %q\n", name)
			fmt.Fprintf(cmd.OutOrStdout(), "Created .loar/project.toml\n")
			return nil
		},
	}
}

// newProjectListCmd returns `loar project list`.
func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := mustDSN(cmd)
			ctx := cmd.Context()

			db, err := postgres.New(ctx, dsn)
			if err != nil {
				return fmt.Errorf("project list: connect: %w", err)
			}
			defer db.Close()

			projects, err := db.ListProjects(ctx)
			if err != nil {
				return fmt.Errorf("project list: %w", err)
			}

			if len(projects) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No projects found.")
				return nil
			}
			for _, p := range projects {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", p.Name)
			}
			return nil
		},
	}
}
