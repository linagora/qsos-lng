package tech

// TechData contains all data fetched from SonarQube
type TechData struct {
	LinesOfCode          int64
	Functions            int64
	CodeSmells           int64
	BrainOverload        int64
	CyclomaticComplexity int64
	CognitiveComplexity  int64
	DuplicationDensity   float64
}

// TechThresholds contains thresholds for all tech scopes
type TechThresholds struct {
	Size                 [4]int64
	CyclomaticComplexity [4]int64
	CognitiveComplexity  [4]int64
	Duplication          [4]int64
	CodeSmells           [4]int64
}
