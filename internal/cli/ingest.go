package cli

import (
	"fmt"
	"os"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/ingestion"
	"github.com/balpal4495/loar/internal/store/postgres"
	"github.com/spf13/cobra"
)

// NewIngestCmd returns the `loar ingest` command.
func NewIngestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest [file|url|-]",
		Short: "Ingest data into the current project",
		Long: `Ingests data from a file, URL, or stdin into the current project.

Supported formats: NDJSON (one JSON object per line) or a JSON array.

Examples:
  loar ingest transfers.json
  loar ingest https://example.com/feed.ndjson
  cat data.ndjson | loar ingest`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dsn := mustDSN(cmd)
			ctx := cmd.Context()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}

			cfg, _, err := config.Find(cwd)
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}

			db, err := postgres.New(ctx, dsn)
			if err != nil {
				return fmt.Errorf("ingest: connect: %w", err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("ingest: migrate: %w", err)
			}

			proj, err := db.GetProjectByName(ctx, cfg.Project)
			if err != nil {
				return fmt.Errorf("ingest: project %q not found; run \"loar project use %s\" first", cfg.Project, cfg.Project)
			}

			ing := ingestion.New(db, proj.ID)

			var count int
			if len(args) == 0 || args[0] == "-" {
				count, err = ing.IngestReader(ctx, cmd.InOrStdin(), "stdin")
			} else if isURL(args[0]) {
				count, err = ing.IngestURL(ctx, args[0])
			} else {
				count, err = ing.IngestFile(ctx, args[0])
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Ingested %d observation(s) into project %q\n", count, cfg.Project)
			return nil
		},
	}
}

// isURL returns true when s starts with http:// or https://.
func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}
