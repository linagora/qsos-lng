package community

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/common"
)

// FetchDocumentation retrieves documentation-related metrics from GitHub
func FetchDocumentation(ctx context.Context, client *github.Client, owner, repo string) (*DocumentationData, error) {
	data := &DocumentationData{}

	// 1. Fetch README and analyze it
	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err == nil && readme.Content != nil {
		content, err := readme.GetContent()
		if err == nil {
			data.ReadmeLength = int64(len(strings.Fields(content)))
			data.KeySectionsCount = countKeySections(content)
		}
	}

	// 2. Check for docs directory
	_, dirContent, _, err := client.Repositories.GetContents(ctx, owner, repo, "docs", nil)
	if err == nil && dirContent != nil {
		data.HasDocsDirectory = true
		data.DocsFileCount = int64(len(dirContent))
	}

	// Alternative locations for docs
	if !data.HasDocsDirectory {
		_, dirContent, _, err := client.Repositories.GetContents(ctx, owner, repo, "doc", nil)
		if err == nil && dirContent != nil {
			data.HasDocsDirectory = true
			data.DocsFileCount = int64(len(dirContent))
		}
	}

	// 3. Check for multi-language READMEs
	_, rootContent, _, err := client.Repositories.GetContents(ctx, owner, repo, "", nil)
	if err == nil && rootContent != nil {
		readmePattern := regexp.MustCompile(`(?i)^readme\.([a-z]{2})\.md$`)
		for _, file := range rootContent {
			if file.Name != nil && readmePattern.MatchString(*file.Name) {
				data.MultiLanguageCount++
			}
		}
	}

	// 4. Check for CONTRIBUTING.md
	_, _, _, err = client.Repositories.GetContents(ctx, owner, repo, "CONTRIBUTING.md", nil)
	if err == nil {
		data.HasContributing = true
	}
	if !data.HasContributing {
		_, _, _, err = client.Repositories.GetContents(ctx, owner, repo, ".github/CONTRIBUTING.md", nil)
		if err == nil {
			data.HasContributing = true
		}
	}

	// 5. Check for CODE_OF_CONDUCT.md
	_, _, _, err = client.Repositories.GetContents(ctx, owner, repo, "CODE_OF_CONDUCT.md", nil)
	if err == nil {
		data.HasCodeOfConduct = true
	}
	if !data.HasCodeOfConduct {
		_, _, _, err = client.Repositories.GetContents(ctx, owner, repo, ".github/CODE_OF_CONDUCT.md", nil)
		if err == nil {
			data.HasCodeOfConduct = true
		}
	}

	// 6. Check for issue templates
	_, issueTemplates, _, err := client.Repositories.GetContents(ctx, owner, repo, ".github/ISSUE_TEMPLATE", nil)
	if err == nil && issueTemplates != nil && len(issueTemplates) > 0 {
		data.HasIssueTemplates = true
	}

	// 7. Check if wiki is enabled (GitHub API doesn't provide direct wiki check, but we can infer from repository)
	repository, _, err := client.Repositories.Get(ctx, owner, repo)
	if err == nil && repository.HasWiki != nil && *repository.HasWiki {
		data.HasWiki = true
	}

	return data, nil
}

// countKeySections counts important sections in the README
func countKeySections(content string) int64 {
	content = strings.ToLower(content)
	keySections := []string{
		"installation",
		"install",
		"getting started",
		"quick start",
		"usage",
		"api",
		"documentation",
		"contributing",
		"contribute",
		"examples",
		"example",
		"configuration",
		"config",
		"requirements",
		"prerequisites",
		"license",
		"features",
	}

	count := int64(0)
	for _, section := range keySections {
		// Look for markdown headers with these sections
		patterns := []string{
			fmt.Sprintf("# %s", section),
			fmt.Sprintf("## %s", section),
			fmt.Sprintf("### %s", section),
		}
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				count++
				break // Count each section only once
			}
		}
	}
	return count
}

// ComputeDocumentation calculates the documentation quality score
// Score is based on a weighted combination of multiple factors
func ComputeDocumentation(data *DocumentationData, threshold [4]int64) int64 {
	// Calculate a composite score based on multiple factors
	score := int64(0)
	maxScore := int64(0)

	// README quality (weight: 40 points)
	// - Length: 20 points
	readmeLengthScore := common.ComputeScore(data.ReadmeLength, [4]int64{100, 500, 1500, 3000}, common.BiggerIsBetter)
	score += readmeLengthScore * 4 // Scale to 20 points max (5 * 4)
	maxScore += 20

	// - Key sections: 20 points
	keySectionsScore := common.ComputeScore(data.KeySectionsCount, [4]int64{2, 4, 6, 8}, common.BiggerIsBetter)
	score += keySectionsScore * 4 // Scale to 20 points max (5 * 4)
	maxScore += 20

	// Documentation coverage (weight: 30 points)
	if data.HasDocsDirectory {
		// Docs directory exists: 15 points
		score += 15
		// Number of files in docs: 15 points
		docsScore := common.ComputeScore(data.DocsFileCount, [4]int64{1, 5, 15, 30}, common.BiggerIsBetter)
		score += docsScore * 3 // Scale to 15 points max (5 * 3)
	}
	maxScore += 30

	// Accessibility (weight: 20 points)
	if data.HasContributing {
		score += 8
	}
	if data.HasCodeOfConduct {
		score += 6
	}
	if data.HasIssueTemplates {
		score += 6
	}
	maxScore += 20

	// Multi-language support (weight: 10 points)
	multiLangScore := common.ComputeScore(data.MultiLanguageCount, [4]int64{1, 2, 3, 5}, common.BiggerIsBetter)
	score += multiLangScore * 2 // Scale to 10 points max (5 * 2)
	maxScore += 10

	// Convert to percentage (0-100) then map to 1-5 scale using thresholds
	percentage := (score * 100) / maxScore
	return common.ComputeScore(percentage, threshold, common.BiggerIsBetter)
}
