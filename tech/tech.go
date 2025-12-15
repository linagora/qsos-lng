package tech

import "github.com/linagora/qsos-lng/common"

// ComputeAll computes all tech scores at once
func ComputeAll(data *TechData, thresholds *TechThresholds) *common.TechScores {
	return &common.TechScores{
		Size:       ComputeSize(data, thresholds.Size),
		Complexity: ComputeComplexity(data, thresholds.Complexity),
	}
}

// ComputeSize calculates the size score based on lines of code
func ComputeSize(data *TechData, threshold [4]int64) int64 {
	return common.ComputeScore(data.LinesOfCode, threshold, common.SmallerIsBetter)
}

// ComputeComplexity calculates the complexity score
// based on the percentage of functions with high cyclomatic complexity (CCN > 15)
func ComputeComplexity(data *TechData, threshold [4]int64) int64 {
	if data.Functions == 0 {
		return 5 // No functions = perfect score
	}
	// Calculate percentage of high-complexity functions
	pct := int64(100 * data.HighComplexityFunctions / data.Functions)
	return common.ComputeScore(pct, threshold, common.SmallerIsBetter)
}
