package community

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
)

// Fetch retrieves all community-related data from GitHub
func Fetch(ctx context.Context, client *github.Client, owner, repo string) (*CommunityData, error) {
	data := &CommunityData{}

	// 1. Get Project Info (Stars, Default Branch)
	repository, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Repositories.Get failed: %w", err)
	}

	if repository.StargazersCount != nil {
		data.Stars = int64(*repository.StargazersCount)
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
		data.LastCommitDate = *lastCommit[0].Commit.Committer.Date.GetTime()
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
		data.FirstCommitDate = *firstCommit[0].Commit.Committer.Date.GetTime()
	} else {
		return nil, fmt.Errorf("could not find first commit date")
	}

	// 4. Get Number of Contributors in the last 6 months, with at least 3 commits
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	uniqueContributors := make(map[string]int64)

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
			if strings.HasSuffix(*commit.Commit.Author.Name, "[bot]") {
				continue
			}
			uniqueContributors[*commit.Commit.Author.Email] += 1
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	for _, nbCommits := range uniqueContributors {
		if nbCommits > 3 {
			data.ActiveContributors++
		}
	}

	// 5. Fetch documentation data
	docData, err := FetchDocumentation(ctx, client, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("FetchDocumentation failed: %w", err)
	}
	data.Documentation = docData

	return data, nil
}
