package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
	"github.com/jackc/pgx/v5"
	"github.com/linagora/qsos-lng/pkg/config"
	"github.com/linagora/qsos-lng/pkg/database"
	"github.com/linagora/qsos-lng/pkg/engine"
	"github.com/linagora/qsos-lng/pkg/formula"
	"github.com/linagora/qsos-lng/pkg/sources"
)

func usage() {
	log.Fatalf(`Usage:
- go run . analyze <owner/repo> for one-shot analysis of a project
- go run . work for working in background for l'Argus du Libre.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "work":
		work()
	case "analyze":
		if len(os.Args) != 3 {
			usage()
		}
		analyze(os.Args[2])
	default:
		usage()
	}
}

func work() {
	ctx := context.Background()

	// Setup credentials
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatalf("GITHUB_TOKEN environment variable is not set")
	}
	githubClient := github.NewClient(nil).WithAuthToken(githubToken)

	// Load TOML configuration
	cfg, err := config.LoadConfig("configs/qsos-default.toml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Configuration loaded successfully\n")

	// Connect to the database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatalf("DATABASE_URL environment variable is not set")
	}

	db, err := database.NewDB(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close(ctx)

	// Validate that required metrics exist in the database
	if err := db.ValidateMetrics(ctx, cfg); err != nil {
		log.Fatalf("Metric validation failed: %v\nPlease create the required metrics in Django admin", err)
	}
	log.Printf("Metric validation successful\n")

	// Build metric lookup cache
	lookup, err := db.BuildMetricLookup(ctx)
	if err != nil {
		log.Fatalf("Failed to build metric lookup: %v", err)
	}
	log.Printf("Metric lookup cache built: %d metrics\n", len(lookup.GetAllSlugs()))

	// Load field map
	fieldMap, err := db.ValidateFields(ctx)
	if err != nil {
		log.Fatalf("Failed to load fields: %v", err)
	}
	log.Printf("Field map loaded: %d fields\n", len(fieldMap))

	// Create metric store
	store := database.NewPostgresMetricStore(db)

	// Create execution engine
	executor := engine.NewExecutor(cfg, db, store, lookup, fieldMap, githubClient, githubToken)

	for {
		// Query for a draft project
		var projectID int
		var repositoryURL string
		err = db.Conn.QueryRow(ctx, `
			SELECT id, repository_url
			FROM categories_software
			WHERE state = 'draft'
			ORDER BY created_at ASC
			LIMIT 1
		`).Scan(&projectID, &repositoryURL)

		if err != nil {
			if err == pgx.ErrNoRows {
				time.Sleep(3 * time.Second)
				continue
			}
			log.Fatalf("Failed to query database: %v", err)
		}

		fmt.Printf("Processing project ID %d: %s\n", projectID, repositoryURL)

		// Parse repository URL
		parsedURL, err := url.Parse(repositoryURL)
		if err != nil {
			log.Printf("Failed to parse repository URL '%s': %v", repositoryURL, err)
			continue
		}

		parts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")
		if len(parts) < 2 {
			log.Printf("Invalid repository URL format '%s'", repositoryURL)
			continue
		}

		owner := parts[0]
		repo := strings.TrimSuffix(parts[1], ".git")

		// Get repository info
		repository, _, err := githubClient.Repositories.Get(ctx, owner, repo)
		if err != nil {
			log.Printf("Failed to fetch repository info: %v", err)
			continue
		}

		language := ""
		if repository.Language != nil {
			language = *repository.Language
		}

		isMirror := repository.MirrorURL != nil

		// Get website URL
		websiteURL := ""
		if repository.Homepage != nil && *repository.Homepage != "" {
			websiteURL = *repository.Homepage
		}

		// Create execution context
		execCtx := &engine.ExecutionContext{
			SoftwareID:    int64(projectID),
			Owner:         owner,
			Repo:          repo,
			RepositoryURL: repositoryURL,
			IsMirror:      isMirror,
			Language:      language,
		}

		// Create source adapters
		sourceAdapters := []engine.SourceAdapter{
			sources.NewGitHubSource(githubClient),
			sources.NewLizardSource(),
			sources.NewScorecardSource(githubToken),
			sources.NewDocumentationSource(githubClient),
		}

		// Create metadata adapters
		metadataAdapters := []engine.MetadataAdapter{
			sources.NewBilingualSummaryAdapter(githubClient),
			sources.NewTagsAdapter(githubClient),
			sources.NewIconAdapter(githubClient),
		}

		// Execute pipeline
		if err := executor.Execute(ctx, execCtx, sourceAdapters, metadataAdapters); err != nil {
			log.Printf("Failed to execute pipeline: %v\n", err)
			continue
		}

		// Update website URL if found
		if websiteURL != "" {
			_, err = db.Conn.Exec(ctx, "UPDATE categories_software SET website_url = $1 WHERE id = $2", websiteURL, projectID)
			if err != nil {
				log.Printf("Warning: Failed to update website URL: %v\n", err)
			}
		}

		fmt.Printf("Project ID %d analysis completed successfully\n\n", projectID)
	}
}

func analyze(project string) {
	ctx := context.Background()

	parts := strings.Split(project, "/")
	if len(parts) != 2 {
		log.Fatalf("Invalid project format. Must be in the format: owner/repo")
	}
	owner, repo := parts[0], parts[1]

	// Setup credentials
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatalf("GITHUB_TOKEN environment variable is not set")
	}
	githubClient := github.NewClient(nil).WithAuthToken(githubToken)

	// Load TOML configuration
	cfg, err := config.LoadConfig("configs/qsos-default.toml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Get repository info
	repository, _, err := githubClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch repository info: %v", err)
	}

	language := ""
	if repository.Language != nil {
		language = *repository.Language
	}

	isMirror := repository.MirrorURL != nil

	fmt.Printf("\n=== %s/%s ===\n", owner, repo)
	fmt.Printf("Language: %s\n", language)
	fmt.Printf("Is Mirror: %v\n", isMirror)

	// Create execution context with temporary software_id
	const tempSoftwareID = 1
	execCtx := &engine.ExecutionContext{
		SoftwareID:    tempSoftwareID,
		Owner:         owner,
		Repo:          repo,
		RepositoryURL: fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		IsMirror:      isMirror,
		Language:      language,
	}

	// Create source adapters
	sourceAdapters := []engine.SourceAdapter{
		sources.NewGitHubSource(githubClient),
		sources.NewLizardSource(),
		sources.NewScorecardSource(githubToken),
		sources.NewDocumentationSource(githubClient),
	}

	// Collect all metrics
	fmt.Printf("\n--- Fetching Metrics ---\n")
	allMetrics := make([]engine.MetricResult, 0)
	for _, source := range sourceAdapters {
		fmt.Printf("\n%s:\n", source.Name())
		metrics, err := source.Fetch(ctx, execCtx)
		if err != nil {
			log.Printf("  Error: %v\n", err)
			continue
		}
		for _, m := range metrics {
			fmt.Printf("  %s = %.2f\n", m.Slug, m.Value)
			allMetrics = append(allMetrics, m)
		}
	}

	// Create in-memory metric store
	store := database.NewInMemoryMetricStore()

	// Create a simple in-memory lookup (no validation needed for analyze mode)
	lookup := createInMemoryLookup(cfg)

	// Store all metrics in the in-memory store
	for _, m := range allMetrics {
		if lookup.HasSlug(m.Slug) {
			_ = store.InsertMetricValue(ctx, lookup, tempSoftwareID, database.MetricValueInsert{
				MetricSlug: m.Slug,
				Value:      m.Value,
				Source:     m.Source,
			})
		}
	}

	// Evaluate scores
	fmt.Printf("\n--- Computing Scores ---\n")
	evaluator := formula.NewEvaluator(store, tempSoftwareID)

	for _, scoreDef := range cfg.Scores {
		scoreValue, err := evaluator.Evaluate(ctx, scoreDef.Formula)
		if err != nil {
			fmt.Printf("  %s: Error - %v\n", scoreDef.Slug, err)
		} else {
			fmt.Printf("  %s = %.2f\n", scoreDef.Slug, scoreValue)
		}
	}

	fmt.Printf("\nAnalysis complete!\n")
}

// createInMemoryLookup creates a simple in-memory metric lookup from config
func createInMemoryLookup(cfg *config.Config) *database.MetricLookup {
	slugToID := make(map[string]int64)
	metrics := make(map[int64]*database.MetricInfo)

	var id int64 = 1
	for _, metricDef := range cfg.Metrics {
		for _, field := range metricDef.Fields {
			slugToID[field.MetricSlug] = id
			metrics[id] = &database.MetricInfo{
				ID:                id,
				Slug:              field.MetricSlug,
				Weight:            1,
				CollectionEnabled: true,
				FieldID:           0,
			}
			id++
		}
	}

	return database.NewInMemoryLookup(slugToID, metrics)
}
