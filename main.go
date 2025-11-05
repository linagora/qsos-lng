package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
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

	executor, err := NewExecutorFromEnv()
	if err != nil {
		log.Fatalf("ERROR: %s", err)
	}

	stats, err := executor.GetProjectStats(owner, repo)
	if err != nil {
		log.Fatalf("Failed to retrieve repository statistics: %v", err)
	}

	day := (24 * 60 * 60 * time.Second).Nanoseconds()
	month := 30 * day
	year := 365 * day
	thresholds := &Thresholds{
		Community: &CommunityThreshold{
			Maturity:     [4]int64{1 * year, 5 * year, 10 * year, 20 * year},
			Activity:     [4]int64{1 * month, 6 * month, 1 * year, 2 * year},
			Popularity:   [4]int64{5_000, 20_000, 40_000, 80_000},
			Contributors: [4]int64{1, 5, 20, 50},
		},
		Tech: &TechThreshold{
			Size:                 [4]int64{1_000, 10_000, 100_000, 1_000_000},
			CyclomaticComplexity: [4]int64{10, 20, 30, 50},
			CognitiveComplexity:  [4]int64{1, 3, 5, 10},
			Duplication:          [4]int64{3, 5, 10, 20},
			CodeSmells:           [4]int64{50, 200, 500, 1_000},
		},
	}

	fmt.Printf("\n--- GitHub Project Statistics ---\n")
	fmt.Printf("Date of the First Commit: %s\n", stats.GitHub.FirstCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Date of the Last Commit:  %s\n", stats.GitHub.LastCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Number of Stars:          %d\n", stats.GitHub.Stars)
	fmt.Printf("Active contributors:      %d\n", stats.GitHub.ActiveContributors)
	fmt.Printf("\n--- Sonarqube Statistics ---\n")
	fmt.Printf("Number of lines of code: %d\n", stats.Sonar.LinesOfCode)
	fmt.Printf("Number of functions:     %d\n", stats.Sonar.Functions)
	fmt.Printf("Cyclomatic complexity:   %d\n", stats.Sonar.CyclomaticComplexity)
	fmt.Printf("Cognitive complexity:    %d\n", stats.Sonar.CognitiveComplexity)
	fmt.Printf("Number of code smells:   %d\n", stats.Sonar.CodeSmells)
	fmt.Printf("Duplication density:     %.1f\n", stats.Sonar.DuplicationDensity)

	scores := ComputeScores(stats, thresholds)
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
}
