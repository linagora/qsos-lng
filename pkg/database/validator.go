package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/linagora/qsos-lng/pkg/config"
)

// ValidateMetrics checks that all metrics referenced in the config exist in the database
func (db *DB) ValidateMetrics(ctx context.Context, cfg *config.Config) error {
	// Collect all metric slugs referenced in the config
	requiredSlugs := make(map[string]bool)
	for _, m := range cfg.Metrics {
		for _, f := range m.Fields {
			requiredSlugs[f.MetricSlug] = true
		}
	}

	// Query database for existing metrics
	rows, err := db.Conn.Query(ctx, `
		SELECT slug, collection_enabled
		FROM categories_metric
	`)
	if err != nil {
		return fmt.Errorf("failed to query categories_metric: %w", err)
	}
	defer rows.Close()

	existingSlugs := make(map[string]bool)
	disabledSlugs := make(map[string]bool)
	for rows.Next() {
		var slug string
		var enabled bool
		if err := rows.Scan(&slug, &enabled); err != nil {
			return fmt.Errorf("failed to scan metric row: %w", err)
		}
		existingSlugs[slug] = true
		if !enabled {
			disabledSlugs[slug] = true
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating metrics: %w", err)
	}

	// Check for missing metrics
	var missingMetrics []string
	for slug := range requiredSlugs {
		if !existingSlugs[slug] {
			missingMetrics = append(missingMetrics, slug)
		}
	}

	if len(missingMetrics) > 0 {
		return fmt.Errorf("the following metrics are not defined in Django admin (categories_metric table): %s. Please create them in Django admin before running QSOS-LNG", strings.Join(missingMetrics, ", "))
	}

	// Warn about disabled metrics
	var disabledMetrics []string
	for slug := range requiredSlugs {
		if disabledSlugs[slug] {
			disabledMetrics = append(disabledMetrics, slug)
		}
	}

	if len(disabledMetrics) > 0 {
		return fmt.Errorf("the following metrics are disabled (collection_enabled=false): %s. Please enable them in Django admin or remove them from the config", strings.Join(disabledMetrics, ", "))
	}

	return nil
}

// ValidateFields checks that all category fields exist in the database
func (db *DB) ValidateFields(ctx context.Context) (map[string]int64, error) {
	rows, err := db.Conn.Query(ctx, "SELECT id, slug FROM categories_field")
	if err != nil {
		return nil, fmt.Errorf("failed to query categories_field: %w", err)
	}
	defer rows.Close()

	fieldMap := make(map[string]int64)
	for rows.Next() {
		var fieldID int64
		var slug string
		if err := rows.Scan(&fieldID, &slug); err != nil {
			return nil, fmt.Errorf("failed to scan field row: %w", err)
		}
		fieldMap[slug] = fieldID
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating fields: %w", err)
	}

	if len(fieldMap) == 0 {
		return nil, fmt.Errorf("no category fields found in database (categories_field table)")
	}

	return fieldMap, nil
}
