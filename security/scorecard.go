package security

import (
	"log"

	"github.com/linagora/qsos-lng/common"
)

// ComputeAll computes all security scores (currently just scorecard)
func ComputeAll(data *SecurityData, weights map[string]int64) *common.SecurityScores {
	return &common.SecurityScores{
		ScoreCard: ComputeScorecard(data, weights),
	}
}

// ComputeScorecard calculates the security scorecard score
// using weighted average of selected checks
func ComputeScorecard(data *SecurityData, weights map[string]int64) int64 {
	var sum, divisor int64
	for name, weight := range weights {
		found := false
		for _, check := range data.Checks {
			if check.Name != name {
				continue
			}
			found = true
			if check.Score == -1 { // -1 means that it doesn't apply
				continue
			}
			sum += check.Score * weight
			divisor += weight
		}
		if !found {
			log.Fatalf("check %s not found in scorecard scores\n", name)
		}
	}
	return (sum + 1) / divisor / 2
}
