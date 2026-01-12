package formula

import (
	"context"

	"github.com/linagora/qsos-lng/pkg/database"
)

// Evaluator evaluates formulas with metric references
type Evaluator struct {
	store      database.MetricStore
	softwareID int64
}

// NewEvaluator creates a new formula evaluator
func NewEvaluator(store database.MetricStore, softwareID int64) *Evaluator {
	return &Evaluator{
		store:      store,
		softwareID: softwareID,
	}
}

// MetricValue holds a metric value retrieved from the database
type MetricValue struct {
	Value float64
	Valid bool
}

// MetricProvider provides metric values for formula evaluation
type MetricProvider interface {
	GetMetric(ctx context.Context, slug string) (float64, error)
}

// Direction indicates whether bigger or smaller values are better
type Direction string

const (
	BiggerIsBetter  Direction = "bigger_is_better"
	SmallerIsBetter Direction = "smaller_is_better"
)
