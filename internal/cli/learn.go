package cli

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/balpal4495/loar/internal/domain"
	"github.com/balpal4495/loar/internal/entity"
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
			ctx := cmd.Context()

			db, cfg, err := openStore(cmd)
			if err != nil {
				return fmt.Errorf("learn: %w", err)
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

			// Compute and record entity confidence for every entity in the project.
			// This is the append-only time-series that lets trend be queried over time.
			entities, err := db.ListEntities(ctx, proj.ID)
			if err != nil {
				return fmt.Errorf("learn: list entities: %w", err)
			}

			now := time.Now()
			confidenceRows := 0
			for _, ent := range entities {
				obsForEnt, err := db.ObservationsForEntity(ctx, ent.ID)
				if err != nil || len(obsForEnt) == 0 {
					continue
				}
				score := computeEntityScore(obsForEnt, now)
				resolved := 0
				for _, o := range obsForEnt {
					if o.Temporal.ResolvedAt != nil {
						resolved++
					}
				}
				ec := &domain.EntityConfidence{
					EntityID:         ent.ID,
					ProjectID:        proj.ID,
					Score:            score,
					ObservationCount: len(obsForEnt),
					ResolvedCount:    resolved,
					ComputedAt:       now,
				}
				if err := db.WriteEntityConfidence(ctx, ec); err != nil {
					fmt.Fprintf(os.Stderr, "warn: entity confidence write failed for %s: %v\n", ent.CanonicalName, err)
					continue
				}
				confidenceRows++
			}

			fmt.Printf("Done. Extracted and linked %d entity references across %d observations.\n", linked, len(observations))
			fmt.Printf("Recorded confidence scores for %d entities.\n", confidenceRows)
			return nil
		},
	}
	return cmd
}

// computeEntityScore returns the mean recency-weighted confidence for a set of
// observations. Mirrors the formula in retrieval/engine.go recencyScore.
func computeEntityScore(obs []*domain.Observation, now time.Time) float64 {
	if len(obs) == 0 {
		return 0.5
	}
	total := 0.0
	for _, o := range obs {
		ref := o.Temporal.OccurredAt
		if ref == nil {
			ref = o.Temporal.ObservedAt
		}
		if ref == nil {
			total += 0.5
			continue
		}
		days := now.Sub(*ref).Hours() / 24
		if days < 0 {
			days = 0
		}
		total += 0.5 + 0.5*math.Exp(-days/365)
	}
	return total / float64(len(obs))
}
