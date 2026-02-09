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
	"github.com/linagora/qsos-lng/metadata"
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
- go run . retag for regenerating tags on all published projects.
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
	case "retag":
		retag()
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

	dockerTimeout := cfg.WorkMode.DockerTimeoutMinutes

	for {
		// Priority 1: Draft projects (immediate processing)
		project, err := db.GetNextDraftProject(ctx)
		if err == nil {
			processDraftProject(ctx, project, executor, githubClient, githubToken, db, dockerTimeout)
			continue
		}

		if err != pgx.ErrNoRows {
			log.Fatalf("Failed to query database: %v", err)
		}

		// Priority 2: Published updates (idle time only)
		if cfg.WorkMode.EnablePublishedUpdates {
			projects, err := db.GetNextPublishedProjectToUpdate(ctx,
				cfg.WorkMode.PublishedUpdateIntervalHours,
				cfg.WorkMode.PublishedBatchSize)

			if err != nil {
				log.Printf("Failed to query published projects: %v", err)
			} else if len(projects) > 0 {
				for _, proj := range projects {
					processPublishedProject(ctx, &proj, executor, githubClient, githubToken, db, dockerTimeout)
				}
				continue
			}
		}

		// No work available
		sleepDuration := time.Duration(cfg.WorkMode.IdleSleepSeconds) * time.Second
		time.Sleep(sleepDuration)
	}
}

// buildExecutionContext creates ExecutionContext from repository URL
func buildExecutionContext(ctx context.Context, projectID int64, repositoryURL string,
	githubClient *github.Client, isPublishedUpdate bool) (*engine.ExecutionContext, string) {

	// Parse repository URL
	parsedURL, err := url.Parse(repositoryURL)
	if err != nil {
		log.Printf("Failed to parse repository URL '%s': %v", repositoryURL, err)
		return nil, ""
	}

	parts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")
	if len(parts) < 2 {
		log.Printf("Invalid repository URL format '%s'", repositoryURL)
		return nil, ""
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	// Get repository info
	repository, _, err := githubClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Printf("Failed to fetch repository info: %v", err)
		return nil, ""
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

	return &engine.ExecutionContext{
		SoftwareID:        projectID,
		Owner:             owner,
		Repo:              repo,
		RepositoryURL:     repositoryURL,
		IsMirror:          isMirror,
		Language:          language,
		IsPublishedUpdate: isPublishedUpdate,
	}, websiteURL
}

// buildSourceAdapters creates all source adapters
func buildSourceAdapters(githubClient *github.Client, githubToken string, dockerTimeoutMinutes int) []engine.SourceAdapter {
	return []engine.SourceAdapter{
		sources.NewGitHubSource(githubClient),
		sources.NewLizardSource(dockerTimeoutMinutes),
		sources.NewScorecardSource(githubToken, dockerTimeoutMinutes),
		sources.NewDocumentationSource(githubClient),
	}
}

// buildMetadataAdapters creates all metadata adapters
func buildMetadataAdapters(githubClient *github.Client) []engine.MetadataAdapter {
	return []engine.MetadataAdapter{
		sources.NewBilingualSummaryAdapter(githubClient),
		sources.NewTagsAdapter(githubClient),
		sources.NewIconAdapter(githubClient),
	}
}

// processDraftProject handles draft project analysis with full pipeline
func processDraftProject(ctx context.Context, project *database.ProjectInfo,
	executor *engine.Executor, githubClient *github.Client, githubToken string, db *database.DB, dockerTimeoutMinutes int) {

	fmt.Printf("Processing DRAFT project ID %d: %s\n", project.ID, project.RepositoryURL)

	execCtx, websiteURL := buildExecutionContext(ctx, project.ID, project.RepositoryURL, githubClient, false)
	if execCtx == nil {
		log.Printf("Failed to build execution context, setting project to error state\n")
		if err := db.UpdateSoftwareState(ctx, project.ID, "error"); err != nil {
			log.Printf("Failed to update software state to error: %v\n", err)
		}
		return
	}

	// Full pipeline: sources + metadata
	sourceAdapters := buildSourceAdapters(githubClient, githubToken, dockerTimeoutMinutes)
	metadataAdapters := buildMetadataAdapters(githubClient)

	if err := executor.Execute(ctx, execCtx, sourceAdapters, metadataAdapters); err != nil {
		log.Printf("Failed to execute pipeline: %v\n", err)
		log.Printf("Setting project to error state\n")
		if err := db.UpdateSoftwareState(ctx, project.ID, "error"); err != nil {
			log.Printf("Failed to update software state to error: %v\n", err)
		}
		return
	}

	// Update website URL if found
	if websiteURL != "" {
		_, err := db.Conn.Exec(ctx, "UPDATE categories_software SET website_url = $1 WHERE id = $2", websiteURL, project.ID)
		if err != nil {
			log.Printf("Warning: Failed to update website URL: %v\n", err)
		}
	}

	fmt.Printf("Draft project ID %d analysis completed successfully\n\n", project.ID)
}

// processPublishedProject handles published project updates (metrics + scores only)
func processPublishedProject(ctx context.Context, project *database.ProjectInfo,
	executor *engine.Executor, githubClient *github.Client, githubToken string, db *database.DB, dockerTimeoutMinutes int) {

	fmt.Printf("Updating PUBLISHED project ID %d: %s\n", project.ID, project.RepositoryURL)

	execCtx, websiteURL := buildExecutionContext(ctx, project.ID, project.RepositoryURL, githubClient, true)
	if execCtx == nil {
		return
	}

	// Partial pipeline: sources only (no metadata)
	sourceAdapters := buildSourceAdapters(githubClient, githubToken, dockerTimeoutMinutes)
	metadataAdapters := []engine.MetadataAdapter{} // Empty!

	if err := executor.Execute(ctx, execCtx, sourceAdapters, metadataAdapters); err != nil {
		log.Printf("Failed to execute pipeline: %v\n", err)
		return
	}

	// Update website URL if found
	if websiteURL != "" {
		_, err := db.Conn.Exec(ctx, "UPDATE categories_software SET website_url = $1 WHERE id = $2", websiteURL, project.ID)
		if err != nil {
			log.Printf("Warning: Failed to update website URL: %v\n", err)
		}
	}

	fmt.Printf("Published project ID %d update completed successfully\n\n", project.ID)
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

	// Create source adapters (use config timeout for analyze mode too)
	dockerTimeout := cfg.WorkMode.DockerTimeoutMinutes
	sourceAdapters := []engine.SourceAdapter{
		sources.NewGitHubSource(githubClient),
		sources.NewLizardSource(dockerTimeout),
		sources.NewScorecardSource(githubToken, dockerTimeout),
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

func retag() {
	ctx := context.Background()

	// Setup credentials
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatalf("GITHUB_TOKEN environment variable is not set")
	}
	githubClient := github.NewClient(nil).WithAuthToken(githubToken)

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

	// Query all published projects
	rows, err := db.Conn.Query(ctx, `
		SELECT id, repository_url
		FROM categories_software
		WHERE state = 'published' AND repository_url LIKE 'https://github.com/%'
		ORDER BY id
	`)
	if err != nil {
		log.Fatalf("Failed to query projects: %v", err)
	}
	defer rows.Close()

	var projects []database.ProjectInfo
	for rows.Next() {
		var p database.ProjectInfo
		if err := rows.Scan(&p.ID, &p.RepositoryURL); err != nil {
			log.Fatalf("Failed to scan project: %v", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating projects: %v", err)
	}

	fmt.Printf("Found %d published projects to retag\n\n", len(projects))

	for i, project := range projects {
		// Parse repository URL
		parsedURL, err := url.Parse(project.RepositoryURL)
		if err != nil {
			log.Printf("[%d/%d] ID %d: Failed to parse URL '%s': %v\n", i+1, len(projects), project.ID, project.RepositoryURL, err)
			continue
		}

		parts := strings.Split(strings.TrimPrefix(parsedURL.Path, "/"), "/")
		if len(parts) < 2 {
			log.Printf("[%d/%d] ID %d: Invalid URL format '%s'\n", i+1, len(projects), project.ID, project.RepositoryURL)
			continue
		}

		owner := parts[0]
		repo := strings.TrimSuffix(parts[1], ".git")

		fmt.Printf("[%d/%d] ID %d: %s/%s\n", i+1, len(projects), project.ID, owner, repo)

		// Generate new tags
		tags, err := metadata.GetTags(ctx, githubClient, db.Conn, owner, repo)
		if err != nil {
			log.Printf("  Error generating tags: %v\n", err)
			continue
		}

		// Start transaction
		tx, err := db.Conn.Begin(ctx)
		if err != nil {
			log.Printf("  Error starting transaction: %v\n", err)
			continue
		}

		// Delete existing tag associations
		_, err = tx.Exec(ctx, "DELETE FROM categories_software_tags WHERE software_id = $1", project.ID)
		if err != nil {
			tx.Rollback(ctx)
			log.Printf("  Error deleting old tags: %v\n", err)
			continue
		}

		// Save new tags
		if err := metadata.SaveTagsToDB(ctx, tx, project.ID, tags); err != nil {
			tx.Rollback(ctx)
			log.Printf("  Error saving tags: %v\n", err)
			continue
		}

		// Commit transaction
		if err := tx.Commit(ctx); err != nil {
			log.Printf("  Error committing: %v\n", err)
			continue
		}

		fmt.Printf("  Tags: %v\n", tags)
	}

	fmt.Printf("\nRetag complete!\n")
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
