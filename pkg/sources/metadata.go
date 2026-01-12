package sources

import (
	"context"
	"log"

	"github.com/google/go-github/v76/github"
	"github.com/jackc/pgx/v5"
	"github.com/linagora/qsos-lng/metadata"
	"github.com/linagora/qsos-lng/pkg/engine"
)

// BilingualSummaryAdapter generates bilingual AI summaries
type BilingualSummaryAdapter struct {
	client *github.Client
}

// NewBilingualSummaryAdapter creates a new bilingual summary adapter
func NewBilingualSummaryAdapter(client *github.Client) *BilingualSummaryAdapter {
	return &BilingualSummaryAdapter{client: client}
}

// Name returns the adapter name
func (a *BilingualSummaryAdapter) Name() string {
	return "Bilingual Summaries"
}

// Execute generates and saves bilingual summaries
func (a *BilingualSummaryAdapter) Execute(ctx context.Context, execCtx *engine.ExecutionContext, tx pgx.Tx) error {
	log.Printf("    Generating bilingual summaries...\n")
	summaries, err := metadata.GetBilingualSummary(ctx, a.client, execCtx.Owner, execCtx.Repo)
	if err != nil {
		return err
	}

	return metadata.SaveSummariesToDB(ctx, tx, execCtx.SoftwareID, summaries)
}

// TagsAdapter generates AI tags
type TagsAdapter struct {
	client *github.Client
}

// NewTagsAdapter creates a new tags adapter
func NewTagsAdapter(client *github.Client) *TagsAdapter {
	return &TagsAdapter{client: client}
}

// Name returns the adapter name
func (a *TagsAdapter) Name() string {
	return "AI Tags"
}

// Execute generates and saves tags
func (a *TagsAdapter) Execute(ctx context.Context, execCtx *engine.ExecutionContext, tx pgx.Tx) error {
	log.Printf("    Generating AI tags...\n")
	tags, err := metadata.GetTags(ctx, a.client, tx, execCtx.Owner, execCtx.Repo)
	if err != nil {
		return err
	}

	return metadata.SaveTagsToDB(ctx, tx, execCtx.SoftwareID, tags)
}

// IconAdapter resolves project icon URL
type IconAdapter struct {
	client *github.Client
}

// NewIconAdapter creates a new icon adapter
func NewIconAdapter(client *github.Client) *IconAdapter {
	return &IconAdapter{client: client}
}

// Name returns the adapter name
func (a *IconAdapter) Name() string {
	return "Icon URL"
}

// Execute resolves and saves the icon URL
func (a *IconAdapter) Execute(ctx context.Context, execCtx *engine.ExecutionContext, tx pgx.Tx) error {
	log.Printf("    Resolving icon URL...\n")
	iconURL, err := metadata.GetIconURL(ctx, a.client, execCtx.Owner, execCtx.Repo, execCtx.Language)
	if err != nil {
		log.Printf("      Warning: Failed to get icon URL: %v\n", err)
		return nil // Non-fatal
	}

	if iconURL != "" {
		_, err = tx.Exec(ctx, "UPDATE categories_software SET logo_url = $1 WHERE id = $2", iconURL, execCtx.SoftwareID)
		return err
	}

	return nil
}
