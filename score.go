package main

import "time"

type ProjectScores struct {
	Community *CommunityScores
	Tech      *TechScores
}

type CommunityScores struct {
	Maturity     int
	Activity     int
	Popularity   int
	Contributors int
}

type TechScores struct {
	Size                 int
	CyclomaticComplexity int
	CognitiveComplexity  int
	Duplication          int
	CodeSmells           int
}

func ComputeScores(stats *ProjectStats) *ProjectScores {
	// TODO extract config
	scores := &ProjectScores{
		Community: &CommunityScores{
			Maturity:     computeMaturityScore(stats),
			Activity:     computeActivityScore(stats),
			Popularity:   computePopularityScore(stats),
			Contributors: computeContributorsScore(stats),
		},
		Tech: &TechScores{
			Size:                 computeSizeScore(stats),
			CyclomaticComplexity: computeCyclomaticComplexityScore(stats),
			CognitiveComplexity:  computeCognitiveComplexityScore(stats),
			Duplication:          computeDuplicationScore(stats),
			CodeSmells:           computeCodeSmellsScore(stats),
		},
	}
	return scores
}

func computeMaturityScore(stats *ProjectStats) int {
	elapsed := time.Since(stats.GitHub.FirstCommitDate)
	day := 24 * 60 * 60 * time.Second
	switch {
	case elapsed > 5*365*day:
		return 5
	case elapsed > 2*365*day:
		return 4
	case elapsed > 1*365*day:
		return 3
	case elapsed > 3*30*day:
		return 2
	default:
		return 1
	}
}

func computeActivityScore(stats *ProjectStats) int {
	elapsed := time.Since(stats.GitHub.LastCommitDate)
	day := 24 * 60 * 60 * time.Second
	switch {
	case elapsed < 30*day:
		return 5
	case elapsed < 6*30*day:
		return 4
	case elapsed < 1*365*day:
		return 3
	case elapsed < 2*365*day:
		return 2
	default:
		return 1
	}
}

func computePopularityScore(stats *ProjectStats) int {
	nb := stats.GitHub.Stars
	switch {
	case nb > 2_000:
		return 5
	case nb > 500:
		return 4
	case nb > 100:
		return 3
	case nb > 10:
		return 2
	default:
		return 1
	}
}

func computeContributorsScore(stats *ProjectStats) int {
	nb := stats.GitHub.ActiveContributors
	switch {
	case nb > 50:
		return 5
	case nb > 20:
		return 4
	case nb > 5:
		return 3
	case nb > 1:
		return 2
	default:
		return 1
	}
}

func computeSizeScore(stats *ProjectStats) int {
	nb := stats.Sonar.LinesOfCode
	switch {
	case nb < 1_000:
		return 5
	case nb < 10_000:
		return 4
	case nb < 100_000:
		return 3
	case nb < 1_000_000:
		return 2
	default:
		return 1
	}
}

func computeCyclomaticComplexityScore(stats *ProjectStats) int {
	nb := stats.Sonar.CyclomaticComplexity / stats.Sonar.Functions
	switch {
	case nb < 10:
		return 5
	case nb < 20:
		return 4
	case nb < 30:
		return 3
	case nb < 50:
		return 2
	default:
		return 1
	}
}

func computeCognitiveComplexityScore(stats *ProjectStats) int {
	nb := stats.Sonar.CognitiveComplexity / stats.Sonar.Functions
	switch {
	case nb < 10:
		return 5
	case nb < 20:
		return 4
	case nb < 30:
		return 3
	case nb < 50:
		return 2
	default:
		return 1
	}
}

func computeDuplicationScore(stats *ProjectStats) int {
	nb := stats.Sonar.DuplicationDensity
	switch {
	case nb < 3.0:
		return 5
	case nb < 5.0:
		return 4
	case nb < 10.0:
		return 3
	case nb < 20.0:
		return 2
	default:
		return 1
	}
}

func computeCodeSmellsScore(stats *ProjectStats) int {
	nb := stats.Sonar.LinesOfCode / stats.Sonar.CodeSmells
	switch {
	case nb < 50:
		return 5
	case nb < 200:
		return 4
	case nb < 500:
		return 3
	case nb < 1000:
		return 2
	default:
		return 1
	}
}
