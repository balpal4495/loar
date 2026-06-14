package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/balpal4495/loar/internal/config"
	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/retrieval"
	"github.com/balpal4495/loar/internal/store/postgres"
	"github.com/spf13/cobra"
)

// NewQueryCmd returns the `loar` default command for natural-language queries.
// It is registered as the root command's RunE so that `loar "question"` works.
func NewQueryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query <question>",
		Short: "Query the knowledge store",
		Long: `Runs a natural-language query against the current project's knowledge store
and returns a structured context package.

Example:
  loar query "Why is Romano reliable?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: runQuery,
	}
}

// NewExplainCmd returns the `loar explain` command.
func NewExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <question>",
		Short: "Produce a human-readable explanation based on evidence",
		Long: `Retrieves evidence from the knowledge store and produces a
human-readable narrative explanation.

Example:
  loar explain "Why is Romano reliable?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: runExplain,
	}
}

func runQuery(cmd *cobra.Command, args []string) error {
	pkg, err := retrieve(cmd, args)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Query:      %s\n", pkg.Query)
	fmt.Fprintf(w, "Confidence: %.0f%%\n\n", pkg.Confidence*100)
	fmt.Fprintf(w, "%s\n", pkg.Summary)

	if len(pkg.Evidence) > 0 {
		fmt.Fprintln(w, "Evidence:")
		for i, ev := range pkg.Evidence {
			fmt.Fprintf(w, "  [%d] %s\n", i+1, ev.Summary)
		}
	}

	if len(pkg.Contradictions) > 0 {
		fmt.Fprintln(w, "\nContradictions:")
		for _, c := range pkg.Contradictions {
			fmt.Fprintf(w, "  - %s\n", c.Summary)
		}
	}

	if len(pkg.RelatedTopics) > 0 {
		fmt.Fprintf(w, "\nRelated topics: %s\n", strings.Join(pkg.RelatedTopics, ", "))
	}

	return nil
}

func runExplain(cmd *cobra.Command, args []string) error {
	pkg, err := retrieve(cmd, args)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Explanation for: %q\n\n", pkg.Query)
	fmt.Fprintln(w, pkg.Summary)

	if len(pkg.Timeline) > 0 {
		fmt.Fprintln(w, "Timeline:")
		for _, ev := range pkg.Timeline {
			ts := "<unknown time>"
			if ev.Temporal.OccurredAt != nil {
				ts = ev.Temporal.OccurredAt.Format("2006-01-02")
			} else if ev.Temporal.ObservedAt != nil {
				ts = ev.Temporal.ObservedAt.Format("2006-01-02")
			}
			fmt.Fprintf(w, "  %s  %s\n", ts, ev.Summary)
		}
	}

	if len(pkg.Evidence) > 0 {
		fmt.Fprintln(w, "\nSupporting evidence:")
		for i, ev := range pkg.Evidence {
			fmt.Fprintf(w, "  [%d] %s\n", i+1, ev.Summary)
		}
	}

	if len(pkg.Entities) > 0 {
		fmt.Fprintln(w, "\nEntities mentioned:")
		for _, e := range pkg.Entities {
			fmt.Fprintf(w, "  %s (%s)\n", e.CanonicalName, e.Type)
		}
	}

	return nil
}

// retrieve runs the full retrieval pipeline and returns the ContextPackage.
func retrieve(cmd *cobra.Command, args []string) (*domain.ContextPackage, error) {
	question := strings.Join(args, " ")
	dsn := mustDSN(cmd)
	ctx := cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	cfg, _, err := config.Find(cwd)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	db, err := postgres.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("query: connect: %w", err)
	}
	defer db.Close()

	proj, err := db.GetProjectByName(ctx, cfg.Project)
	if err != nil {
		return nil, fmt.Errorf("query: project %q not found; run \"loar project use %s\" first", cfg.Project, cfg.Project)
	}

	engine := retrieval.New(db)
	return engine.Query(ctx, proj.ID, question)
}
