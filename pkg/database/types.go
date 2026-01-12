package database

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DB wraps the database connection with helper methods
type DB struct {
	Conn *pgx.Conn
}

// MetricInfo holds information about a metric from the database
type MetricInfo struct {
	ID                int64
	Slug              string
	Weight            int
	CollectionEnabled bool
	FieldID           int64
}

// FieldInfo holds information about a category field
type FieldInfo struct {
	ID   int64
	Slug string
}

// NewDB creates a new database wrapper
func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &DB{Conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close(ctx context.Context) error {
	return db.Conn.Close(ctx)
}
