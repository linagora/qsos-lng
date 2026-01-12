package database

import (
	"context"
	"fmt"
)

// MetricLookup provides fast slug → metric_id lookups
type MetricLookup struct {
	slugToID map[string]int64
	metrics  map[int64]*MetricInfo
}

// BuildMetricLookup creates a lookup cache from the database
func (db *DB) BuildMetricLookup(ctx context.Context) (*MetricLookup, error) {
	rows, err := db.Conn.Query(ctx, `
		SELECT id, slug, weight, collection_enabled, field_id
		FROM categories_metric
		WHERE collection_enabled = true
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories_metric: %w", err)
	}
	defer rows.Close()

	lookup := &MetricLookup{
		slugToID: make(map[string]int64),
		metrics:  make(map[int64]*MetricInfo),
	}

	for rows.Next() {
		var info MetricInfo
		if err := rows.Scan(&info.ID, &info.Slug, &info.Weight, &info.CollectionEnabled, &info.FieldID); err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w", err)
		}
		lookup.slugToID[info.Slug] = info.ID
		lookup.metrics[info.ID] = &info
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics: %w", err)
	}

	return lookup, nil
}

// GetMetricID returns the metric ID for a given slug
func (ml *MetricLookup) GetMetricID(slug string) (int64, error) {
	id, exists := ml.slugToID[slug]
	if !exists {
		return 0, fmt.Errorf("metric slug '%s' not found in lookup cache", slug)
	}
	return id, nil
}

// GetMetricInfo returns full metric information for a given slug
func (ml *MetricLookup) GetMetricInfo(slug string) (*MetricInfo, error) {
	id, err := ml.GetMetricID(slug)
	if err != nil {
		return nil, err
	}
	return ml.metrics[id], nil
}

// GetAllSlugs returns all known metric slugs
func (ml *MetricLookup) GetAllSlugs() []string {
	slugs := make([]string, 0, len(ml.slugToID))
	for slug := range ml.slugToID {
		slugs = append(slugs, slug)
	}
	return slugs
}

// HasSlug checks if a metric slug exists in the lookup cache
func (ml *MetricLookup) HasSlug(slug string) bool {
	_, exists := ml.slugToID[slug]
	return exists
}

// NewInMemoryLookup creates a MetricLookup from maps (for testing/analyze mode)
func NewInMemoryLookup(slugToID map[string]int64, metrics map[int64]*MetricInfo) *MetricLookup {
	return &MetricLookup{
		slugToID: slugToID,
		metrics:  metrics,
	}
}
