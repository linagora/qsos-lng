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

	// Create execution engine
	executor := engine.NewExecutor(cfg, db, lookup, fieldMap, githubClient, githubToken)

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

	// Create temporary execution context (no database, software_id = 0)
	// Note: This won't work with the full pipeline since scoring requires database
	// For analyze mode, we'll just fetch and display raw metrics

	fmt.Printf("\n--- Fetching Metrics ---\n")

	// GitHub metrics
	githubSource := sources.NewGitHubSource(githubClient)
	execCtx := &engine.ExecutionContext{
		SoftwareID:    0,
		Owner:         owner,
		Repo:          repo,
		RepositoryURL: fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		IsMirror:      isMirror,
		Language:      language,
	}

	fmt.Printf("\nGitHub Metrics:\n")
	githubMetrics, err := githubSource.Fetch(ctx, execCtx)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	} else {
		for _, m := range githubMetrics {
			fmt.Printf("  %s = %.2f\n", m.Slug, m.Value)
		}
	}

	// Lizard metrics
	lizardSource := sources.NewLizardSource()
	fmt.Printf("\nLizard Metrics:\n")
	lizardMetrics, err := lizardSource.Fetch(ctx, execCtx)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	} else {
		for _, m := range lizardMetrics {
			fmt.Printf("  %s = %.2f\n", m.Slug, m.Value)
		}
	}

	// Scorecard metrics
	scorecardSource := sources.NewScorecardSource(githubToken)
	fmt.Printf("\nScorecard Metrics:\n")
	scorecardMetrics, err := scorecardSource.Fetch(ctx, execCtx)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	} else {
		for _, m := range scorecardMetrics {
			fmt.Printf("  %s = %.2f\n", m.Slug, m.Value)
		}
	}

	// Documentation metrics
	docSource := sources.NewDocumentationSource(githubClient)
	fmt.Printf("\nDocumentation Metrics:\n")
	docMetrics, err := docSource.Fetch(ctx, execCtx)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	} else {
		for _, m := range docMetrics {
			fmt.Printf("  %s = %.2f\n", m.Slug, m.Value)
		}
	}

	fmt.Printf("\nNote: Scoring requires database connection. Use 'work' mode for full analysis with scoring.\n")
}
