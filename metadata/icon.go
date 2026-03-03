package metadata

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v76/github"
	"github.com/linagora/qsos-lng/pkg/ratelimit"
)

// iconCache holds cached icon data from GitHub
type iconCache struct {
	mu sync.RWMutex

	// simple-icons: normalized name -> original slug
	simpleIconsSlugs  map[string]string
	simpleIconsCached time.Time

	// devicons: list of available icon names
	deviconsNames  []string
	deviconsCached time.Time

	// selfhst-icons: list of available icon names
	selfhstIconsNames  []string
	selfhstIconsCached time.Time

	// Cache TTL (24 hours by default)
	ttl time.Duration
}

var cache = &iconCache{
	ttl: 24 * time.Hour,
}

// GetIconURL returns an icon URL for the given project, falling back to devicons if needed
func GetIconURL(ctx context.Context, githubClient *github.Client, owner, repo string, language string) (string, error) {
	projectName := strings.ToLower(repo)

	// Try selfhst-icons first
	iconURL, err := getSelfhstIcon(ctx, githubClient, projectName)
	if err == nil && iconURL != "" {
		return iconURL, nil
	}

	// Try simple-icons next
	iconURL, err = getSimpleIcon(ctx, githubClient, projectName)
	if err == nil && iconURL != "" {
		return iconURL, nil
	}

	// Fallback to devicons
	return getDevicon(ctx, githubClient, projectName, language)
}

// getSelfhstIcon tries to find an icon from selfhst/icons
func getSelfhstIcon(ctx context.Context, githubClient *github.Client, projectName string) (string, error) {
	icons, err := getSelfhstIconsNames(ctx, githubClient)
	if err != nil {
		return "", err
	}

	if iconName, ok := findSelfhstIconName(projectName, icons); ok {
		return fmt.Sprintf("https://raw.githubusercontent.com/selfhst/icons/main/svg/%s.svg", iconName), nil
	}

	return "", fmt.Errorf("no match found in selfhst/icons")
}

// getSelfhstIconsNames returns cached selfhst-icons names, fetching if needed
func getSelfhstIconsNames(ctx context.Context, githubClient *github.Client) ([]string, error) {
	cache.mu.RLock()
	if cache.selfhstIconsNames != nil && time.Since(cache.selfhstIconsCached) < cache.ttl {
		defer cache.mu.RUnlock()
		return cache.selfhstIconsNames, nil
	}
	cache.mu.RUnlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check after acquiring write lock
	if cache.selfhstIconsNames != nil && time.Since(cache.selfhstIconsCached) < cache.ttl {
		return cache.selfhstIconsNames, nil
	}

	log.Printf("      Fetching selfhst-icons list (cache miss)...\n")

	// Fetch icons directory from selfhst/icons repository
	var dirContents []*github.RepositoryContent
	for {
		var err error
		_, dirContents, _, err = githubClient.Repositories.GetContents(
			ctx,
			"selfhst",
			"icons",
			"svg",
			&github.RepositoryContentGetOptions{},
		)
		if err != nil {
			if ratelimit.HandleGitHub(err) {
				continue
			}
			return nil, fmt.Errorf("failed to fetch selfhst-icons list: %w", err)
		}
		break
	}

	var names []string
	for _, item := range dirContents {
		if item.Type != nil && *item.Type == "file" && item.Name != nil {
			name := strings.TrimSuffix(*item.Name, ".svg")
			names = append(names, name)
		}
	}

	cache.selfhstIconsNames = names
	cache.selfhstIconsCached = time.Now()
	log.Printf("      Cached %d selfhst-icons names", len(names))

	return names, nil
}

// getSimpleIcon tries to find an icon from simple-icons
func getSimpleIcon(ctx context.Context, githubClient *github.Client, projectName string) (string, error) {
	slugs, err := getSimpleIconsSlugs(ctx, githubClient)
	if err != nil {
		return "", err
	}

	projectNameNormalized := normalizeProjectName(projectName)
	if slug, ok := slugs[projectNameNormalized]; ok {
		return fmt.Sprintf("https://cdn.simpleicons.org/%s", slug), nil
	}

	return "", fmt.Errorf("no match found in simple-icons")
}

// getSimpleIconsSlugs returns cached simple-icons slugs, fetching if needed
func getSimpleIconsSlugs(ctx context.Context, githubClient *github.Client) (map[string]string, error) {
	cache.mu.RLock()
	if cache.simpleIconsSlugs != nil && time.Since(cache.simpleIconsCached) < cache.ttl {
		defer cache.mu.RUnlock()
		return cache.simpleIconsSlugs, nil
	}
	cache.mu.RUnlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check after acquiring write lock
	if cache.simpleIconsSlugs != nil && time.Since(cache.simpleIconsCached) < cache.ttl {
		return cache.simpleIconsSlugs, nil
	}

	log.Printf("      Fetching simple-icons slugs (cache miss)...\n")

	// Fetch slugs.md from simple-icons repository
	var fileContent *github.RepositoryContent
	for {
		var err error
		fileContent, _, _, err = githubClient.Repositories.GetContents(
			ctx,
			"simple-icons",
			"simple-icons",
			"slugs.md",
			&github.RepositoryContentGetOptions{},
		)
		if err != nil {
			if ratelimit.HandleGitHub(err) {
				continue
			}
			return nil, fmt.Errorf("failed to fetch simple-icons slugs: %w", err)
		}
		break
	}

	if fileContent == nil || fileContent.Content == nil {
		return nil, fmt.Errorf("empty slugs.md file")
	}

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode slugs.md: %w", err)
	}

	// Parse slugs into a map
	slugs := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		slug := strings.TrimSpace(line)
		normalized := normalizeProjectName(slug)
		slugs[normalized] = slug
	}

	cache.simpleIconsSlugs = slugs
	cache.simpleIconsCached = time.Now()
	log.Printf("      Cached %d simple-icons slugs", len(slugs))

	return slugs, nil
}

// getDevicon tries to find an icon from devicons
func getDevicon(ctx context.Context, githubClient *github.Client, projectName string, language string) (string, error) {
	icons, err := getDeviconsNames(ctx, githubClient)
	if err != nil {
		return "", err
	}

	projectNameNormalized := normalizeProjectName(projectName)

	// Try exact match first
	for _, iconName := range icons {
		if normalizeProjectName(iconName) == projectNameNormalized {
			return fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", iconName, iconName), nil
		}
	}

	// Try language match
	if language != "" {
		languageNormalized := normalizeProjectName(language)
		for _, iconName := range icons {
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
		for _, iconName := range icons {
			if iconName == "bash" {
				return fmt.Sprintf("https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/%s/%s-original.svg", iconName, iconName), nil
			}
		}
	}

	return "", fmt.Errorf("no match found in devicons")
}

// getDeviconsNames returns cached devicons names, fetching if needed
func getDeviconsNames(ctx context.Context, githubClient *github.Client) ([]string, error) {
	cache.mu.RLock()
	if cache.deviconsNames != nil && time.Since(cache.deviconsCached) < cache.ttl {
		defer cache.mu.RUnlock()
		return cache.deviconsNames, nil
	}
	cache.mu.RUnlock()

	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check after acquiring write lock
	if cache.deviconsNames != nil && time.Since(cache.deviconsCached) < cache.ttl {
		return cache.deviconsNames, nil
	}

	log.Printf("      Fetching devicons list (cache miss)...\n")

	// Fetch icons directory from devicons repository
	var dirContents []*github.RepositoryContent
	for {
		var err error
		_, dirContents, _, err = githubClient.Repositories.GetContents(
			ctx,
			"devicons",
			"devicon",
			"icons",
			&github.RepositoryContentGetOptions{},
		)
		if err != nil {
			if ratelimit.HandleGitHub(err) {
				continue
			}
			return nil, fmt.Errorf("failed to fetch devicons list: %w", err)
		}
		break
	}

	var names []string
	for _, item := range dirContents {
		if item.Type != nil && *item.Type == "dir" && item.Name != nil {
			names = append(names, *item.Name)
		}
	}

	cache.deviconsNames = names
	cache.deviconsCached = time.Now()
	log.Printf("      Cached %d devicons names", len(names))

	return names, nil
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

func findSelfhstIconName(projectName string, icons []string) (string, bool) {
	target := normalizeProjectName(projectName)
	for _, iconName := range icons {
		if normalizeProjectName(iconName) == target {
			return iconName, true
		}

		trimmed := trimIconVariant(iconName)
		if normalizeProjectName(trimmed) == target {
			return iconName, true
		}
	}
	return "", false
}

func trimIconVariant(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{"-light", "-dark"} {
		if len(lower) > len(suffix) && strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}
