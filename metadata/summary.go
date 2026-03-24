package metadata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v76/github"
	"github.com/jackc/pgx/v5"
	"github.com/otiai10/openaigo"
)

const promptTLDRFrench = `
Tu es un agent dont le rôle est de créer une introduction en français pour un
logiciel Open-Source. Cette introduction devra faire un paragraphe de 3 ou 4
phrases (sans titre). Tu peux utiliser du markdown. Voici le README du logiciel
en question.
`

const promptTLDREnglish = `
You are an agent whose role is to create an introduction in English for an
Open-Source software. This introduction should be a paragraph of 3 or 4
sentences (no title). Markdown is allowed. Here is the README of the software
in question.
`

// BilingualSummary holds summaries in French and English
type BilingualSummary struct {
	French  string
	English string
}

// newAIClient constructs an openaigo client using environment variables and
// applies a sensible default base URL when AI_BASE_URL is not set.
func newAIClient() *openaigo.Client {
	c := openaigo.NewClient(os.Getenv("AI_API_KEY"))
	if u := os.Getenv("AI_BASE_URL"); u != "" {
		c.BaseURL = u
	} else {
		c.BaseURL = "https://openrouter.ai/api/v1"
	}
	return c
}

// GetSummary fetches and summarizes the project README using AI
func GetSummary(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return "", err
	}
	content, err := readme.GetContent()
	if err != nil {
		return "", err
	}

	aiClient := newAIClient()

	summary, err := summarize(ctx, aiClient, content, "fr")
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return summary, nil
}

// GetBilingualSummary fetches and summarizes the project README in both French and English
func GetBilingualSummary(ctx context.Context, client *github.Client, owner, repo string) (*BilingualSummary, error) {
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		return nil, err
	}
	content, err := readme.GetContent()
	if err != nil {
		return nil, err
	}

	aiClient := newAIClient()

	// Generate French summary
	frenchSummary, err := summarize(ctx, aiClient, content, "fr")
	if err != nil {
		return nil, fmt.Errorf("summarize french: %w", err)
	}

	// Generate English summary
	englishSummary, err := summarize(ctx, aiClient, content, "en")
	if err != nil {
		return nil, fmt.Errorf("summarize english: %w", err)
	}

	return &BilingualSummary{
		French:  frenchSummary,
		English: englishSummary,
	}, nil
}

const maxReadmeChars = 8000

func summarize(ctx context.Context, client *openaigo.Client, content string, locale string) (string, error) {
	if len(content) > maxReadmeChars {
		content = content[:maxReadmeChars]
	}
	if client.APIKey == "" {
		return "", errors.New("AI_API_KEY not set")
	}

	// Select prompt based on locale
	prompt := promptTLDRFrench
	if locale == "en" {
		prompt = promptTLDREnglish
	}

	model := "mistralai/mistral-small-2603"
	if m := os.Getenv("AI_MODEL"); m != "" {
		model = m
	}
	request := openaigo.ChatRequest{
		Model:     model,
		MaxTokens: 512,
		Messages: []openaigo.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: content},
		},
	}
	response, err := client.Chat(ctx, request)
	if err != nil {
		return "", fmt.Errorf("AI error: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("AI error: no response")
	}
	return response.Choices[0].Message.Content, nil
}

// SaveSummariesToDB saves bilingual summaries to the categories_block table
func SaveSummariesToDB(ctx context.Context, tx pgx.Tx, softwareID int64, summary *BilingualSummary) error {
	now := time.Now()
	kind := "overview"

	// Insert French summary
	_, err := tx.Exec(ctx, `
		INSERT INTO categories_block (software_id, kind, locale, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (software_id, kind, locale)
		DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at
	`, softwareID, kind, "fr", summary.French, now, now)
	if err != nil {
		return fmt.Errorf("failed to save French summary: %w", err)
	}

	// Insert English summary
	_, err = tx.Exec(ctx, `
		INSERT INTO categories_block (software_id, kind, locale, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (software_id, kind, locale)
		DO UPDATE SET content = EXCLUDED.content, updated_at = EXCLUDED.updated_at
	`, softwareID, kind, "en", summary.English, now, now)
	if err != nil {
		return fmt.Errorf("failed to save English summary: %w", err)
	}

	return nil
}
