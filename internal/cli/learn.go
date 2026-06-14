package cli

import (
	"fmt"
	"os"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/entity"
	"github.com/balpal4495/loar/internal/store/postgres"
	"github.com/spf13/cobra"
)

// NewLearnCmd returns the `loar learn` command.
func NewLearnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Extract and link entities from all stored observations",
		Long: `Scans every observation already in the database and extracts entities
(phases, components, hooks, commits, file paths) from their content,
then writes the results back as entity links.

Run this after a bulk ingest to backfill entity extraction, or whenever
the extraction rules have been updated and you want to re-learn from existing data.

Example:
  loar learn`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dsn := mustProjectDSN(cmd)
			ctx := cmd.Context()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("learn: %w", err)
			}

			cfg, _, err := config.Find(cwd)
			if err != nil {
				return fmt.Errorf("learn: %w", err)
			}

			db, err := postgres.New(ctx, dsn)
			if err != nil {
				return fmt.Errorf("learn: connect: %w", err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("learn: migrate: %w", err)
			}

			proj, err := db.GetProjectByName(ctx, cfg.Project)
			if err != nil {
				return fmt.Errorf("learn: project %q not found; run \"loar project use\" first", cfg.Project)
			}

			observations, err := db.ListObservations(ctx, proj.ID)
			if err != nil {
				return fmt.Errorf("learn: list observations: %w", err)
			}

			if len(observations) == 0 {
				fmt.Println("No observations found — nothing to learn from.")
				return nil
			}

			fmt.Printf("Learning from %d observations...\n", len(observations))

			linked := 0
			for _, obs := range observations {
				n := entity.ExtractAndLink(ctx, db, proj.ID, obs)
				linked += n
			}

			fmt.Printf("Done. Extracted and linked %d entity references across %d observations.\n", linked, len(observations))
			return nil
		},
	}
	return cmd
}
