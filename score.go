package main

import (
	"log"
	"time"
)

type Thresholds struct {
	Community *CommunityThreshold
	Tech      *TechThreshold
}

type CommunityThreshold struct {
	Maturity     [4]int64
	Activity     [4]int64
	Popularity   [4]int64
	Contributors [4]int64
}

type TechThreshold struct {
	Size                 [4]int64
	CyclomaticComplexity [4]int64
	CognitiveComplexity  [4]int64
	Duplication          [4]int64
	CodeSmells           [4]int64
}

type Weights struct {
	ScoreCard map[string]int64
}

type ProjectScores struct {
	Community *CommunityScores
	Tech      *TechScores
	Security  *SecurityScores
}

type CommunityScores struct {
	Maturity     int64
	Activity     int64
	Popularity   int64
	Contributors int64
}

type TechScores struct {
	Size                 int64
	CyclomaticComplexity int64
	CognitiveComplexity  int64
	Duplication          int64
	CodeSmells           int64
}

type SecurityScores struct {
	ScoreCard int64
}

func ComputeScores(stats *ProjectStats, thresholds *Thresholds, weights *Weights) *ProjectScores {
	scores := &ProjectScores{
		Community: &CommunityScores{
			Maturity:     computeMaturityScore(stats, thresholds),
			Activity:     computeActivityScore(stats, thresholds),
			Popularity:   computePopularityScore(stats, thresholds),
			Contributors: computeContributorsScore(stats, thresholds),
		},
		Tech: &TechScores{
			Size:                 computeSizeScore(stats, thresholds),
			CyclomaticComplexity: computeCyclomaticComplexityScore(stats, thresholds),
			CognitiveComplexity:  computeCognitiveComplexityScore(stats, thresholds),
			Duplication:          computeDuplicationScore(stats, thresholds),
			CodeSmells:           computeCodeSmellsScore(stats, thresholds),
		},
		Security: &SecurityScores{
			ScoreCard: computeScoreCardScore(stats, weights),
		},
	}
	return scores
}

func computeMaturityScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	elapsed := time.Since(stats.GitHub.FirstCommitDate).Nanoseconds()
	return computeScore(elapsed, thresholds.Community.Maturity, BiggerIsBetter)
}

func computeActivityScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	elapsed := time.Since(stats.GitHub.LastCommitDate).Nanoseconds()
	return computeScore(elapsed, thresholds.Community.Activity, SmallerIsBetter)
}

func computePopularityScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	nb := stats.GitHub.Stars
	return computeScore(nb, thresholds.Community.Popularity, BiggerIsBetter)
}

func computeContributorsScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	nb := stats.GitHub.ActiveContributors
	return computeScore(nb, thresholds.Community.Contributors, BiggerIsBetter)
}

func computeSizeScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	nb := stats.Sonar.LinesOfCode
	return computeScore(nb, thresholds.Tech.Size, SmallerIsBetter)
}

func computeCyclomaticComplexityScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	// What is the percentage of functions with high complexity?
	pct := int64(100.0 * stats.Sonar.BrainOverload / stats.Sonar.Functions)
	return computeScore(pct, thresholds.Tech.CyclomaticComplexity, SmallerIsBetter)
}

func computeCognitiveComplexityScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	// What is the average cognitive complexity per function?
	nb := int64(stats.Sonar.CognitiveComplexity / stats.Sonar.Functions)
	return computeScore(nb, thresholds.Tech.CognitiveComplexity, SmallerIsBetter)
}

func computeDuplicationScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	nb := int64(stats.Sonar.DuplicationDensity)
	return computeScore(nb, thresholds.Tech.Duplication, SmallerIsBetter)
}

func computeCodeSmellsScore(stats *ProjectStats, thresholds *Thresholds) int64 {
	// What is the average number of lines between 2 code smells?
	nb := int64(stats.Sonar.LinesOfCode / stats.Sonar.CodeSmells)
	return computeScore(nb, thresholds.Tech.CodeSmells, BiggerIsBetter)
}

type Direction bool

const (
	BiggerIsBetter  Direction = true
	SmallerIsBetter Direction = false
)

func computeScore(nb int64, thresholds [4]int64, direction Direction) int64 {
	scale := [5]int64{1, 2, 3, 4, 5}
	if direction == SmallerIsBetter {
		scale = [5]int64{5, 4, 3, 2, 1}
	}
	switch {
	case nb > thresholds[3]:
		return scale[4]
	case nb > thresholds[2]:
		return scale[3]
	case nb > thresholds[1]:
		return scale[2]
	case nb > thresholds[0]:
		return scale[1]
	default:
		return scale[0]
	}
}

func computeScoreCardScore(stats *ProjectStats, weights *Weights) int64 {
	var sum, divisor int64
	for name, weight := range weights.ScoreCard {
		found := false
		for _, check := range stats.ScoreCard.Checks {
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
