package block

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/go-github/v76/github"
	"github.com/otiai10/openaigo"
)

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

	aiClient := openaigo.NewClient(os.Getenv("AI_API_KEY"))
	if u := os.Getenv("AI_BASE_URL"); u != "" {
		aiClient.BaseURL = u
	}

	summary, err := summarize(ctx, aiClient, content)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return summary, nil
}

const promptTLDR = `
Tu es un agent dont le rôle est de créer une introduction en français pour un
logiciel Open-Source. Cette introduction devra faire 4 ou 5 phrases. Voici le
README du logiciel en question.
`

func summarize(ctx context.Context, client *openaigo.Client, content string) (string, error) {
	if client.APIKey == "" {
		return "", errors.New("AI_API_KEY not set")
	}

	model := "gpt-oss-120b"
	if m := os.Getenv("AI_MODEL"); m != "" {
		model = m
	}
	request := openaigo.ChatRequest{
		Model: model,
		Messages: []openaigo.Message{
			{Role: "system", Content: promptTLDR},
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
