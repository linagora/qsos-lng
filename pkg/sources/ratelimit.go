package sources

import "github.com/linagora/qsos-lng/pkg/ratelimit"

// handleRateLimit is a convenience wrapper for ratelimit.HandleGitHub
func handleRateLimit(err error) bool {
	return ratelimit.HandleGitHub(err)
}
