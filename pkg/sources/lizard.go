package sources

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linagora/qsos-lng/pkg/engine"
)

// FileInfo holds information about a source file
type FileInfo struct {
	Path     string
	Language string
	Lines    int
	IsTest   bool
}

// TokeiReport represents a single file report from tokei
type TokeiReport struct {
	Name  string `json:"name"`
	Stats struct {
		Code     int `json:"code"`
		Comments int `json:"comments"`
		Blanks   int `json:"blanks"`
	} `json:"stats"`
}

// TokeiLanguage represents a language entry from tokei output
type TokeiLanguage struct {
	Reports []TokeiReport `json:"reports"`
}

// LizardSource fetches metrics using Lizard code analysis
type LizardSource struct {
	dockerTimeout       time.Duration
	maxFilesForAnalysis int
}

// NewLizardSource creates a new Lizard source adapter
func NewLizardSource(dockerTimeoutMinutes int) *LizardSource {
	return &LizardSource{
		dockerTimeout:       time.Duration(dockerTimeoutMinutes) * time.Minute,
		maxFilesForAnalysis: 2000,
	}
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

	// Run tokei for fast file statistics
	tokeiOutput, err := s.runTokei(ctx, tmpDir)
	if err != nil {
		log.Printf("Tokei failed, falling back to Lizard-only: %v", err)
		return s.fetchWithLizardOnly(ctx, tmpDir)
	}

	// Classify files into production and test
	prodFiles, testFiles := s.classifyFiles(tokeiOutput)

	// Calculate LOC metrics from tokei (all files)
	var totalProdLOC, totalTestLOC int
	for _, f := range prodFiles {
		totalProdLOC += f.Lines
	}
	for _, f := range testFiles {
		totalTestLOC += f.Lines
	}

	log.Printf("  Tokei: %d production files (%d LOC), %d test files (%d LOC)",
		len(prodFiles), totalProdLOC, len(testFiles), totalTestLOC)

	// Select files for complexity analysis
	filesToAnalyze := s.selectFilesForAnalysis(prodFiles)

	// Run Lizard on selected files
	var totalFunctions, highComplexityCount int64
	if len(filesToAnalyze) > 0 {
		lizardRecords, err := s.runLizardOnFiles(ctx, tmpDir, filesToAnalyze)
		if err != nil {
			log.Printf("Lizard analysis failed: %v", err)
		} else {
			// Count functions and complexity from Lizard output
			for _, record := range lizardRecords {
				totalFunctions++
				if record.CCN > 15 {
					highComplexityCount++
				}
			}
		}
	}

	// Calculate derived metrics
	var testRatio float64
	if totalProdLOC > 0 {
		testRatio = float64(totalTestLOC) / float64(totalProdLOC)
	}

	var complexityPercentage float64
	if totalFunctions > 0 {
		complexityPercentage = float64(highComplexityCount) * 100.0 / float64(totalFunctions)
	}

	results := []engine.MetricResult{
		{Slug: "lines_of_code", Value: float64(totalProdLOC), Source: "qsos-lng:tokei"},
		{Slug: "functions", Value: float64(totalFunctions), Source: "qsos-lng:lizard"},
		{Slug: "high_complexity_functions", Value: float64(highComplexityCount), Source: "qsos-lng:lizard"},
		{Slug: "test_lines_of_code", Value: float64(totalTestLOC), Source: "qsos-lng:tokei"},
		{Slug: "test_ratio", Value: testRatio, Source: "qsos-lng:tokei"},
		{Slug: "complexity", Value: complexityPercentage, Source: "qsos-lng:lizard"},
	}

	log.Printf("  LOC: %d, Functions: %d, High-complexity: %d (%.1f%%), Test ratio: %.2f\n",
		totalProdLOC, totalFunctions, highComplexityCount, complexityPercentage, testRatio)

	return results, nil
}

// LizardRecord holds parsed data from a Lizard CSV row
type LizardRecord struct {
	NLOC     int64
	CCN      int64
	FilePath string
}

// runTokei executes tokei and returns parsed output
func (s *LizardSource) runTokei(ctx context.Context, dir string) (map[string]TokeiLanguage, error) {
	dockerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(dockerCtx,
		"docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v", fmt.Sprintf("%s:/src:ro", dir),
		"ghcr.io/xampprocky/tokei:latest",
		"--output", "json", "--files", "/src",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tokei execution failed: %w (stderr: %s)", err, stderr.String())
	}

	var result map[string]TokeiLanguage
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse tokei output: %w", err)
	}

	return result, nil
}

// classifyFiles separates production and test files
func (s *LizardSource) classifyFiles(tokei map[string]TokeiLanguage) (prod []FileInfo, test []FileInfo) {
	for lang, langData := range tokei {
		for _, report := range langData.Reports {
			lines := report.Stats.Code
			if lines == 0 {
				continue
			}

			info := FileInfo{
				Path:     report.Name,
				Language: lang,
				Lines:    lines,
				IsTest:   isTestFile(report.Name),
			}

			if info.IsTest {
				test = append(test, info)
			} else {
				prod = append(prod, info)
			}
		}
	}
	return prod, test
}

// skipPathPatterns contains directory/path patterns to skip for complexity analysis
var skipPathPatterns = []string{
	// Third-party and vendored code
	"/vendor/", "/third_party/", "/third-party/", "/thirdparty/",
	"/external/", "/extern/", "/deps/", "/dependencies/",
	"/node_modules/", "/bower_components/",
	"/contrib/", "/contributed/",

	// Build, CI, and tooling
	"/.ci/", "/.github/", "/.gitlab/", "/.circleci/",
	"/build/", "/cmake/", "/make/", "/m4/",
	"/scripts/", "/tools/", "/tooling/",
	"/.buildkite/", "/.travis/",

	// Configuration and metadata
	"/config/", "/configs/", "/configuration/",
	"/.config/", "/settings/",

	// Documentation and examples
	"/docs/", "/doc/", "/documentation/",
	"/examples/", "/example/", "/samples/", "/sample/",
	"/tutorials/", "/tutorial/", "/demo/", "/demos/",

	// Benchmarks and performance tests
	"/benchmarks/", "/benchmark/", "/bench/",
	"/perf/", "/performance/",

	// Generated code markers
	"/generated/", "/gen/", "/auto/", "/autogen/",
	"/_generated/", "/codegen/",

	// Assets and resources
	"/assets/", "/static/", "/public/",
	"/resources/", "/res/", "/data/",
	"/images/", "/img/", "/icons/",
	"/fonts/", "/locales/", "/i18n/", "/l10n/",

	// IDE and editor
	"/.idea/", "/.vscode/", "/.vs/",
}

// shouldSkipFile checks if a file should be skipped based on path patterns
func shouldSkipFile(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, pattern := range skipPathPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// selectFilesForAnalysis filters and selects the most relevant files for complexity analysis
func (s *LizardSource) selectFilesForAnalysis(files []FileInfo) []FileInfo {
	// Filter out non-essential directories
	var filtered []FileInfo
	for _, f := range files {
		if !shouldSkipFile(f.Path) {
			filtered = append(filtered, f)
		}
	}
	if len(filtered) < len(files) {
		log.Printf("  Filtered out %d files in non-essential directories", len(files)-len(filtered))
	}

	// If small enough, use all
	if len(filtered) <= s.maxFilesForAnalysis {
		return filtered
	}

	// Sort by size (descending) and take largest files
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Lines > filtered[j].Lines
	})

	log.Printf("  Selected %d largest files for complexity analysis", s.maxFilesForAnalysis)
	return filtered[:s.maxFilesForAnalysis]
}

// runLizardOnFiles executes Lizard on specific files
func (s *LizardSource) runLizardOnFiles(ctx context.Context, dir string, files []FileInfo) ([]LizardRecord, error) {
	if len(files) == 0 {
		return nil, nil
	}

	// Write file paths to a temporary file (one per line)
	fileListPath := filepath.Join(dir, ".lizard_files.txt")
	var fileListContent strings.Builder
	for _, f := range files {
		// Convert from /src/... to relative path within container
		relPath := strings.TrimPrefix(f.Path, "/src/")
		fmt.Fprintf(&fileListContent, "/src/%s\n", relPath)
	}
	if err := os.WriteFile(fileListPath, []byte(fileListContent.String()), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file list: %w", err)
	}

	dockerCtx, cancel := context.WithTimeout(ctx, s.dockerTimeout)
	defer cancel()

	// Use xargs to pass file list to lizard (lizard doesn't support --input-file)
	cmd := exec.CommandContext(dockerCtx,
		"docker", "run", "--rm",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", "HOME=/tmp",
		"-v", fmt.Sprintf("%s:/src", dir),
		"python:3.13-alpine",
		"sh", "-c", "pip install lizard --quiet && cat /src/.lizard_files.txt | xargs /tmp/.local/bin/lizard --csv",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("lizard execution failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse CSV output
	reader := csv.NewReader(bytes.NewReader(stdout.Bytes()))
	csvRecords, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	var records []LizardRecord
	for _, row := range csvRecords {
		if len(row) < 7 {
			continue
		}

		nloc, err := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
		if err != nil {
			continue
		}

		ccn, err := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		if err != nil {
			continue
		}

		records = append(records, LizardRecord{
			NLOC:     nloc,
			CCN:      ccn,
			FilePath: strings.TrimSpace(row[6]),
		})
	}

	return records, nil
}

// fetchWithLizardOnly is the fallback when tokei fails
func (s *LizardSource) fetchWithLizardOnly(ctx context.Context, tmpDir string) ([]engine.MetricResult, error) {
	dockerCtx, cancel := context.WithTimeout(ctx, s.dockerTimeout)
	defer cancel()

	cmd := exec.CommandContext(dockerCtx,
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
	var testNLOC int64

	for _, record := range records {
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

		filePath := strings.TrimSpace(record[6])
		isTest := isTestFile(filePath)

		if isTest {
			testNLOC += nloc
		} else {
			totalNLOC += nloc
			totalFunctions++
			if ccn > 15 {
				highComplexityCount++
			}
		}
	}

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
