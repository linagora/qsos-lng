package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/pkg/engine"
	"github.com/otiai10/openaigo"
)

// truncateLines returns at most maxLines lines of content.
func truncateLines(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

// stripMarkdownFences removes optional ```json or ``` fences that LLMs sometimes add.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence line
		idx := strings.Index(s, "\n")
		if idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	return s
}

// debtResponse is the expected JSON shape from Stage 2.
type debtResponse struct {
	Score     float64  `json:"score"`
	KeyPoints []string `json:"key_points"`
}

// parseDebtResponse parses the Stage 2 LLM JSON into a clamped score and key points.
func parseDebtResponse(raw string) (float64, []string, error) {
	raw = stripMarkdownFences(raw)
	var resp debtResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return 0, nil, fmt.Errorf("failed to parse debt JSON: %w (raw: %s)", err, raw)
	}
	if len(resp.KeyPoints) == 0 {
		return 0, nil, fmt.Errorf("LLM returned empty key_points")
	}
	// Clamp score to [1.0, 5.0]
	score := resp.Score
	if score < 1.0 {
		score = 1.0
	}
	if score > 5.0 {
		score = 5.0
	}
	return score, resp.KeyPoints, nil
}

// parseFileSelection parses the Stage 1 LLM JSON array of file paths.
func parseFileSelection(raw string) ([]string, error) {
	raw = stripMarkdownFences(raw)
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, fmt.Errorf("failed to parse file selection JSON: %w (raw: %s)", err, raw)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("LLM returned empty file selection")
	}
	return paths, nil
}

// LLMDebtSource fetches a technical debt score using a two-stage LLM analysis.
type LLMDebtSource struct {
	client *github.Client
}

// NewLLMDebtSource creates a new LLM debt analysis source adapter.
func NewLLMDebtSource(client *github.Client) *LLMDebtSource {
	return &LLMDebtSource{client: client}
}

// Name returns the source name.
func (s *LLMDebtSource) Name() string {
	return "LLM Debt Analysis"
}

const promptFileSelection = `You are a senior software engineer conducting a technical debt review.
You will be given a repository file tree with file paths and sizes in bytes.

Select 8 to 12 files that are most likely to reveal the structural health
of the production codebase. Prioritize:
- Core business logic files (not tests, not generated code, not configuration)
- Large files by size — they tend to accumulate debt
- Files in directories suggesting central responsibility
  (e.g., core/, lib/, src/, internal/, pkg/)
- Files whose names suggest broad responsibility
  (e.g., manager, handler, service, processor, engine)

Avoid: test files, generated code, vendor code, documentation,
configuration, migration files.

Reply ONLY with a valid JSON array of file paths, no explanation,
no markdown:
["src/core/engine.py", "lib/parser.go"]`

const promptDebtScoring = `You are a senior software engineer performing a technical debt assessment.
You have been given a selection of key source files from a software project.

Score the technical debt on a scale from 1 to 5:
- 5: Clean, well-structured. Functions are short and focused, naming is
     clear, responsibilities are well-separated, minimal duplication.
- 4: Generally good with minor issues. A few long functions or unclear
     names, but the overall structure is sound.
- 3: Noticeable debt. Some functions too complex or too long, mixed
     responsibilities, moderate duplication or coupling.
- 2: Significant debt. Frequent violations: large functions/classes,
     unclear naming, tight coupling, considerable duplication.
     Maintenance is clearly impacted.
- 1: Severe debt. The codebase is hard to understand and modify.
     Pervasive complexity, poor structure, heavy coupling throughout.

Consider: cyclomatic complexity, function and file length, separation of
concerns, naming clarity, code duplication, coupling between modules.

Note: files longer than 300 lines have been truncated to their first 300
lines.

Reply ONLY with valid JSON, no markdown, no explanation:
{"score": <float 1.0-5.0>, "key_points": ["<one sentence>", ...]}

Include 3 to 5 key_points, each a single concrete observation
(positive or negative) about the code quality.`

const maxLinesPerFile = 300

// Fetch runs the two-stage LLM debt analysis.
func (s *LLMDebtSource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error) {
	log.Printf("  Fetching file tree for debt analysis...\n")

	// Stage 1: fetch file tree and ask LLM to pick files
	treeText, err := s.buildTreePrompt(ctx, execCtx.Owner, execCtx.Repo)
	if err != nil {
		return nil, fmt.Errorf("failed to build tree prompt: %w", err)
	}

	aiClient := s.newAIClient()
	selectedPaths, err := s.callFileSelection(ctx, aiClient, treeText)
	if err != nil {
		return nil, fmt.Errorf("file selection LLM call failed: %w", err)
	}
	log.Printf("  LLM selected %d files for debt analysis\n", len(selectedPaths))

	// Stage 2: fetch file contents and score
	files := s.fetchFileContents(ctx, execCtx.Owner, execCtx.Repo, selectedPaths)
	if len(files) < 3 {
		return nil, fmt.Errorf("too few files fetched (%d), need at least 3", len(files))
	}

	filesText := s.buildFilesPrompt(files)
	score, keyPoints, err := s.callDebtScoring(ctx, aiClient, filesText)
	if err != nil {
		return nil, fmt.Errorf("debt scoring LLM call failed: %w", err)
	}

	for _, point := range keyPoints {
		log.Printf("  [debt] %s\n", point)
	}

	return []engine.MetricResult{
		{Slug: "ai_technical_debt", Value: score, Source: "qsos-lng:llm"},
	}, nil
}

// newAIClient creates an openaigo client from environment variables.
func (s *LLMDebtSource) newAIClient() *openaigo.Client {
	client := openaigo.NewClient(os.Getenv("AI_API_KEY"))
	if u := os.Getenv("AI_BASE_URL"); u != "" {
		client.BaseURL = u
	}
	return client
}

// aiModel returns the configured model name.
func aiModel() string {
	if m := os.Getenv("AI_MODEL"); m != "" {
		return m
	}
	return "gpt-oss-120b"
}

// buildTreePrompt fetches the repo file tree from GitHub and formats it for the LLM.
func (s *LLMDebtSource) buildTreePrompt(ctx context.Context, owner, repo string) (string, error) {
	tree, _, err := s.client.Git.GetTree(ctx, owner, repo, "HEAD", true)
	if err != nil {
		return "", fmt.Errorf("failed to get repo tree: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Repository file tree (path, size in bytes):\n")
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		fmt.Fprintf(&sb, "%s (%d bytes)\n", entry.GetPath(), entry.GetSize())
	}
	return sb.String(), nil
}

// callFileSelection calls the LLM with the file tree and returns selected paths.
func (s *LLMDebtSource) callFileSelection(ctx context.Context, client *openaigo.Client, treeText string) ([]string, error) {
	req := openaigo.ChatRequest{
		Model:     aiModel(),
		MaxTokens: 1024,
		Messages: []openaigo.Message{
			{Role: "system", Content: promptFileSelection},
			{Role: "user", Content: treeText},
		},
	}
	resp, err := client.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("AI returned no choices")
	}
	return parseFileSelection(resp.Choices[0].Message.Content)
}

// fetchFileContents fetches file contents in parallel via GitHub API.
// Files that fail to fetch are silently skipped (with a log warning).
func (s *LLMDebtSource) fetchFileContents(ctx context.Context, owner, repo string, paths []string) map[string]string {
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make(map[string]string, len(paths))

	for _, path := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			content, err := s.fetchOneFile(ctx, owner, repo, p)
			if err != nil {
				log.Printf("  Warning: failed to fetch %s: %v\n", p, err)
				return
			}
			mu.Lock()
			results[p] = truncateLines(content, maxLinesPerFile)
			mu.Unlock()
		}(path)
	}
	wg.Wait()
	return results
}

// fetchOneFile fetches and decodes a single file from GitHub.
func (s *LLMDebtSource) fetchOneFile(ctx context.Context, owner, repo, path string) (string, error) {
	fileContent, _, _, err := s.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return "", err
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return "", err
	}
	return content, nil
}

// buildFilesPrompt formats the fetched files into a single prompt string.
func (s *LLMDebtSource) buildFilesPrompt(files map[string]string) string {
	var sb strings.Builder
	for path, content := range files {
		fmt.Fprintf(&sb, "\n\n=== %s ===\n%s", path, content)
	}
	return sb.String()
}

// callDebtScoring calls the LLM with the file contents and returns a score and key points.
func (s *LLMDebtSource) callDebtScoring(ctx context.Context, client *openaigo.Client, filesText string) (float64, []string, error) {
	req := openaigo.ChatRequest{
		Model:     aiModel(),
		MaxTokens: 1024,
		Messages: []openaigo.Message{
			{Role: "system", Content: promptDebtScoring},
			{Role: "user", Content: filesText},
		},
	}
	resp, err := client.Chat(ctx, req)
	if err != nil {
		return 0, nil, fmt.Errorf("AI error: %w", err)
	}
	if len(resp.Choices) == 0 {
		return 0, nil, fmt.Errorf("AI returned no choices")
	}
	return parseDebtResponse(resp.Choices[0].Message.Content)
}
