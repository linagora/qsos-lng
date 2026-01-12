package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/linagora/qsos-lng/pkg/engine"
)

// ScorecardSource fetches security metrics from OpenSSF Scorecard
type ScorecardSource struct {
	githubToken string
}

// NewScorecardSource creates a new Scorecard source adapter
func NewScorecardSource(githubToken string) *ScorecardSource {
	return &ScorecardSource{githubToken: githubToken}
}

// Name returns the source name
func (s *ScorecardSource) Name() string {
	return "OpenSSF Scorecard"
}

// scorecardResult represents the Scorecard JSON output structure
type scorecardResult struct {
	Checks []struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	} `json:"checks"`
}

// Fetch retrieves all Scorecard metrics
func (s *ScorecardSource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error) {
	cmd := exec.CommandContext(ctx,
		"docker", "run", "--rm", "--net=host",
		"-e", fmt.Sprintf("GITHUB_AUTH_TOKEN=%s", s.githubToken),
		"gcr.io/openssf/scorecard:stable",
		fmt.Sprintf("--repo=https://github.com/%s/%s", execCtx.Owner, execCtx.Repo),
		"--format=json",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run scorecard: %w", err)
	}

	var result scorecardResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse scorecard output: %w", err)
	}

	results := make([]engine.MetricResult, 0)

	// Flatten each check into a separate metric
	for _, check := range result.Checks {
		// Convert check name to slug format (e.g., "Code-Review" -> "scorecard_code_review")
		slug := "scorecard_" + slugify(check.Name)

		// Skip Code-Review check if the repository is a mirror
		if execCtx.IsMirror && check.Name == "Code-Review" {
			log.Printf("  Skipping %s check (repository is a mirror)\n", check.Name)
			continue
		}

		results = append(results, engine.MetricResult{
			Slug:   slug,
			Value:  float64(check.Score),
			Source: "qsos-lng:scorecard",
		})
	}

	log.Printf("  Collected %d Scorecard checks\n", len(results))

	return results, nil
}

// slugify converts a check name to a slug (e.g., "Code-Review" -> "code_review")
func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, "-", "_")
	slug = strings.ReplaceAll(slug, " ", "_")
	return slug
}
