package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/linagora/qsos-lng/pkg/database"
	"github.com/linagora/qsos-lng/pkg/formula"
)

// Execute runs the full analysis pipeline for a project
func (e *Executor) Execute(ctx context.Context, execCtx *ExecutionContext, sources []SourceAdapter, metadata []MetadataAdapter) error {
	log.Printf("Starting execution for %s/%s (software_id=%d)\n", execCtx.Owner, execCtx.Repo, execCtx.SoftwareID)

	// Step 1: Collect metrics from all sources
	log.Printf("Step 1: Collecting metrics...\n")
	allMetrics := make([]MetricResult, 0)
	for _, source := range sources {
		log.Printf("  - Fetching from %s...\n", source.Name())
		metrics, err := source.Fetch(ctx, execCtx)
		if err != nil {
			return fmt.Errorf("failed to fetch from %s: %w", source.Name(), err)
		}
		allMetrics = append(allMetrics, metrics...)
		log.Printf("    Collected %d metrics\n", len(metrics))
	}

	// Step 2: Store metrics in database
	if e.cfg.Output.SaveMetrics {
		log.Printf("Step 2: Storing metrics in database...\n")
		inserts := make([]database.MetricValueInsert, 0, len(allMetrics))
		skipped := 0
		for _, m := range allMetrics {
			// Only store metrics that are configured in the TOML
			if !e.lookup.HasSlug(m.Slug) {
				log.Printf("  Skipping metric '%s' (not configured in TOML)\n", m.Slug)
				skipped++
				continue
			}
			inserts = append(inserts, database.MetricValueInsert{
				MetricSlug: m.Slug,
				Value:      m.Value,
				Source:     m.Source,
			})
		}
		if err := e.store.InsertMetricValues(ctx, e.lookup, execCtx.SoftwareID, inserts); err != nil {
			return fmt.Errorf("failed to store metrics: %w", err)
		}
		log.Printf("  Stored %d metrics (skipped %d unconfigured metrics)\n", len(inserts), skipped)
	}

	// Step 3: Evaluate scores using formulas
	log.Printf("Step 3: Evaluating scores...\n")
	evaluator := formula.NewEvaluator(e.store, execCtx.SoftwareID)
	scores := make([]database.ScoreInsert, 0, len(e.cfg.Scores))

	for _, scoreDef := range e.cfg.Scores {
		log.Printf("  - Evaluating score '%s'...\n", scoreDef.Slug)
		scoreValue, err := evaluator.Evaluate(ctx, scoreDef.Formula)
		if err != nil {
			return fmt.Errorf("failed to evaluate score '%s': %w", scoreDef.Slug, err)
		}

		scores = append(scores, database.ScoreInsert{
			FieldSlug:   scoreDef.Slug,
			Score:       scoreValue,
			IsPublished: e.cfg.Output.ScoreMetadata["is_published"],
			IsManual:    e.cfg.Output.ScoreMetadata["is_manual"],
		})
		log.Printf("    Score '%s' = %.2f\n", scoreDef.Slug, scoreValue)
	}

	// Step 4: Store scores in database
	if e.cfg.Output.SaveScores {
		log.Printf("Step 4: Storing scores in database...\n")
		if err := e.db.InsertScores(ctx, e.fieldMap, execCtx.SoftwareID, scores); err != nil {
			return fmt.Errorf("failed to store scores: %w", err)
		}
		log.Printf("  Stored %d scores\n", len(scores))
	}

	// Step 5: Execute metadata operations in a transaction
	if len(metadata) > 0 {
		log.Printf("Step 5: Executing metadata operations...\n")
		tx, err := e.db.Conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		for _, meta := range metadata {
			log.Printf("  - Executing %s...\n", meta.Name())
			if err := meta.Execute(ctx, execCtx, tx); err != nil {
				log.Printf("    Warning: Failed to execute %s: %v\n", meta.Name(), err)
				// Continue with other metadata operations
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit metadata transaction: %w", err)
		}
		log.Printf("  Metadata operations completed\n")
	}

	// Step 6: Update software state
	if execCtx.IsPublishedUpdate {
		// For published updates, preserve the 'published' state and update timestamp
		log.Printf("Step 6: Skipping state update (preserving 'published' state)\n")
		log.Printf("  Updating last_metrics_update_at timestamp...\n")
		if err := e.db.UpdateLastMetricsUpdateTime(ctx, execCtx.SoftwareID); err != nil {
			log.Printf("  Warning: Failed to update last_metrics_update_at: %v\n", err)
		}
	} else {
		// For draft projects, update state as configured
		if e.cfg.Output.UpdateSoftwareState != "" {
			log.Printf("Step 6: Updating software state to '%s'...\n", e.cfg.Output.UpdateSoftwareState)
			if err := e.db.UpdateSoftwareState(ctx, execCtx.SoftwareID, e.cfg.Output.UpdateSoftwareState); err != nil {
				return fmt.Errorf("failed to update software state: %w", err)
			}
		}
	}

	log.Printf("Execution completed successfully for %s/%s\n\n", execCtx.Owner, execCtx.Repo)
	return nil
}
