package tech

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Fetch retrieves all tech-related data using Lizard
func Fetch(owner, repo string) (*TechData, error) {
	component := owner + "-" + repo
	tmpDir, err := os.MkdirTemp("", component+"-")
	if err != nil {
		return nil, fmt.Errorf("Cannot create a temporary dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone the repository
	cmd := exec.Command("git", "clone", "--depth=1",
		fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), ".")
	cmd.Dir = tmpDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Cannot clone git repository: %w", err)
	}

	// Run Lizard analysis via Docker
	cmd = exec.Command(
		"docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", "HOME=/tmp",
		"-v", fmt.Sprintf(`%s:/src`, tmpDir),
		"python:3.13-alpine",
		"sh", "-c", "pip install lizard --quiet && python -m lizard --csv /src",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Lizard stderr: %s", stderr.String())
		return nil, fmt.Errorf("Cannot run lizard: %w", err)
	}

	// Parse CSV output
	data, err := parseLizardOutput(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("Cannot parse lizard output: %w", err)
	}

	log.Printf("\n--- Lizard Statistics ---\n")
	log.Printf("Production code:\n")
	log.Printf("  Lines of code:                %d\n", data.LinesOfCode)
	log.Printf("  Number of functions:          %d\n", data.Functions)
	log.Printf("  High-complexity functions (>15): %d\n", data.HighComplexityFunctions)
	if data.Functions > 0 {
		log.Printf("  Percentage high-complexity:   %d%%\n", 100*data.HighComplexityFunctions/data.Functions)
	}
	log.Printf("Test code:\n")
	log.Printf("  Lines of code:                %d\n", data.TestLinesOfCode)
	log.Printf("  Number of functions:          %d\n", data.TestFunctions)
	log.Printf("  Test/production ratio:        %.2f\n", data.TestRatio)

	return data, nil
}

// isTestFile checks if a file path represents a test file
// based on common test location patterns across different languages
func isTestFile(filepath string) bool {
	// Normalize path separators
	filepath = strings.ReplaceAll(filepath, "\\", "/")
	filepath = strings.ToLower(filepath)

	// Go: *_test.go
	// Python: test_*.py, *_test.py, or tests/ directory
	if strings.Contains(filepath, "_test") || strings.Contains(filepath, "test_") {
		return true
	}

	// JavaScript/TypeScript: .test.js, .spec.js, .test.ts, .spec.ts
	if strings.Contains(filepath, ".test.") || strings.Contains(filepath, ".spec.") {
		return true
	}

	// Check for test directories (most languages)
	testDirs := []string{"/test/", "/tests/", "/__tests__/", "/t/", "specs"}
	for _, dir := range testDirs {
		if strings.Contains(filepath, dir) {
			return true
		}
	}

	// Java/Kotlin: src/test/
	if strings.Contains(filepath, "src/test/") {
		return true
	}

	return false
}

func parseLizardOutput(csvData []byte) (*TechData, error) {
	reader := csv.NewReader(bytes.NewReader(csvData))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot read CSV: %w", err)
	}

	if len(records) < 1 {
		return nil, fmt.Errorf("no code found by Lizard")
	}

	var totalNLOC, totalFunctions, highComplexityCount int64
	var testNLOC, testFunctions int64

	// Parse each function row (Lizard CSV has no header, start from row 0)
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 7 {
			log.Printf("Warning: skipping malformed CSV row %d with %d columns (need at least 7)", i, len(record))
			continue
		}

		// Column 0: NLOC (non-comment lines of code)
		nloc, err := strconv.ParseInt(strings.TrimSpace(record[0]), 10, 64)
		if err != nil {
			log.Printf("Warning: skipping row %d, column 0 (%q) is not a number: %v", i, record[0], err)
			continue
		}

		// Column 1: CCN (cyclomatic complexity number)
		ccn, err := strconv.ParseInt(strings.TrimSpace(record[1]), 10, 64)
		if err != nil {
			log.Printf("Warning: skipping row %d, column 1 (%q) is not a number: %v", i, record[1], err)
			continue
		}

		// Column 6: filepath
		filepath := strings.TrimSpace(record[6])
		isTest := isTestFile(filepath)

		// Accumulate metrics separately for test and production code
		if isTest {
			testNLOC += nloc
			testFunctions++
		} else {
			totalNLOC += nloc
			totalFunctions++

			// Only count complexity for production code
			if ccn > 15 {
				highComplexityCount++
			}
		}
	}

	// Calculate test ratio
	var testRatio float64
	if totalNLOC > 0 {
		testRatio = float64(testNLOC) / float64(totalNLOC)
	}

	return &TechData{
		LinesOfCode:             totalNLOC,
		Functions:               totalFunctions,
		HighComplexityFunctions: highComplexityCount,
		TestLinesOfCode:         testNLOC,
		TestFunctions:           testFunctions,
		TestRatio:               testRatio,
	}, nil
}
