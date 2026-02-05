package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/pkg/engine"
)

// GitHubSource fetches metrics from GitHub
type GitHubSource struct {
	client *github.Client
}

// NewGitHubSource creates a new GitHub source adapter
func NewGitHubSource(client *github.Client) *GitHubSource {
	return &GitHubSource{client: client}
}

// Name returns the source name
func (s *GitHubSource) Name() string {
	return "GitHub"
}

// Fetch retrieves all GitHub metrics
func (s *GitHubSource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error) {
	results := make([]engine.MetricResult, 0)

	// 1. Get repository info (stars, language, mirror status)
	var repository *github.Repository
	for {
		var err error
		repository, _, err = s.client.Repositories.Get(ctx, execCtx.Owner, execCtx.Repo)
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get repository info: %w", err)
		}
		break
	}

	if repository.StargazersCount != nil {
		results = append(results, engine.MetricResult{
			Slug:   "stars",
			Value:  float64(*repository.StargazersCount),
			Source: "qsos-lng:github",
		})
	}

	if repository.Language != nil {
		// Language is stored as a string, but we can hash it or use a numeric code
		// For now, we'll just log it and not store as a metric
		log.Printf("  Primary language: %s\n", *repository.Language)
	}

	isMirror := repository.MirrorURL != nil
	results = append(results, engine.MetricResult{
		Slug:   "is_mirror",
		Value:  boolToFloat(isMirror),
		Source: "qsos-lng:github",
	})

	defaultBranch := repository.GetDefaultBranch()

	// 2. Get last commit date
	var lastCommits []*github.RepositoryCommit
	for {
		var err error
		lastCommits, _, err = s.client.Repositories.ListCommits(ctx, execCtx.Owner, execCtx.Repo, &github.CommitsListOptions{
			SHA:         defaultBranch,
			ListOptions: github.ListOptions{PerPage: 1},
		})
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get last commit: %w", err)
		}
		break
	}

	var lastCommitDate time.Time
	if len(lastCommits) > 0 && lastCommits[0].Commit.Committer.Date != nil {
		lastCommitDate = *lastCommits[0].Commit.Committer.Date.GetTime()
	} else {
		return nil, fmt.Errorf("could not find last commit date")
	}

	// Calculate days since last commit
	daysSinceLastCommit := time.Since(lastCommitDate).Hours() / 24
	results = append(results, engine.MetricResult{
		Slug:   "activity_days",
		Value:  daysSinceLastCommit,
		Source: "qsos-lng:github",
	})

	// 3. Get first commit date (by fetching the last page)
	var firstCommitPage int
	for {
		_, resp, err := s.client.Repositories.ListCommits(ctx, execCtx.Owner, execCtx.Repo, &github.CommitsListOptions{
			SHA:         defaultBranch,
			ListOptions: github.ListOptions{PerPage: 1},
		})
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get commit page count: %w", err)
		}
		firstCommitPage = resp.LastPage
		break
	}

	var firstCommits []*github.RepositoryCommit
	for {
		var err error
		firstCommits, _, err = s.client.Repositories.ListCommits(ctx, execCtx.Owner, execCtx.Repo, &github.CommitsListOptions{
			SHA:         defaultBranch,
			ListOptions: github.ListOptions{PerPage: 1, Page: firstCommitPage},
		})
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get first commit: %w", err)
		}
		break
	}

	var firstCommitDate time.Time
	if len(firstCommits) > 0 && firstCommits[0].Commit.Committer.Date != nil {
		firstCommitDate = *firstCommits[0].Commit.Committer.Date.GetTime()
	} else {
		return nil, fmt.Errorf("could not find first commit date")
	}

	// Calculate days since first commit (project maturity)
	daysSinceFirstCommit := time.Since(firstCommitDate).Hours() / 24
	results = append(results, engine.MetricResult{
		Slug:   "maturity_days",
		Value:  daysSinceFirstCommit,
		Source: "qsos-lng:github",
	})

	// 4. Get active contributors (last 6 months, >3 commits, no bots)
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
		commits, resp, err := s.client.Repositories.ListCommits(ctx, execCtx.Owner, execCtx.Repo, opts)
		if err != nil {
			if handleRateLimit(err) {
				continue // Retry the same page after rate limit sleep
			}
			return nil, fmt.Errorf("failed to list commits for contributors: %w", err)
		}

		for _, commit := range commits {
			if commit.Commit.Author.Name != nil && strings.HasSuffix(*commit.Commit.Author.Name, "[bot]") {
				continue
			}
			if commit.Commit.Author.Email != nil {
				uniqueContributors[*commit.Commit.Author.Email]++
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Count contributors with >3 commits
	activeContributors := int64(0)
	for _, commitCount := range uniqueContributors {
		if commitCount > 3 {
			activeContributors++
		}
	}

	results = append(results, engine.MetricResult{
		Slug:   "active_contributors",
		Value:  float64(activeContributors),
		Source: "qsos-lng:github",
	})

	log.Printf("  Stars: %d, Contributors: %d, Days since first commit: %.0f, Days since last commit: %.0f\n",
		int(*repository.StargazersCount), activeContributors, daysSinceFirstCommit, daysSinceLastCommit)

	return results, nil
}

// boolToFloat converts a boolean to a float (1.0 for true, 0.0 for false)
func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}
