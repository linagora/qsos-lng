package community

import (
	"context"
	"fmt"
	"log"
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

	// 2. Fetch root directory once and check for both docs directory and multi-language READMEs
	_, rootContent, _, err := client.Repositories.GetContents(ctx, owner, repo, "", nil)
	if err == nil && rootContent != nil {
		readmePattern := regexp.MustCompile(`(?i)^readme\.([a-z]{2})\.md$`)

		for _, file := range rootContent {
			if file.Name == nil {
				continue
			}

			// Check for docs directory (case-insensitive: docs, Docs, doc, Doc, etc.)
			if !data.HasDocsDirectory && file.Type != nil && *file.Type == "dir" {
				nameLower := strings.ToLower(*file.Name)
				if nameLower == "docs" || nameLower == "doc" {
					// Found a docs directory, now get its contents
					_, dirContent, _, err := client.Repositories.GetContents(ctx, owner, repo, *file.Name, nil)
					if err == nil && dirContent != nil {
						data.HasDocsDirectory = true
						data.DocsFileCount = int64(len(dirContent))
					}
				}
			}

			// Check for multi-language READMEs
			if readmePattern.MatchString(*file.Name) {
				data.MultiLanguageCount++
			}
		}
	}

	// 3. Check for CONTRIBUTING.md
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

	// 4. Check for issue templates
	_, issueTemplates, _, err := client.Repositories.GetContents(ctx, owner, repo, ".github/ISSUE_TEMPLATE", nil)
	if err == nil && issueTemplates != nil && len(issueTemplates) > 0 {
		data.HasIssueTemplates = true
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
	log.Printf("\n--- Documentation Score Computation ---")

	// Log raw metrics
	log.Printf("Raw Metrics:")
	log.Printf("  README length (words): %d", data.ReadmeLength)
	log.Printf("  Key sections count: %d", data.KeySectionsCount)
	log.Printf("  Has docs directory: %v", data.HasDocsDirectory)
	log.Printf("  Docs file count: %d", data.DocsFileCount)
	log.Printf("  Has CONTRIBUTING.md: %v", data.HasContributing)
	log.Printf("  Has issue templates: %v", data.HasIssueTemplates)
	log.Printf("  Multi-language READMEs: %d", data.MultiLanguageCount)

	// Calculate a composite score based on multiple factors
	score := int64(0)
	maxScore := int64(0)

	// README quality (weight: 40 points)
	log.Printf("\nREADME Quality (40 points max):")

	// - Length: 20 points
	readmeLengthScore := common.ComputeScore(data.ReadmeLength, [4]int64{100, 500, 1500, 3000}, common.BiggerIsBetter)
	readmeLengthPoints := readmeLengthScore * 4
	log.Printf("  Length score: %d/5 → %d/20 points", readmeLengthScore, readmeLengthPoints)
	score += readmeLengthPoints
	maxScore += 20

	// - Key sections: 20 points
	keySectionsScore := common.ComputeScore(data.KeySectionsCount, [4]int64{2, 4, 6, 8}, common.BiggerIsBetter)
	keySectionsPoints := keySectionsScore * 4
	log.Printf("  Key sections score: %d/5 → %d/20 points", keySectionsScore, keySectionsPoints)
	score += keySectionsPoints
	maxScore += 20

	// Documentation coverage (weight: 30 points)
	log.Printf("\nDocumentation Coverage (30 points max):")
	if data.HasDocsDirectory {
		// Docs directory exists: 15 points
		log.Printf("  Docs directory exists: +15 points")
		score += 15
		// Number of files in docs: 15 points
		docsScore := common.ComputeScore(data.DocsFileCount, [4]int64{1, 5, 15, 30}, common.BiggerIsBetter)
		docsPoints := docsScore * 3
		log.Printf("  Docs files score: %d/5 → %d/15 points", docsScore, docsPoints)
		score += docsPoints
	} else {
		log.Printf("  No docs directory: 0/30 points")
	}
	maxScore += 30

	// Accessibility (weight: 14 points)
	log.Printf("\nAccessibility (14 points max):")
	accessibilityPoints := int64(0)
	if data.HasContributing {
		log.Printf("  Has CONTRIBUTING.md: +8 points")
		score += 8
		accessibilityPoints += 8
	}
	if data.HasIssueTemplates {
		log.Printf("  Has issue templates: +6 points")
		score += 6
		accessibilityPoints += 6
	}
	if accessibilityPoints == 0 {
		log.Printf("  No accessibility features: 0/14 points")
	}
	maxScore += 14

	// Multi-language support (weight: 10 points)
	log.Printf("\nMulti-language Support (10 points max):")
	multiLangScore := common.ComputeScore(data.MultiLanguageCount, [4]int64{1, 2, 3, 5}, common.BiggerIsBetter)
	multiLangPoints := multiLangScore * 2
	log.Printf("  Multi-language score: %d/5 → %d/10 points", multiLangScore, multiLangPoints)
	score += multiLangPoints
	maxScore += 10

	// Convert to percentage (0-100) then map to 1-5 scale using thresholds
	percentage := (score * 100) / maxScore
	finalScore := common.ComputeScore(percentage, threshold, common.BiggerIsBetter)

	log.Printf("\n--- Final Calculation ---")
	log.Printf("Total points: %d/%d", score, maxScore)
	log.Printf("Percentage: %d%%", percentage)
	log.Printf("Thresholds: [%d%%, %d%%, %d%%, %d%%] → Scores [1, 2, 3, 4, 5]", threshold[0], threshold[1], threshold[2], threshold[3])
	log.Printf("Final documentation score: %d/5", finalScore)

	return finalScore
}
