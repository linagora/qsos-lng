package database

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MetricStore defines the interface for storing and retrieving metrics
type MetricStore interface {
	// GetMetricValue retrieves the latest metric value for a given software and metric slug
	GetMetricValue(ctx context.Context, softwareID int64, metricSlug string) (float64, error)

	// InsertMetricValue stores a metric value
	InsertMetricValue(ctx context.Context, lookup *MetricLookup, softwareID int64, insert MetricValueInsert) error

	// InsertMetricValues stores multiple metric values
	InsertMetricValues(ctx context.Context, lookup *MetricLookup, softwareID int64, inserts []MetricValueInsert) error
}

// PostgresMetricStore implements MetricStore using PostgreSQL
type PostgresMetricStore struct {
	db *DB
}

// NewPostgresMetricStore creates a new PostgreSQL-backed metric store
func NewPostgresMetricStore(db *DB) *PostgresMetricStore {
	return &PostgresMetricStore{db: db}
}

// GetMetricValue retrieves the latest metric value from PostgreSQL
func (s *PostgresMetricStore) GetMetricValue(ctx context.Context, softwareID int64, metricSlug string) (float64, error) {
	// This requires lookup to get metric ID, but we'll query by slug directly for simplicity
	var value float64
	err := s.db.Conn.QueryRow(ctx, `
		SELECT mv.value
		FROM categories_metricvalue mv
		JOIN categories_metric m ON mv.metric_id = m.id
		WHERE m.slug = $1 AND mv.software_id = $2
		ORDER BY mv.collected_at DESC
		LIMIT 1
	`, metricSlug, softwareID).Scan(&value)

	if err != nil {
		return 0, fmt.Errorf("failed to get metric '%s': %w", metricSlug, err)
	}

	return value, nil
}

// InsertMetricValue stores a metric value in PostgreSQL
func (s *PostgresMetricStore) InsertMetricValue(ctx context.Context, lookup *MetricLookup, softwareID int64, insert MetricValueInsert) error {
	return s.db.InsertMetricValue(ctx, lookup, softwareID, insert)
}

// InsertMetricValues stores multiple metric values in PostgreSQL
func (s *PostgresMetricStore) InsertMetricValues(ctx context.Context, lookup *MetricLookup, softwareID int64, inserts []MetricValueInsert) error {
	return s.db.InsertMetricValues(ctx, lookup, softwareID, inserts)
}

// InMemoryMetricStore implements MetricStore using an in-memory map
type InMemoryMetricStore struct {
	mu      sync.RWMutex
	metrics map[string]metricEntry // key: "softwareID:slug"
}

type metricEntry struct {
	value       float64
	collectedAt time.Time
}

// NewInMemoryMetricStore creates a new in-memory metric store
func NewInMemoryMetricStore() *InMemoryMetricStore {
	return &InMemoryMetricStore{
		metrics: make(map[string]metricEntry),
	}
}

// GetMetricValue retrieves the latest metric value from memory
func (s *InMemoryMetricStore) GetMetricValue(ctx context.Context, softwareID int64, metricSlug string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%d:%s", softwareID, metricSlug)
	entry, exists := s.metrics[key]
	if !exists {
		return 0, fmt.Errorf("metric '%s' not found for software %d", metricSlug, softwareID)
	}

	return entry.value, nil
}

// InsertMetricValue stores a metric value in memory
func (s *InMemoryMetricStore) InsertMetricValue(ctx context.Context, lookup *MetricLookup, softwareID int64, insert MetricValueInsert) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%d:%s", softwareID, insert.MetricSlug)
	s.metrics[key] = metricEntry{
		value:       insert.Value,
		collectedAt: time.Now(),
	}

	return nil
}

// InsertMetricValues stores multiple metric values in memory
func (s *InMemoryMetricStore) InsertMetricValues(ctx context.Context, lookup *MetricLookup, softwareID int64, inserts []MetricValueInsert) error {
	for _, insert := range inserts {
		if err := s.InsertMetricValue(ctx, lookup, softwareID, insert); err != nil {
			return err
		}
	}
	return nil
}

// GetAllMetrics returns all stored metrics (useful for debugging/display)
func (s *InMemoryMetricStore) GetAllMetrics(softwareID int64) map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]float64)
	prefix := fmt.Sprintf("%d:", softwareID)

	for key, entry := range s.metrics {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			slug := key[len(prefix):]
			result[slug] = entry.value
		}
	}

	return result
}
