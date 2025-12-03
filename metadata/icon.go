package metadata

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/go-github/v76/github"
)

// GetIconURL returns an icon URL for the given project, falling back to devicons if needed
func GetIconURL(ctx context.Context, githubClient *github.Client, owner, repo string, language string) (string, error) {
	projectName := strings.ToLower(repo)

	// Try simple-icons first
	iconURL, err := getSimpleIcon(ctx, githubClient, projectName)
	if err == nil && iconURL != "" {
		return iconURL, nil
	}

	// Fallback to devicons
	return getDevicon(ctx, githubClient, projectName, language)
}

// getSimpleIcon tries to find an icon from simple-icons
func getSimpleIcon(ctx context.Context, githubClient *github.Client, projectName string) (string, error) {
	// Get slugs.md content from simple-icons repository
	fileContent, _, _, err := githubClient.Repositories.GetContents(
		ctx,
		"simple-icons",
		"simple-icons",
		"slugs.md",
		&github.RepositoryContentGetOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to fetch simple-icons slugs: %w", err)
	}

	if fileContent == nil || fileContent.Content == nil {
		return "", fmt.Errorf("empty slugs.md file")
	}

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode slugs.md: %w", err)
	}

	// Parse slugs and look for a match
	lines := strings.Split(string(content), "\n")
	projectNameNormalized := normalizeProjectName(projectName)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// slugs.md format is just one slug per line
		slug := strings.TrimSpace(line)
		slugNormalized := normalizeProjectName(slug)

		if slugNormalized == projectNameNormalized {
			return fmt.Sprintf("https://cdn.simpleicons.org/%s", slug), nil
		}
	}

	return "", fmt.Errorf("no match found in simple-icons")
}

// getDevicon tries to find an icon from devicons
func getDevicon(ctx context.Context, githubClient *github.Client, projectName string, language string) (string, error) {
	// Get list of icon directories from devicons repository
	_, dirContents, _, err := githubClient.Repositories.GetContents(
		ctx,
		"devicons",
		"devicon",
		"icons",
		&github.RepositoryContentGetOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to fetch devicons list: %w", err)
	}

	projectNameNormalized := normalizeProjectName(projectName)
	var availableIcons []string

	for _, item := range dirContents {
		if item.Type != nil && *item.Type == "dir" && item.Name != nil {
			availableIcons = append(availableIcons, *item.Name)
		}
	}

	// Try exact match first
	for _, iconName := range availableIcons {
		if normalizeProjectName(iconName) == projectNameNormalized {
			return fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", iconName, iconName), nil
		}
	}

	// Try language match
	if language != "" {
		languageNormalized := normalizeProjectName(language)
		for _, iconName := range availableIcons {
			if normalizeProjectName(iconName) == languageNormalized {
				return fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", iconName, iconName), nil
			}
		}
	}

	// Try partial matches or related icons
	// For terminal/CLI projects, use bash
	if strings.Contains(projectNameNormalized, "terminal") ||
		strings.Contains(projectNameNormalized, "cli") ||
		strings.Contains(projectNameNormalized, "shell") {
		for _, iconName := range availableIcons {
			if iconName == "bash" {
				return fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", iconName, iconName), nil
			}
		}
	}

	// Default: return empty string if nothing matches
	return "", fmt.Errorf("no match found in devicons")
}

// normalizeProjectName normalizes a project name for comparison
func normalizeProjectName(name string) string {
	// Convert to lowercase and remove common suffixes/prefixes
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "the-")
	name = strings.TrimSuffix(name, ".js")
	name = strings.TrimSuffix(name, "-js")
	name = strings.TrimSuffix(name, ".py")
	name = strings.TrimSuffix(name, "-py")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}
