package tech

// TechData contains all data fetched from Lizard
type TechData struct {
	LinesOfCode             int64
	Functions               int64
	HighComplexityFunctions int64 // Functions with CCN > 15
}

// TechThresholds contains thresholds for all tech scopes
type TechThresholds struct {
	Size       [4]int64
	Complexity [4]int64 // Percentage thresholds for high-complexity functions
}
