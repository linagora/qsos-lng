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

type Executor struct {
	GitHub         *github.Client
	GitHubToken    string
	SonarqubeURL   *url.URL
	SonarqubeToken string
}

type ProjectStats struct {
	GitHub    *GitHubStats
	Sonar     *SonarStats
	ScoreCard *ScoreCardStats
}

type GitHubStats struct {
	FirstCommitDate    time.Time
	LastCommitDate     time.Time
	Stars              int64
	ActiveContributors int64
}

type SonarStats struct {
	LinesOfCode          int64
	Functions            int64
	CodeSmells           int64
	BrainOverload        int64
	CyclomaticComplexity int64
	CognitiveComplexity  int64
	DuplicationDensity   float64
}

type ScoreCardStats struct {
	Checks []struct {
		Name  string
		Score int64
	}
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
		GitHubToken:    token,
		SonarqubeURL:   u,
		SonarqubeToken: sonarToken,
	}, nil
}

func (e *Executor) GetProjectStats(owner, repo string) (*ProjectStats, error) {
	github, err := e.GetGitHubStats(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("GitHub: %w", err)
	}
	card, err := e.GetScoreCardStats(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("ScoreCard: %w", err)
	}
	sonar, err := e.GetSonarStats(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("Sonar: %w", err)
	}
	return &ProjectStats{
		GitHub:    github,
		ScoreCard: card,
		Sonar:     sonar,
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
		stats.Stars = int64(*repository.StargazersCount)
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
	uniqueContributors := make(map[string]int64)

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
			stats.ActiveContributors++
		}
	}

	return stats, nil
}

func (e *Executor) GetScoreCardStats(owner, repo string) (*ScoreCardStats, error) {
	// TODO make the command configurable
	cmd := exec.Command(
		"docker", "run", "--rm", "--net=host",
		"-e", fmt.Sprintf(`GITHUB_AUTH_TOKEN=%s`, e.GitHubToken),
		"gcr.io/openssf/scorecard:stable",
		fmt.Sprintf(`--repo=https://github.com/%s/%s`, owner, repo),
		"--format=json",
	)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Cannot run scorecard: %w", err)
	}

	var card ScoreCardStats
	if err := json.Unmarshal(output, &card); err != nil {
		return nil, fmt.Errorf("Unexpected output from scorecard: %w", err)
	}
	return &card, nil
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
	skipped := false
	if skip := os.Getenv("SKIP_SONAR_SCANNER"); skip != "" {
		s, err := strconv.ParseBool(skip)
		if err != nil {
			log.Fatalf("Invalid value for SKIP_SONAR_SCANNER")
		}
		skipped = s
	}
	if !skipped {
		if err := e.runSonarScannerCLI(owner, repo); err != nil {
			return nil, err
		}
	}

	// XXX Sonarqube takes some time to build the measures after the scanner
	// has sent its result...
	for i := 0; i < 100; i++ {
		stats, err := e.getSonarStats(owner, repo)
		if err != nil {
			return nil, err
		}
		if stats.LinesOfCode > 0 && stats.BrainOverload > 0 {
			return stats, nil
		}
		log.Printf("measures not yet available in Sonarqube")
		time.Sleep(1 * time.Second)
	}
	return e.getSonarStats(owner, repo)
}

func (e *Executor) runSonarScannerCLI(owner, repo string) error {
	component := owner + "-" + repo
	tmpDir, err := os.MkdirTemp("", component+"-")
	if err != nil {
		return fmt.Errorf("Cannot create a temporary dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd := exec.Command("git", "clone", fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), ".")
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Cannot clone git repository: %w", err)
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
		return fmt.Errorf("Cannot run sonar-scanner-cli: %w", err)
	}
	return nil
}

func (e *Executor) getSonarStats(owner, repo string) (*SonarStats, error) {
	component := owner + "-" + repo
	stats, err := e.getSonarMeasures(component)
	if err != nil {
		return nil, fmt.Errorf("cannot get sonar stats: %w", err)
	}
	if stats.LinesOfCode == 0 {
		return stats, nil
	}
	nb, err := e.getSonarBrainOverloadIssues(component)
	if err != nil {
		return nil, fmt.Errorf("cannot get sonar issues: %w", err)
	}
	stats.BrainOverload = nb
	return stats, nil
}

func (e *Executor) getSonarMeasures(component string) (*SonarStats, error) {
	cloned := *e.SonarqubeURL
	cloned.Path = "/api/measures/component"
	cloned.RawQuery = url.Values{
		"component":  []string{component},
		"metricKeys": []string{"ncloc,functions,code_smells,complexity,cognitive_complexity,duplicated_lines_density"},
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

	stats := &SonarStats{}
	var data SonarMeasuresResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	for _, measure := range data.Component.Measures {
		switch measure.Metric {
		case "ncloc":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid ncloc value: %w", err)
			}
			stats.LinesOfCode = nb
		case "functions":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid functions value: %w", err)
			}
			stats.Functions = nb
		case "code_smells":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid code_smells value: %w", err)
			}
			stats.CodeSmells = nb
		case "complexity":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid complexity value: %w", err)
			}
			stats.CyclomaticComplexity = nb
		case "cognitive_complexity":
			nb, err := strconv.ParseInt(measure.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid cognitive_complexity value: %w", err)
			}
			stats.CognitiveComplexity = nb
		case "duplicated_lines_density":
			nb, err := strconv.ParseFloat(measure.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid duplicated_lines_density value: %w", err)
			}
			stats.DuplicationDensity = nb
		}
	}

	return stats, nil
}

type SonarIssues struct {
	Total int64
}

func (e *Executor) getSonarBrainOverloadIssues(component string) (int64, error) {
	cloned := *e.SonarqubeURL
	cloned.Path = "/api/issues/search"
	cloned.RawQuery = url.Values{
		"components": []string{component},
		"tags":       []string{"brain-overload"},
	}.Encode()
	req, err := http.NewRequest(http.MethodGet, cloned.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("Cannot create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+e.SonarqubeToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Error on request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response: %d", res.StatusCode)
	}
	defer res.Body.Close()

	var data SonarIssues
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("invalid response: %w", err)
	}
	return data.Total, nil
}
