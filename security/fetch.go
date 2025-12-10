package security

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

// Fetch retrieves all security-related data from OpenSSF Scorecard
func Fetch(owner, repo, githubToken string) (*SecurityData, error) {
	// TODO make the command configurable
	cmd := exec.Command(
		"docker", "run", "--rm", "--net=host",
		"-e", fmt.Sprintf(`GITHUB_AUTH_TOKEN=%s`, githubToken),
		"gcr.io/openssf/scorecard:stable",
		fmt.Sprintf(`--repo=https://github.com/%s/%s`, owner, repo),
		"--format=json",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Cannot run scorecard: %w", err)
	}

	var data SecurityData
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("Unexpected output from scorecard: %w", err)
	}

	log.Printf("\n--- ScoreCard checks ---\n")
	for _, check := range data.Checks {
		log.Printf("%-24s: %d\n", check.Name, check.Score)
	}

	return &data, nil
}
