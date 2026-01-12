package sources

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/linagora/qsos-lng/pkg/engine"
)

// LizardSource fetches metrics using Lizard code analysis
type LizardSource struct{}

// NewLizardSource creates a new Lizard source adapter
func NewLizardSource() *LizardSource {
	return &LizardSource{}
}

// Name returns the source name
func (s *LizardSource) Name() string {
	return "Lizard"
}

// Fetch retrieves all Lizard metrics
func (s *LizardSource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error) {
	component := execCtx.Owner + "-" + execCtx.Repo
	tmpDir, err := os.MkdirTemp("", component+"-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone repository
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1",
		fmt.Sprintf("https://github.com/%s/%s.git", execCtx.Owner, execCtx.Repo), ".")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Run Lizard analysis
	cmd = exec.CommandContext(ctx,
		"docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", "HOME=/tmp",
		"-v", fmt.Sprintf("%s:/src", tmpDir),
		"python:3.13-alpine",
		"sh", "-c", "pip install lizard --quiet && python -m lizard --csv /src",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Lizard stderr: %s", stderr.String())
		return nil, fmt.Errorf("failed to run lizard: %w", err)
	}

	// Parse CSV output
	reader := csv.NewReader(bytes.NewReader(stdout.Bytes()))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) < 1 {
		return nil, fmt.Errorf("no code found by Lizard")
	}

	var totalNLOC, totalFunctions, highComplexityCount int64
	var testNLOC, testFunctions int64

	// Parse each function row
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 7 {
			continue
		}

		nloc, err := strconv.ParseInt(strings.TrimSpace(record[0]), 10, 64)
		if err != nil {
			continue
		}

		ccn, err := strconv.ParseInt(strings.TrimSpace(record[1]), 10, 64)
		if err != nil {
			continue
		}

		filepath := strings.TrimSpace(record[6])
		isTest := isTestFile(filepath)

		if isTest {
			testNLOC += nloc
			testFunctions++
		} else {
			totalNLOC += nloc
			totalFunctions++
			if ccn > 15 {
				highComplexityCount++
			}
		}
	}

	// Calculate derived metrics
	var testRatio float64
	if totalNLOC > 0 {
		testRatio = float64(testNLOC) / float64(totalNLOC)
	}

	var complexityPercentage float64
	if totalFunctions > 0 {
		complexityPercentage = float64(highComplexityCount) * 100.0 / float64(totalFunctions)
	}

	results := []engine.MetricResult{
		{Slug: "lines_of_code", Value: float64(totalNLOC), Source: "qsos-lng:lizard"},
		{Slug: "functions", Value: float64(totalFunctions), Source: "qsos-lng:lizard"},
		{Slug: "high_complexity_functions", Value: float64(highComplexityCount), Source: "qsos-lng:lizard"},
		{Slug: "test_lines_of_code", Value: float64(testNLOC), Source: "qsos-lng:lizard"},
		{Slug: "test_ratio", Value: testRatio, Source: "qsos-lng:lizard"},
		{Slug: "complexity", Value: complexityPercentage, Source: "qsos-lng:lizard"},
	}

	log.Printf("  LOC: %d, Functions: %d, High-complexity: %d (%.1f%%), Test ratio: %.2f\n",
		totalNLOC, totalFunctions, highComplexityCount, complexityPercentage, testRatio)

	return results, nil
}

// isTestFile checks if a file path represents a test file
func isTestFile(filepath string) bool {
	filepath = strings.ReplaceAll(filepath, "\\", "/")
	filepath = strings.ToLower(filepath)

	// Common test patterns
	if strings.Contains(filepath, "_test") || strings.Contains(filepath, "test_") {
		return true
	}
	if strings.Contains(filepath, ".test.") || strings.Contains(filepath, ".spec.") {
		return true
	}

	// Test directories
	testDirs := []string{"/test/", "/tests/", "/__tests__/", "/t/", "specs", "src/test/"}
	for _, dir := range testDirs {
		if strings.Contains(filepath, dir) {
			return true
		}
	}

	return false
}
