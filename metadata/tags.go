package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v76/github"
	"github.com/jackc/pgx/v5"
	"github.com/otiai10/openaigo"
)

// DBQuerier is an interface for database operations (satisfied by both pgx.Conn and pgx.Tx)
type DBQuerier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

const promptTagsTemplate = `
You are an agent whose role is to analyze an Open-Source project and generate 2 to 4 relevant tags that describe it.

RETURN ONLY a valid JSON array of strings and NOTHING ELSE. Example format:
["metrics database", "monitoring"]

Do NOT include any surrounding text, explanation, or Markdown code fences (triple backticks), and do NOT wrap the JSON in single backticks or other inline code markers.

Tags should be:
- Short (1-3 words maximum)
- Descriptive of the project's domain, technology, or purpose
- Lowercase
- Generic enough to be reused across similar projects
- But not too generic (like tool or software)

Tags have 2 purposes:
- to compare projects that do the same thing, like "container runtime"
- to explore and search projects, like "kubernetes".

Prioritize reusing existing tags when they are relevant. Here is the list of existing tags in the database:
%s

Here is the README of the software in question:
`

// FetchExistingTags retrieves all existing tags from the database
func FetchExistingTags(ctx context.Context, db DBQuerier) ([]string, error) {
	rows, err := db.Query(ctx, "SELECT name FROM categories_tag ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}

	return tags, nil
}

// GetTags fetches and analyzes the project README to generate relevant tags using AI
func GetTags(ctx context.Context, client *github.Client, db DBQuerier, owner, repo string) ([]string, error) {
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return nil, err
	}
	content, err := readme.GetContent()
	if err != nil {
		return nil, err
	}

	// Fetch existing tags from database
	existingTags, err := FetchExistingTags(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("fetch existing tags: %w", err)
	}

	aiClient := newAIClient()

	tags, err := generateTags(ctx, aiClient, content, existingTags)
	if err != nil {
		return nil, fmt.Errorf("generate tags: %w", err)
	}
	return tags, nil
}

func generateTags(ctx context.Context, client *openaigo.Client, content string, existingTags []string) ([]string, error) {
	if len(content) > maxReadmeChars {
		content = content[:maxReadmeChars]
	}
	if client.APIKey == "" {
		return nil, errors.New("AI_API_KEY not set")
	}

	// Build the list of existing tags for the prompt
	existingTagsList := "None yet."
	if len(existingTags) > 0 {
		existingTagsList = strings.Join(existingTags, ", ")
	}

	// Build the prompt with existing tags
	prompt := fmt.Sprintf(promptTagsTemplate, existingTagsList)

	model := "mistralai/mistral-small-2603"
	if m := os.Getenv("AI_MODEL"); m != "" {
		model = m
	}
	request := openaigo.ChatRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages: []openaigo.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: content},
		},
	}
	response, err := client.Chat(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("AI error: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("AI error: no response")
	}

	// Parse JSON response defensively: models sometimes wrap JSON in markdown
	var tags []string
	responseContent := strings.TrimSpace(response.Choices[0].Message.Content)
	if responseContent == "" {
		return nil, fmt.Errorf("AI returned empty content (finish_reason: %s)", response.Choices[0].FinishReason)
	}

	// Helper: clean common markdown/code-fence wrappers
	cleanForJSON := func(s string) string {
		s = strings.TrimSpace(s)
		// If the model returned a fenced block (```...```), extract inner content
		if strings.HasPrefix(s, "```") {
			last := strings.LastIndex(s, "```")
			if last > 3 {
				inner := s[3:last]
				inner = strings.TrimSpace(inner)
				// strip optional leading "json"
				if strings.HasPrefix(inner, "json") {
					inner = strings.TrimSpace(inner[4:])
				}
				return inner
			}
		}
		// Remove stray backticks
		s = strings.ReplaceAll(s, "`", "")
		return strings.TrimSpace(s)
	}

	cleaned := cleanForJSON(responseContent)

	// First try: unmarshal cleaned content
	err = json.Unmarshal([]byte(cleaned), &tags)
	if err != nil {
		// Second try: extract first JSON array between '[' and ']' and unmarshal that
		if start := strings.Index(cleaned, "["); start != -1 {
			if end := strings.LastIndex(cleaned, "]"); end != -1 && end > start {
				candidate := cleaned[start : end+1]
				candidate = strings.TrimSpace(candidate)
				err2 := json.Unmarshal([]byte(candidate), &tags)
				if err2 == nil {
					err = nil
				}
			}
		}
	}
	if err != nil {
		// Truncate cleaned response for logs to avoid huge dumps
		display := cleaned
		if len(display) > 500 {
			display = display[:500] + "..."
		}
		return nil, fmt.Errorf("failed to parse tags JSON: %w (cleaned response: %s)", err, display)
	}

	// Validate and clean tags
	var validTags []string
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag != "" && len(tag) <= 50 { // Reasonable max length
			validTags = append(validTags, tag)
		}
	}

	if len(validTags) < 2 {
		return nil, fmt.Errorf("insufficient tags generated: got %d, expected 3-5", len(validTags))
	}
	if len(validTags) > 8 {
		validTags = validTags[:8]
	}

	return validTags, nil
}

// slugify converts a tag name to a URL-friendly slug
func slugify(name string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '_' || r == '.' {
			return '-'
		}
		return -1 // Remove character
	}, slug)

	// Remove consecutive hyphens and trim
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	return slug
}

// SaveTagsToDB inserts tags and associates them with the software project
func SaveTagsToDB(ctx context.Context, tx pgx.Tx, softwareID int64, tags []string) error {
	for _, tagName := range tags {
		tagSlug := slugify(tagName)
		if tagSlug == "" {
			continue // Skip invalid tags
		}

		// Insert tag if it doesn't exist (using ON CONFLICT to handle duplicates)
		var tagID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO categories_tag (name, slug)
			VALUES ($1, $2)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tagName, tagSlug).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("failed to insert/get tag '%s': %w", tagName, err)
		}

		// Create association between software and tag (ignore if already exists)
		_, err = tx.Exec(ctx, `
			INSERT INTO categories_software_tags (software_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT (software_id, tag_id) DO NOTHING
		`, softwareID, tagID)
		if err != nil {
			return fmt.Errorf("failed to associate tag '%s' with software: %w", tagName, err)
		}
	}

	return nil
}
