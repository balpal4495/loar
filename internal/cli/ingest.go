package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/balpal4495/loar/internal/ingestion"
	"github.com/spf13/cobra"
)

// NewIngestCmd returns the `loar ingest` command.
func NewIngestCmd() *cobra.Command {
	var recursive bool
	var skipErrors bool
	var incremental bool

	cmd := &cobra.Command{
		Use:   "ingest [file|dir|url|-]",
		Short: "Ingest data into the current project",
		Long: `Ingests data from a file, directory, URL, or stdin into the current project.

Supported formats: NDJSON (one JSON object per line), JSON array.
Supported file extensions: .json, .ndjson, .jsonl

Examples:
  loar ingest transfers.json
  loar ingest ./data/
  loar ingest ./data/ --recursive
  loar ingest https://example.com/feed.ndjson
  cat data.ndjson | loar ingest
  loar ingest ./committed --incremental   # skip already-ingested records`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			db, cfg, err := openStore(cmd)
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
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
			ing.Incremental = incremental

			var count int
			switch {
			case len(args) == 0 || args[0] == "-":
				count, err = ing.IngestReader(ctx, cmd.InOrStdin(), "stdin")
				if err != nil {
					return err
				}

			case isURL(args[0]):
				count, err = ing.IngestURL(ctx, args[0])
				if err != nil {
					return err
				}

			default:
				info, statErr := os.Stat(args[0])
				if statErr != nil {
					return fmt.Errorf("ingest: %w", statErr)
				}
				if info.IsDir() {
					var errs []error
					count, errs = ing.IngestDir(ctx, args[0], recursive)
					if !skipErrors {
						for _, e := range errs {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", e)
						}
					}
				} else {
					count, err = ing.IngestFile(ctx, args[0])
					if err != nil {
						return err
					}
				}
			}

			if incremental {
				fmt.Fprintf(cmd.OutOrStdout(), "Ingested %d observation(s) into project %q (incremental — duplicates skipped)\n", count, cfg.Project)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Ingested %d observation(s) into project %q\n", count, cfg.Project)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recurse into subdirectories")
	cmd.Flags().BoolVar(&skipErrors, "skip-errors", false, "Suppress warnings for unreadable or invalid files")
	cmd.Flags().BoolVarP(&incremental, "incremental", "i", false, "Skip records whose source ID already exists in the store")
	return cmd
}

// isURL returns true when s starts with http:// or https://.
func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

