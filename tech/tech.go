package tech

import "github.com/linagora/qsos-lng/common"

// ComputeAll computes all tech scores at once
func ComputeAll(data *TechData, thresholds *TechThresholds) *common.TechScores {
	return &common.TechScores{
		Size:                 ComputeSize(data, thresholds.Size),
		CyclomaticComplexity: ComputeCyclomaticComplexity(data, thresholds.CyclomaticComplexity),
		CognitiveComplexity:  ComputeCognitiveComplexity(data, thresholds.CognitiveComplexity),
		Duplication:          ComputeDuplication(data, thresholds.Duplication),
		CodeSmells:           ComputeCodeSmells(data, thresholds.CodeSmells),
	}
}

// ComputeSize calculates the size score based on lines of code
func ComputeSize(data *TechData, threshold [4]int64) int64 {
	return common.ComputeScore(data.LinesOfCode, threshold, common.SmallerIsBetter)
}

// ComputeCognitiveComplexity calculates the cognitive complexity score
// based on average cognitive complexity per function
func ComputeCognitiveComplexity(data *TechData, threshold [4]int64) int64 {
	// What is the average cognitive complexity per function?
	avg := int64(data.CognitiveComplexity / data.Functions)
	return common.ComputeScore(avg, threshold, common.SmallerIsBetter)
}

// ComputeCyclomaticComplexity calculates the cyclomatic complexity score
// based on the percentage of functions with high complexity (brain-overload)
func ComputeCyclomaticComplexity(data *TechData, threshold [4]int64) int64 {
	// What is the percentage of functions with high complexity?
	pct := int64(100.0 * data.BrainOverload / data.Functions)
	return common.ComputeScore(pct, threshold, common.SmallerIsBetter)
}

// ComputeDuplication calculates the duplication score based on duplication density percentage
func ComputeDuplication(data *TechData, threshold [4]int64) int64 {
	density := int64(data.DuplicationDensity)
	return common.ComputeScore(density, threshold, common.SmallerIsBetter)
}

// ComputeCodeSmells calculates the code smells score
// based on the average number of lines between code smells
func ComputeCodeSmells(data *TechData, threshold [4]int64) int64 {
	// What is the average number of lines between 2 code smells?
	avg := int64(data.LinesOfCode / data.CodeSmells)
	return common.ComputeScore(avg, threshold, common.BiggerIsBetter)
}
