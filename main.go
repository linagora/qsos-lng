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
	"github.com/linagora/qsos-lng/common"
	"github.com/linagora/qsos-lng/community"
	"github.com/linagora/qsos-lng/metadata"
	"github.com/linagora/qsos-lng/security"
	"github.com/linagora/qsos-lng/tech"
)

var (
	// Setup thresholds
	day   = (24 * 60 * 60 * time.Second).Nanoseconds()
	month = 30 * day
	year  = 365 * day

	communityThresholds = &community.CommunityThresholds{
		Maturity:      [4]int64{1 * year, 5 * year, 10 * year, 20 * year},
		Activity:      [4]int64{1 * month, 6 * month, 1 * year, 2 * year},
		Popularity:    [4]int64{5_000, 20_000, 40_000, 80_000},
		Contributors:  [4]int64{1, 5, 20, 50},
		Documentation: [4]int64{20, 40, 60, 80}, // Percentage score: 20%=poor, 40%=partial, 60%=good, 80%=excellent
	}

	techThresholds = &tech.TechThresholds{
		Size:       [4]int64{1_000, 10_000, 100_000, 1_000_000},
		Complexity: [4]int64{5, 10, 20, 30}, // Percentage of high-CCN functions (>15)
	}

	securityWeights = map[string]int64{
		// https://scorecard.dev/#the-checks
		// 1 for low upto 4 for critical
		"Vulnerabilities":        2, // Only known vulnerabilities, so it may give better scores for less known projects
		"Dependency-Update-Tool": 3,
		// "Maintained" is disabled, as it's already in the community section
		"Security-Policy": 2,
		// "License" is disabled, as it's not for the security section
		// "CII-Best-Practices" is disabled, as it's not relevant for us
		// "CI-Tests" is disabled, as it's for more for the tech section
		"Fuzzing":            1, // Only some tools are detected
		"SAST":               1, // Only some tools are detected
		"Binary-Artifacts":   3,
		"Branch-Protection":  3,
		"Dangerous-Workflow": 4,
		"Code-Review":        3,
		// "Contributors" is disabled, as it's already in the community section
		"Pinned-Dependencies": 2,
		"Token-Permissions":   3,
		"Packaging":           4, // Increased, as it's really important for us
		"Signed-Releases":     4, // Increased, as it's really important for us
	}
)

func usage() {
	log.Fatalf(`Usage:
- go run analyze . <owner/repo> for one-shot analysis of a project
- go run work for working in background for l'Argus du Libre.
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

	// Connect to the database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatalf("DATABASE_URL environment variable is not set")
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	// Load fields from database once
	rows, err := conn.Query(ctx, "SELECT id, slug FROM categories_field")
	if err != nil {
		log.Fatalf("Failed to load fields: %v", err)
	}
	fieldMap := make(map[string]int)
	for rows.Next() {
		var fieldID int
		var slug string
		if err := rows.Scan(&fieldID, &slug); err != nil {
			log.Fatalf("Failed to scan field: %v", err)
		}
		fieldMap[slug] = fieldID
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating fields: %v", err)
	}

	for {
		// Query for a draft project
		var projectID int
		var repositoryURL string
		err = conn.QueryRow(ctx, `
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

		scores, err := process(repositoryURL, githubClient, githubToken)
		if err != nil {
			log.Printf("Failed to process '%s': %v", repositoryURL, err)
			continue
		}

		// Parse repository URL to extract owner and repo
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

		// Get repository info for language and homepage
		repository, _, err := githubClient.Repositories.Get(ctx, owner, repo)
		if err != nil {
			log.Printf("Warning: Failed to fetch repository info: %v", err)
		}
		language := ""
		websiteURL := ""
		if repository != nil {
			if repository.Language != nil {
				language = *repository.Language
			}
			if repository.Homepage != nil && *repository.Homepage != "" {
				websiteURL = *repository.Homepage
				fmt.Printf("Found website URL: %s\n", websiteURL)
			}
		}

		// Get icon URL
		iconURL := ""
		iconURLResult, err := metadata.GetIconURL(ctx, githubClient, owner, repo, language)
		if err != nil {
			log.Printf("Warning: Failed to get icon URL: %v", err)
		} else {
			iconURL = iconURLResult
		}
		if iconURL != "" {
			fmt.Printf("Found icon URL: %s\n", iconURL)
		}

		// Generate bilingual summaries
		fmt.Printf("Generating project summaries...\n")
		summaries, err := metadata.GetBilingualSummary(ctx, githubClient, owner, repo)
		if err != nil {
			log.Printf("Warning: Failed to generate summaries: %v", err)
			// Continue without summaries rather than failing the entire analysis
		}

		// Generate tags
		fmt.Printf("Generating project tags...\n")
		tags, err := metadata.GetTags(ctx, githubClient, conn, owner, repo)
		if err != nil {
			log.Printf("Warning: Failed to generate tags: %v", err)
			// Continue without tags rather than failing the entire analysis
		} else {
			fmt.Printf("Generated tags: %v\n", tags)
		}

		// Map scores to field slugs and database score format (1.00-5.00)
		// Scores are 0-4, so we add 1 to get 1-5
		scoreResults := map[string]float64{
			"maturity":      float64(scores.Community.Maturity),
			"activity":      float64(scores.Community.Activity),
			"popularity":    float64(scores.Community.Popularity),
			"contributors":  float64(scores.Community.Contributors),
			"documentation": float64(scores.Community.Documentation),
			"size":          float64(scores.Tech.Size),
			"complexity":    float64(scores.Tech.Complexity),
			"scorecard":     float64(scores.Security.ScoreCard),
		}

		// Log computed scores
		fmt.Printf("Computed scores:\n")
		fmt.Printf("  Community:\n")
		fmt.Printf("    - maturity:      %.0f\n", scoreResults["maturity"])
		fmt.Printf("    - activity:      %.0f\n", scoreResults["activity"])
		fmt.Printf("    - popularity:    %.0f\n", scoreResults["popularity"])
		fmt.Printf("    - contributors:  %.0f\n", scoreResults["contributors"])
		fmt.Printf("    - documentation: %.0f\n", scoreResults["documentation"])
		fmt.Printf("  Tech:\n")
		fmt.Printf("    - size:          %.0f\n", scoreResults["size"])
		fmt.Printf("    - complexity:    %.0f\n", scoreResults["complexity"])
		fmt.Printf("  Security:\n")
		fmt.Printf("    - scorecard:     %.0f\n", scoreResults["scorecard"])

		tx, err := conn.Begin(ctx)
		if err != nil {
			log.Printf("Failed to begin transaction: %v", err)
			continue
		}

		// Insert analysis results
		saveSuccess := true
		for slug, score := range scoreResults {
			fieldID, exists := fieldMap[slug]
			if !exists {
				log.Printf("Warning: Field with slug '%s' not found in database, skipping", slug)
				continue
			}

			_, err := tx.Exec(ctx, `
				INSERT INTO categories_analysisresult (software_id, field_id, score, is_published, is_manual, created_at)
				VALUES ($1, $2, $3, true, false, NOW())
			`, projectID, fieldID, score)

			if err != nil {
				log.Printf("Failed to insert analysis result for field '%s': %v", slug, err)
				saveSuccess = false
				break
			}
		}

		if !saveSuccess {
			tx.Rollback(ctx)
			log.Printf("Transaction rolled back due to errors\n")
			continue
		}

		// Save summaries to database if they were generated
		if summaries != nil {
			err = metadata.SaveSummariesToDB(ctx, tx, int64(projectID), summaries)
			if err != nil {
				log.Printf("Warning: Failed to save summaries to database: %v", err)
				// Continue without failing the transaction for summaries
			} else {
				fmt.Printf("Summaries saved to database\n")
			}
		}

		// Save tags to database if they were generated
		if len(tags) > 0 {
			err = metadata.SaveTagsToDB(ctx, tx, int64(projectID), tags)
			if err != nil {
				log.Printf("Warning: Failed to save tags to database: %v", err)
				// Continue without failing the transaction for tags
			} else {
				fmt.Printf("Tags saved to database\n")
			}
		}

		// Update software state to 'in_review', save icon URL and website URL
		_, err = tx.Exec(ctx, `
			UPDATE categories_software
			SET state = 'in_review', logo_url = $2, website_url = $3
			WHERE id = $1
		`, projectID, iconURL, websiteURL)

		if err != nil {
			log.Printf("Failed to update software state: %v", err)
			tx.Rollback(ctx)
			continue
		}

		err = tx.Commit(ctx)
		if err != nil {
			log.Printf("Failed to commit transaction: %v", err)
			continue
		}

		fmt.Printf("Project ID %d analysis completed and saved to database\n\n", projectID)
	}
}

func process(repositoryURL string, githubClient *github.Client, githubToken string) (*common.ProjectScores, error) {
	ctx := context.Background()

	parsedURL, err := url.Parse(repositoryURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse repositoryURL: %v", err)
	}
	if parsedURL.Host != "github.com" {
		return nil, fmt.Errorf("only github.com projects are supported")
	}

	path := strings.Trim(parsedURL.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GitHub repository path")
	}
	owner, repo := parts[0], parts[1]

	fmt.Printf("Processing project %s/%s\n", owner, repo)

	communityData, err := community.Fetch(ctx, githubClient, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch community data: %v", err)
	}
	techData, err := tech.Fetch(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch tech data: %v", err)
	}
	securityData, err := security.Fetch(owner, repo, githubToken)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch security data: %v", err)
	}

	communityScores := community.ComputeAll(communityData, communityThresholds)
	techScores := tech.ComputeAll(techData, techThresholds)
	securityScores := security.ComputeAll(securityData, securityWeights)
	return &common.ProjectScores{
		Community: communityScores,
		Tech:      techScores,
		Security:  securityScores,
	}, nil
}

func analyze(project string) {
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

	// Fetch data from each category
	ctx := context.Background()

	// Get repository info for language
	repository, _, err := githubClient.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch repository info: %v", err)
	}
	language := ""
	if repository.Language != nil {
		language = *repository.Language
	}

	// Get icon URL
	iconURL, err := metadata.GetIconURL(ctx, githubClient, owner, repo, language)
	if err != nil {
		log.Printf("Warning: Failed to get icon URL: %v", err)
	}

	communityData, err := community.Fetch(ctx, githubClient, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch community data: %v", err)
	}
	techData, err := tech.Fetch(owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch tech data: %v", err)
	}
	securityData, err := security.Fetch(owner, repo, githubToken)
	if err != nil {
		log.Fatalf("Failed to fetch security data: %v", err)
	}
	summary, err := metadata.GetSummary(ctx, githubClient, owner, repo)
	if err != nil {
		log.Fatalf("Failed to get summary: %v", err)
	}

	// Compute scores
	communityScores := community.ComputeAll(communityData, communityThresholds)
	techScores := tech.ComputeAll(techData, techThresholds)
	securityScores := security.ComputeAll(securityData, securityWeights)

	scores := &common.ProjectScores{
		Community: communityScores,
		Tech:      techScores,
		Security:  securityScores,
	}

	// Display results
	fmt.Printf("\n=== %s ===\n", project)
	fmt.Printf("\n--- Community ---\n")
	fmt.Printf("Maturity:      %d\n", scores.Community.Maturity)
	fmt.Printf("Activity:      %d\n", scores.Community.Activity)
	fmt.Printf("Popularity:    %d\n", scores.Community.Popularity)
	fmt.Printf("Contributors:  %d\n", scores.Community.Contributors)
	fmt.Printf("Documentation: %d\n", scores.Community.Documentation)
	fmt.Printf("\n--- Tech ---\n")
	fmt.Printf("Code size:  %d\n", scores.Tech.Size)
	fmt.Printf("Complexity: %d\n", scores.Tech.Complexity)
	fmt.Printf("\n--- Security ---\n")
	fmt.Printf("Scorecard: %d\n", scores.Security.ScoreCard)

	fmt.Printf("\n--- Summary ---\n%s\n", summary)
	if iconURL != "" {
		fmt.Printf("Icon URL: %s\n", iconURL)
	}
}
