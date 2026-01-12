package database

import (
	"context"
	"fmt"
	"time"
)

// MetricValueInsert represents a metric value to be inserted
type MetricValueInsert struct {
	MetricSlug string
	Value      float64
	Source     string // e.g., "qsos-lng:github", "qsos-lng:lizard"
}

// ScoreInsert represents a score to be inserted
type ScoreInsert struct {
	FieldSlug   string
	Score       float64
	IsPublished bool
	IsManual    bool
}

// InsertMetricValue inserts a metric value into categories_metricvalue
func (db *DB) InsertMetricValue(ctx context.Context, lookup *MetricLookup, softwareID int64, insert MetricValueInsert) error {
	metricID, err := lookup.GetMetricID(insert.MetricSlug)
	if err != nil {
		return fmt.Errorf("cannot insert metric value: %w", err)
	}

	_, err = db.Conn.Exec(ctx, `
		INSERT INTO categories_metricvalue (metric_id, software_id, value, collected_at, source)
		VALUES ($1, $2, $3, $4, $5)
	`, metricID, softwareID, insert.Value, time.Now(), insert.Source)

	if err != nil {
		return fmt.Errorf("failed to insert metric value for '%s': %w", insert.MetricSlug, err)
	}

	return nil
}

// InsertMetricValues inserts multiple metric values in a batch
func (db *DB) InsertMetricValues(ctx context.Context, lookup *MetricLookup, softwareID int64, inserts []MetricValueInsert) error {
	for _, insert := range inserts {
		if err := db.InsertMetricValue(ctx, lookup, softwareID, insert); err != nil {
			return err
		}
	}
	return nil
}

// InsertScore inserts a score into categories_analysisresult
func (db *DB) InsertScore(ctx context.Context, fieldMap map[string]int64, softwareID int64, insert ScoreInsert) error {
	fieldID, exists := fieldMap[insert.FieldSlug]
	if !exists {
		return fmt.Errorf("field slug '%s' not found in field map", insert.FieldSlug)
	}

	_, err := db.Conn.Exec(ctx, `
		INSERT INTO categories_analysisresult (software_id, field_id, score, is_published, is_manual, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, softwareID, fieldID, insert.Score, insert.IsPublished, insert.IsManual)

	if err != nil {
		return fmt.Errorf("failed to insert score for field '%s': %w", insert.FieldSlug, err)
	}

	return nil
}

// InsertScores inserts multiple scores in a batch
func (db *DB) InsertScores(ctx context.Context, fieldMap map[string]int64, softwareID int64, inserts []ScoreInsert) error {
	for _, insert := range inserts {
		if err := db.InsertScore(ctx, fieldMap, softwareID, insert); err != nil {
			return err
		}
	}
	return nil
}

// UpdateSoftwareState updates the software state
func (db *DB) UpdateSoftwareState(ctx context.Context, softwareID int64, state string) error {
	_, err := db.Conn.Exec(ctx, `
		UPDATE categories_software
		SET state = $2
		WHERE id = $1
	`, softwareID, state)

	if err != nil {
		return fmt.Errorf("failed to update software state: %w", err)
	}

	return nil
}

// UpdateSoftwareFields updates multiple fields on a software record
func (db *DB) UpdateSoftwareFields(ctx context.Context, softwareID int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}

	// Build dynamic UPDATE query
	query := "UPDATE categories_software SET "
	args := []interface{}{softwareID}
	argNum := 2

	first := true
	for field, value := range fields {
		if !first {
			query += ", "
		}
		query += fmt.Sprintf("%s = $%d", field, argNum)
		args = append(args, value)
		argNum++
		first = false
	}

	query += " WHERE id = $1"

	_, err := db.Conn.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update software fields: %w", err)
	}

	return nil
}
