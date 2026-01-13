package engine

import (
	"context"

	"github.com/google/go-github/v76/github"
	"github.com/jackc/pgx/v5"
	"github.com/linagora/qsos-lng/pkg/config"
	"github.com/linagora/qsos-lng/pkg/database"
)

// Executor orchestrates the execution of metrics, scores, and metadata
type Executor struct {
	cfg          *config.Config
	db           *database.DB
	store        database.MetricStore
	lookup       *database.MetricLookup
	fieldMap     map[string]int64
	githubClient *github.Client
	githubToken  string
}

// ExecutionContext holds context for a single execution run
type ExecutionContext struct {
	SoftwareID        int64
	Owner             string
	Repo              string
	RepositoryURL     string
	IsMirror          bool
	Language          string
	IsPublishedUpdate bool // true for published project updates (skip metadata, preserve state)
}

// NewExecutor creates a new execution engine
func NewExecutor(
	cfg *config.Config,
	db *database.DB,
	store database.MetricStore,
	lookup *database.MetricLookup,
	fieldMap map[string]int64,
	githubClient *github.Client,
	githubToken string,
) *Executor {
	return &Executor{
		cfg:          cfg,
		db:           db,
		store:        store,
		lookup:       lookup,
		fieldMap:     fieldMap,
		githubClient: githubClient,
		githubToken:  githubToken,
	}
}

// MetricResult represents a collected metric value
type MetricResult struct {
	Slug   string
	Value  float64
	Source string
}

// SourceAdapter defines the interface for data source adapters
type SourceAdapter interface {
	Name() string
	Fetch(ctx context.Context, execCtx *ExecutionContext) ([]MetricResult, error)
}

// MetadataAdapter defines the interface for metadata operations
type MetadataAdapter interface {
	Name() string
	Execute(ctx context.Context, execCtx *ExecutionContext, tx pgx.Tx) error
}
