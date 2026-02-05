package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/pkg/engine"
)

// DocumentationSource fetches documentation metrics from GitHub
type DocumentationSource struct {
	client *github.Client
}

// NewDocumentationSource creates a new documentation source adapter
func NewDocumentationSource(client *github.Client) *DocumentationSource {
	return &DocumentationSource{client: client}
}

// Name returns the source name
func (s *DocumentationSource) Name() string {
	return "Documentation"
}

// Fetch retrieves all documentation metrics
func (s *DocumentationSource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error) {
	results := make([]engine.MetricResult, 0)

	var readmeLength int64
	var keySectionsCount int64

	// 1. Fetch README and analyze it
	for {
		readme, _, err := s.client.Repositories.GetReadme(ctx, execCtx.Owner, execCtx.Repo, nil)
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			// README not found is not fatal, just continue with zero values
			break
		}
		if readme.Content != nil {
			content, err := readme.GetContent()
			if err == nil {
				readmeLength = int64(len(strings.Fields(content)))
				keySectionsCount = countKeySections(content)
			}
		}
		break
	}

	// Calculate README quality score (0-100)
	readmeQuality := calculateReadmeQuality(readmeLength, keySectionsCount)
	results = append(results, engine.MetricResult{
		Slug:   "readme_quality",
		Value:  float64(readmeQuality),
		Source: "qsos-lng:documentation",
	})

	results = append(results, engine.MetricResult{
		Slug:   "readme_sections",
		Value:  float64(keySectionsCount),
		Source: "qsos-lng:documentation",
	})

	// 2. Check for docs directory and count files
	var hasDocsDirectory bool
	var docsFileCount int64
	var rootContent []*github.RepositoryContent

	for {
		var err error
		_, rootContent, _, err = s.client.Repositories.GetContents(ctx, execCtx.Owner, execCtx.Repo, "", nil)
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			// Root content not accessible, continue with defaults
			break
		}
		break
	}

	if rootContent != nil {
		readmePattern := regexp.MustCompile(`(?i)^readme\.([a-z]{2})\.md$`)
		multiLanguageCount := int64(0)

		for _, file := range rootContent {
			if file.Name == nil {
				continue
			}

			// Check for docs directory
			if !hasDocsDirectory && file.Type != nil && *file.Type == "dir" {
				nameLower := strings.ToLower(*file.Name)
				if nameLower == "docs" || nameLower == "doc" {
					for {
						_, dirContent, _, err := s.client.Repositories.GetContents(ctx, execCtx.Owner, execCtx.Repo, *file.Name, nil)
						if err != nil {
							if handleRateLimit(err) {
								continue
							}
							break
						}
						if dirContent != nil {
							hasDocsDirectory = true
							docsFileCount = int64(len(dirContent))
						}
						break
					}
				}
			}

			// Check for multi-language READMEs
			if readmePattern.MatchString(*file.Name) {
				multiLanguageCount++
			}
		}

		// Add multi-language metric
		results = append(results, engine.MetricResult{
			Slug:   "multilang_readmes",
			Value:  float64(multiLanguageCount),
			Source: "qsos-lng:documentation",
		})
	}

	// Calculate docs directory score (0-100)
	docsScore := calculateDocsScore(hasDocsDirectory, docsFileCount)
	results = append(results, engine.MetricResult{
		Slug:   "docs_directory",
		Value:  float64(docsScore),
		Source: "qsos-lng:documentation",
	})

	// 3. Check for accessibility features
	var hasContributing bool
	var hasIssueTemplates bool

	for {
		_, _, _, err := s.client.Repositories.GetContents(ctx, execCtx.Owner, execCtx.Repo, "CONTRIBUTING.md", nil)
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			// File not found, try alternate location
			break
		}
		hasContributing = true
		break
	}
	if !hasContributing {
		for {
			_, _, _, err := s.client.Repositories.GetContents(ctx, execCtx.Owner, execCtx.Repo, ".github/CONTRIBUTING.md", nil)
			if err != nil {
				if handleRateLimit(err) {
					continue
				}
				break
			}
			hasContributing = true
			break
		}
	}

	for {
		_, issueTemplates, _, err := s.client.Repositories.GetContents(ctx, execCtx.Owner, execCtx.Repo, ".github/ISSUE_TEMPLATE", nil)
		if err != nil {
			if handleRateLimit(err) {
				continue
			}
			break
		}
		if issueTemplates != nil && len(issueTemplates) > 0 {
			hasIssueTemplates = true
		}
		break
	}

	// Calculate accessibility score (0-100)
	accessibilityScore := calculateAccessibilityScore(hasContributing, hasIssueTemplates)
	results = append(results, engine.MetricResult{
		Slug:   "accessibility",
		Value:  float64(accessibilityScore),
		Source: "qsos-lng:documentation",
	})

	log.Printf("  README quality: %d, Sections: %d, Docs: %d, Accessibility: %d\n",
		readmeQuality, keySectionsCount, docsScore, accessibilityScore)

	return results, nil
}

// calculateReadmeQuality computes a 0-100 score for README quality
func calculateReadmeQuality(length, sections int64) int64 {
	// Length score (0-50 points)
	lengthScore := int64(0)
	if length >= 3000 {
		lengthScore = 50
	} else if length >= 1500 {
		lengthScore = 40
	} else if length >= 500 {
		lengthScore = 30
	} else if length >= 100 {
		lengthScore = 20
	} else {
		lengthScore = 10
	}

	// Sections score (0-50 points)
	sectionsScore := int64(0)
	if sections >= 8 {
		sectionsScore = 50
	} else if sections >= 6 {
		sectionsScore = 40
	} else if sections >= 4 {
		sectionsScore = 30
	} else if sections >= 2 {
		sectionsScore = 20
	} else {
		sectionsScore = 10
	}

	return lengthScore + sectionsScore
}

// calculateDocsScore computes a 0-100 score for documentation directory
func calculateDocsScore(hasDir bool, fileCount int64) int64 {
	if !hasDir {
		return 0
	}

	// Base 50 points for having a docs directory
	score := int64(50)

	// Up to 50 more points based on file count
	if fileCount >= 30 {
		score += 50
	} else if fileCount >= 15 {
		score += 40
	} else if fileCount >= 5 {
		score += 30
	} else if fileCount >= 1 {
		score += 20
	}

	return score
}

// calculateAccessibilityScore computes a 0-100 score for accessibility
func calculateAccessibilityScore(hasContributing, hasIssueTemplates bool) int64 {
	score := int64(0)
	if hasContributing {
		score += 57 // 8/14 * 100 ≈ 57
	}
	if hasIssueTemplates {
		score += 43 // 6/14 * 100 ≈ 43
	}
	return score
}

// countKeySections counts important sections in the README
func countKeySections(content string) int64 {
	content = strings.ToLower(content)
	keySections := []string{
		"installation", "install", "getting started", "quick start",
		"usage", "api", "documentation", "contributing", "contribute",
		"examples", "example", "configuration", "config",
		"requirements", "prerequisites", "license", "features",
	}

	count := int64(0)
	for _, section := range keySections {
		patterns := []string{
			fmt.Sprintf("# %s", section),
			fmt.Sprintf("## %s", section),
			fmt.Sprintf("### %s", section),
		}
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				count++
				break
			}
		}
	}
	return count
}
