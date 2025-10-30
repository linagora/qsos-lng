package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v76/github"
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

	fmt.Printf("\n--- GitHub Project Statistics ---\n")
	fmt.Printf("Date of the First Commit: %s\n", stats.GitHub.FirstCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Date of the Last Commit:  %s\n", stats.GitHub.LastCommitDate.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Number of Stars:          %d\n", stats.GitHub.Stars)
	fmt.Printf("Active contributors:      %d\n", stats.GitHub.ActiveContributors)
	fmt.Printf("\n--- Sonarqube Statistics ---\n")
	fmt.Printf("Number of lines of code: %d\n", stats.Sonar.LinesOfCode)
}

type Executor struct {
	GitHub         *github.Client
	SonarqubeURL   *url.URL
	SonarqubeToken string
}

type ProjectStats struct {
	GitHub *GitHubStats
	Sonar  *SonarStats
}

type GitHubStats struct {
	FirstCommitDate    time.Time
	LastCommitDate     time.Time
	Stars              int
	ActiveContributors int
}

type SonarStats struct {
	LinesOfCode int
}

func NewExecutorFromEnv() (*Executor, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, errors.New("GITHUB_TOKEN environment variable is not set")
	}
	client := github.NewClient(nil).WithAuthToken(token)

	sonarqube := os.Getenv("SONARQUBE_URL")
	if sonarqube == "" {
		return nil, errors.New("SONARQUBE_URL environment variable is not set")
	}
	u, err := url.Parse(sonarqube)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse SONARQUBE_URL: %w", err)
	}

	sonarToken := os.Getenv("SONARQUBE_TOKEN")
	if sonarToken == "" {
		return nil, errors.New("SONARQUBE_TOKEN environment variable is not set")
	}

	return &Executor{
		GitHub:         client,
		SonarqubeURL:   u,
		SonarqubeToken: sonarToken,
	}, nil
}

func (e *Executor) GetProjectStats(owner, repo string) (*ProjectStats, error) {
	github, err := e.GetGitHubStats(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("GitHub: %w", err)
	}
	sonar, err := e.GetSonarStats(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Sonar: %w", err)
	}
	return &ProjectStats{
		GitHub: github,
		Sonar:  sonar,
	}, nil
}

func (e *Executor) GetGitHubStats(owner, repo string) (*GitHubStats, error) {
	stats := &GitHubStats{}
	ctx := context.Background()

	// 1. Get Project Info (Stars, Default Branch)
	repository, _, err := e.GitHub.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Repositories.Get failed: %w", err)
	}

	if repository.StargazersCount != nil {
		stats.Stars = *repository.StargazersCount
	}
	defaultBranch := *repository.DefaultBranch

	// 2. Get Date of the Last Commit (reverse chronological by default, page 1)
	lastCommit, _, err := e.GitHub.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
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
	_, resp, err := e.GitHub.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         defaultBranch,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, fmt.Errorf("ListCommits for first commit page count failed: %w", err)
	}
	firstCommitPage := resp.LastPage
	firstCommit, _, err := e.GitHub.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
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
		commits, resp, err := e.GitHub.Repositories.ListCommits(ctx, owner, repo, opts)
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

type SonarMeasuresResponse struct {
	Component struct {
		Measures []struct {
			Metric string
			Value  string
		}
	}
}

func (e *Executor) GetSonarStats(owner, repo string) (*SonarStats, error) {
	stats := &SonarStats{}
	component := owner + "-" + repo
	tmpDir, err := os.MkdirTemp("", component+"-")
	if err != nil {
		return nil, fmt.Errorf("Cannot create a temporary dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd := exec.Command("git", "clone", fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), ".")
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Cannot clone git repository: %w", err)
	}

	// TODO make the command configurable
	cmd = exec.Command(
		"docker", "run", "--rm", "--net=host",
		"-e", fmt.Sprintf(`SONAR_HOST_URL=%s`, e.SonarqubeURL),
		"-e", fmt.Sprintf(`SONAR_TOKEN=%s`, e.SonarqubeToken),
		"-v", fmt.Sprintf(`%s:/usr/src`, tmpDir),
		"sonarsource/sonar-scanner-cli",
		fmt.Sprintf(`-Dsonar.projectKey=%s`, component),
		"-Dsonar.sources=.",
	)
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Cannot run sonar-scanner-cli: %w", err)
	}

	cloned := *e.SonarqubeURL
	cloned.Path = "/api/measures/component"
	cloned.RawQuery = url.Values{
		"component":  []string{component},
		"metricKeys": []string{"ncloc"},
	}.Encode()
	req, err := http.NewRequest(http.MethodGet, cloned.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Cannot create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+e.SonarqubeToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error on request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response: %d", res.StatusCode)
	}
	defer res.Body.Close()

	var data SonarMeasuresResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	for _, measure := range data.Component.Measures {
		if measure.Metric == "ncloc" {
			loc, err := strconv.Atoi(measure.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid ncloc value: %w", err)
			}
			stats.LinesOfCode = loc
		}
	}

	return stats, nil
}
