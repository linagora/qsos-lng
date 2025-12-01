package community

import (
	"time"

	"github.com/linagora/qsos-lng/common"
)

// ComputeAll computes all community scores at once
func ComputeAll(data *CommunityData, thresholds *CommunityThresholds) *common.CommunityScores {
	return &common.CommunityScores{
		Maturity:     ComputeMaturity(data, thresholds.Maturity),
		Activity:     ComputeActivity(data, thresholds.Activity),
		Popularity:   ComputePopularity(data, thresholds.Popularity),
		Contributors: ComputeContributors(data, thresholds.Contributors),
	}
}

// ComputeMaturity calculates the maturity score based on project age
func ComputeMaturity(data *CommunityData, threshold [4]int64) int64 {
	elapsed := time.Since(data.FirstCommitDate).Nanoseconds()
	return common.ComputeScore(elapsed, threshold, common.BiggerIsBetter)
}

// ComputeActivity calculates the activity score based on time since last commit
func ComputeActivity(data *CommunityData, threshold [4]int64) int64 {
	elapsed := time.Since(data.LastCommitDate).Nanoseconds()
	return common.ComputeScore(elapsed, threshold, common.SmallerIsBetter)
}

// ComputePopularity calculates the popularity score based on GitHub stars
func ComputePopularity(data *CommunityData, threshold [4]int64) int64 {
	return common.ComputeScore(data.Stars, threshold, common.BiggerIsBetter)
}

// ComputeContributors calculates the contributors score based on active contributors
func ComputeContributors(data *CommunityData, threshold [4]int64) int64 {
	return common.ComputeScore(data.ActiveContributors, threshold, common.BiggerIsBetter)
}
