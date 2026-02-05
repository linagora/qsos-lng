package ratelimit

import (
	"errors"
	"log"
	"time"

	"github.com/google/go-github/v76/github"
)

// HandleGitHub checks if the error is a GitHub rate limit error and sleeps until
// the rate limit resets. Returns true if the operation should be retried.
func HandleGitHub(err error) bool {
	var rateLimitErr *github.RateLimitError
	if errors.As(err, &rateLimitErr) {
		sleepUntilReset(rateLimitErr.Rate.Reset.Time, "primary")
		return true
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		if abuseErr.RetryAfter != nil {
			sleepDuration := *abuseErr.RetryAfter + 5*time.Second
			log.Printf("  GitHub abuse rate limit hit, sleeping for %v", sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			// Default to 1 minute if no RetryAfter provided
			log.Printf("  GitHub abuse rate limit hit, sleeping for 1 minute")
			time.Sleep(1 * time.Minute)
		}
		return true
	}

	return false
}

// sleepUntilReset sleeps until the rate limit reset time plus a small buffer
func sleepUntilReset(resetTime time.Time, limitType string) {
	sleepDuration := time.Until(resetTime) + 5*time.Second
	if sleepDuration > 0 {
		log.Printf("  GitHub %s rate limit exceeded, sleeping until %v (%v)",
			limitType, resetTime.Format(time.RFC3339), sleepDuration.Round(time.Second))
		time.Sleep(sleepDuration)
		log.Printf("  Resuming after rate limit sleep")
	}
}
