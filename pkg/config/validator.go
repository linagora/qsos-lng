package config

import (
	"fmt"
	"strings"
)

// Validate checks the configuration for correctness
func (c *Config) Validate() error {
	// Track all metric slugs defined in [[metrics]]
	definedMetrics := make(map[string]bool)
	for _, m := range c.Metrics {
		if m.Name == "" {
			return fmt.Errorf("metric definition missing 'name' field")
		}
		if len(m.Fields) == 0 {
			return fmt.Errorf("metric '%s' has no fields defined", m.Name)
		}
		for _, f := range m.Fields {
			if f.APIField == "" {
				return fmt.Errorf("metric '%s' has field with empty 'api_field'", m.Name)
			}
			if f.MetricSlug == "" {
				return fmt.Errorf("metric '%s' has field with empty 'metric_slug'", m.Name)
			}
			definedMetrics[f.MetricSlug] = true
		}
	}

	// Validate score definitions
	for _, s := range c.Scores {
		if s.Slug == "" {
			return fmt.Errorf("score definition missing 'slug' field")
		}
		if s.Formula == "" {
			return fmt.Errorf("score '%s' missing 'formula' field", s.Slug)
		}

		// Check that formula references valid metric slugs
		if err := validateFormulaMetricReferences(s.Formula, definedMetrics); err != nil {
			return fmt.Errorf("score '%s': %w", s.Slug, err)
		}
	}

	// Validate metadata operations
	for _, m := range c.Metadata {
		if m.Name == "" {
			return fmt.Errorf("metadata operation missing 'name' field")
		}
		if m.Type == "" {
			return fmt.Errorf("metadata operation '%s' missing 'type' field", m.Name)
		}
		if m.SaveTo == "" {
			return fmt.Errorf("metadata operation '%s' missing 'save_to' field", m.Name)
		}

		// Validate type
		validTypes := map[string]bool{
			"ai_summarize": true,
			"ai_tags":      true,
			"icon_url":     true,
		}
		if !validTypes[m.Type] {
			return fmt.Errorf("metadata operation '%s' has invalid type '%s' (must be: ai_summarize, ai_tags, icon_url)", m.Name, m.Type)
		}
	}

	// Validate output configuration
	if c.Output.SaveMetrics && c.Output.MetricsTable == "" {
		return fmt.Errorf("output.save_metrics is true but output.metrics_table is empty")
	}
	if c.Output.SaveScores && c.Output.ScoreTable == "" {
		return fmt.Errorf("output.save_scores is true but output.score_table is empty")
	}

	// Validate work mode configuration
	if c.WorkMode.EnablePublishedUpdates {
		if c.WorkMode.PublishedUpdateIntervalHours <= 0 {
			return fmt.Errorf("work_mode.published_update_interval_hours must be positive")
		}
		if c.WorkMode.PublishedBatchSize <= 0 {
			return fmt.Errorf("work_mode.published_batch_size must be positive")
		}
		if c.WorkMode.IdleSleepSeconds < 0 {
			return fmt.Errorf("work_mode.idle_sleep_seconds must be non-negative")
		}
	}

	return nil
}

// validateFormulaMetricReferences checks that metric references in formulas are defined
func validateFormulaMetricReferences(formula string, definedMetrics map[string]bool) error {
	// Simple check: look for "metric.slug_name" patterns
	// This is a basic validation; the formula evaluator will do more thorough checking
	parts := strings.Split(formula, "metric.")
	for i := 1; i < len(parts); i++ {
		// Extract the slug name (alphanumeric + underscore)
		slug := ""
		for _, ch := range parts[i] {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
				slug += string(ch)
			} else {
				break
			}
		}

		if slug != "" && !definedMetrics[slug] {
			return fmt.Errorf("formula references undefined metric 'metric.%s'", slug)
		}
	}

	return nil
}
