package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
)

type ProjectStats struct {
	FirstCommitDate    time.Time
	LastCommitDate     time.Time
	Stars              int
	ActiveContributors int
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: go run . <owner/repo>")
	}

	parts := strings.Split(os.Args[1], "/")
	if len(parts) != 2 {
		log.Fatalf("Invalid project format. Must be in the format: owner/repo")
	}
	owner, repo := parts[0], parts[1]

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatalf("Error: GITHUB_TOKEN environment variable is not set")
	}
	client := github.NewClient(nil).WithAuthToken(token)

	stats, err := getProjectStats(client, owner, repo)
	if err != nil {
		log.Fatalf("Failed to retrieve repository statistics: %v", err)
	}

	fmt.Printf("\n--- GitHub Project Statistics: %s/%s ---\n", owner, repo)
	fmt.Printf("Date of the First Commit: %s\n", stats.FirstCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Date of the Last Commit:  %s\n", stats.LastCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Number of Stars:          %d\n", stats.Stars)
	fmt.Printf("Active contributors:      %d\n", stats.ActiveContributors)
}

func getProjectStats(client *github.Client, owner, repo string) (*ProjectStats, error) {
	stats := &ProjectStats{}
	ctx := context.Background()

	// 1. Get Project Info (Stars, Default Branch)
	repository, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Repositories.Get failed: %w", err)
	}

	if repository.StargazersCount != nil {
		stats.Stars = *repository.StargazersCount
	}
	defaultBranch := *repository.DefaultBranch

	// 2. Get Date of the Last Commit (reverse chronological by default, page 1)
	lastCommit, _, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         defaultBranch,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, fmt.Errorf("ListCommits for last commit failed: %w", err)
	}
	if len(lastCommit) > 0 && lastCommit[0].Commit.Committer.Date != nil {
		stats.LastCommitDate = *lastCommit[0].Commit.Committer.Date.GetTime()
	} else {
		return nil, fmt.Errorf("could not find last commit date")
	}

	// 3. Get Date of the First Commit (by fetching the last page of commits)
	_, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         defaultBranch,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, fmt.Errorf("ListCommits for first commit page count failed: %w", err)
	}
	firstCommitPage := resp.LastPage
	firstCommit, _, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         defaultBranch,
		ListOptions: github.ListOptions{PerPage: 1, Page: firstCommitPage},
	})
	if err != nil {
		return nil, fmt.Errorf("ListCommits for first commit failed: %w", err)
	}
	if len(firstCommit) > 0 && firstCommit[0].Commit.Committer.Date != nil {
		stats.FirstCommitDate = *firstCommit[0].Commit.Committer.Date.GetTime()
	} else {
		return nil, fmt.Errorf("could not find first commit date")
	}

	// 4. Get Number of Contributors in the last 6 months, with at least 5 commits
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	uniqueContributors := make(map[string]int)

	opts := &github.CommitsListOptions{
		Since: sixMonthsAgo,
		SHA:   defaultBranch,
		ListOptions: github.ListOptions{
			PerPage: 1000,
		},
	}
	for {
		commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("ListCommits for contributors failed: %w", err)
		}
		for _, commit := range commits {
			uniqueContributors[*commit.Commit.Author.Email] += 1
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, nbCommits := range uniqueContributors {
		if nbCommits > 5 {
			stats.ActiveContributors++
		}
	}

	return stats, nil
}
