package tech

// TechData contains all data fetched from Lizard
type TechData struct {
	// Production code metrics
	LinesOfCode             int64
	Functions               int64
	HighComplexityFunctions int64 // Functions with CCN > 15

	// Test code metrics
	TestLinesOfCode int64
	TestFunctions   int64

	// Derived metric
	TestRatio float64 // testLOC / productionLOC
}

// TechThresholds contains thresholds for all tech scopes
type TechThresholds struct {
	Size        [4]int64
	Complexity  [4]int64   // Percentage thresholds for high-complexity functions
	TestCoverage [4]float64 // Test/production code ratio thresholds
}
