package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/common"
	"github.com/linagora/qsos-lng/community"
	"github.com/linagora/qsos-lng/security"
	"github.com/linagora/qsos-lng/tech"
	"github.com/otiai10/openaigo"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: go run . <owner/repo>")
	}

	parts := strings.Split(os.Args[1], "/")
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

	sonarqubeURL := os.Getenv("SONARQUBE_URL")
	if sonarqubeURL == "" {
		log.Fatalf("SONARQUBE_URL environment variable is not set")
	}

	sonarToken := os.Getenv("SONARQUBE_TOKEN")
	if sonarToken == "" {
		log.Fatalf("SONARQUBE_TOKEN environment variable is not set")
	}

	// Fetch data from each category
	ctx := context.Background()

	communityData, err := community.Fetch(ctx, githubClient, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch community data: %v", err)
	}

	techData, err := tech.Fetch(owner, repo, sonarqubeURL, sonarToken)
	if err != nil {
		log.Fatalf("Failed to fetch tech data: %v", err)
	}

	securityData, err := security.Fetch(owner, repo, githubToken)
	if err != nil {
		log.Fatalf("Failed to fetch security data: %v", err)
	}

	summary, err := getSummary(ctx, githubClient, owner, repo)
	if err != nil {
		log.Fatalf("Failed to get summary: %v", err)
	}

	// Setup thresholds
	day := (24 * 60 * 60 * time.Second).Nanoseconds()
	month := 30 * day
	year := 365 * day

	communityThresholds := &community.CommunityThresholds{
		Maturity:     [4]int64{1 * year, 5 * year, 10 * year, 20 * year},
		Activity:     [4]int64{1 * month, 6 * month, 1 * year, 2 * year},
		Popularity:   [4]int64{5_000, 20_000, 40_000, 80_000},
		Contributors: [4]int64{1, 5, 20, 50},
	}

	techThresholds := &tech.TechThresholds{
		Size:                 [4]int64{1_000, 10_000, 100_000, 1_000_000},
		CyclomaticComplexity: [4]int64{1, 5, 10, 20},
		CognitiveComplexity:  [4]int64{1, 3, 5, 10},
		Duplication:          [4]int64{3, 5, 10, 20},
		CodeSmells:           [4]int64{50, 200, 500, 1_000},
	}

	securityWeights := map[string]int64{
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
	fmt.Printf("\n--- GitHub Project Statistics ---\n")
	fmt.Printf("Date of the First Commit: %s\n", communityData.FirstCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Date of the Last Commit:  %s\n", communityData.LastCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Number of Stars:          %d\n", communityData.Stars)
	fmt.Printf("Active contributors:      %d\n", communityData.ActiveContributors)
	fmt.Printf("\n--- Sonarqube Statistics ---\n")
	fmt.Printf("Number of lines of code: %d\n", techData.LinesOfCode)
	fmt.Printf("Number of functions:     %d\n", techData.Functions)
	fmt.Printf("Cyclomatic complexity:   %d\n", techData.CyclomaticComplexity)
	fmt.Printf("Cognitive complexity:    %d\n", techData.CognitiveComplexity)
	fmt.Printf("Brain-overload issues:   %d\n", techData.BrainOverload)
	fmt.Printf("Number of code smells:   %d\n", techData.CodeSmells)
	fmt.Printf("Duplication density:     %.1f\n", techData.DuplicationDensity)
	fmt.Printf("\n--- ScoreCard checks ---\n")
	for _, check := range securityData.Checks {
		fmt.Printf("%-24s: %d\n", check.Name, check.Score)
	}

	fmt.Printf("\n--- Community ---\n")
	fmt.Printf("Maturity:     %d\n", scores.Community.Maturity)
	fmt.Printf("Activity:     %d\n", scores.Community.Activity)
	fmt.Printf("Popularity:   %d\n", scores.Community.Popularity)
	fmt.Printf("Contributors: %d\n", scores.Community.Contributors)
	fmt.Printf("\n--- Tech ---\n")
	fmt.Printf("Code size:             %d\n", scores.Tech.Size)
	fmt.Printf("Cyclomatic complexity: %d\n", scores.Tech.CyclomaticComplexity)
	fmt.Printf("Cognitive complexity:  %d\n", scores.Tech.CognitiveComplexity)
	fmt.Printf("Duplication:           %d\n", scores.Tech.Duplication)
	fmt.Printf("Code smells:           %d\n", scores.Tech.CodeSmells)
	fmt.Printf("\n--- Security ---\n")
	fmt.Printf("Scorecard: %d\n", scores.Security.ScoreCard)

	fmt.Printf("\n--- Summary ---\n%s\n", summary)
}

// getSummary fetches and summarizes the project README using AI
func getSummary(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		return "", err
	}

	aiClient := openaigo.NewClient(os.Getenv("AI_API_KEY"))
	if u := os.Getenv("AI_BASE_URL"); u != "" {
		aiClient.BaseURL = u
	}

	summary, err := summarize(ctx, aiClient, content)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return summary, nil
}

const promptTLDR = `
Tu es un agent dont le rôle est de créer une introduction en français pour un
logiciel Open-Source. Cette introduction devra faire 4 ou 5 phrases. Voici le
README du logiciel en question.
`

func summarize(ctx context.Context, client *openaigo.Client, content string) (string, error) {
	if client.APIKey == "" {
		return "", errors.New("AI_API_KEY not set")
	}

	model := "gpt-oss-120b"
	if m := os.Getenv("AI_MODEL"); m != "" {
		model = m
	}
	request := openaigo.ChatRequest{
		Model: model,
		Messages: []openaigo.Message{
			{Role: "system", Content: promptTLDR},
			{Role: "user", Content: content},
		},
	}
	response, err := client.Chat(ctx, request)
	if err != nil {
		return "", fmt.Errorf("AI error: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("AI error: no response")
	}
	return response.Choices[0].Message.Content, nil
}
