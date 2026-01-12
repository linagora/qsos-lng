package formula

import (
	"fmt"
)

// ComputeScore maps a value to a 1-5 score using four thresholds
// This matches the common.ComputeScore function from the existing codebase
func ComputeScore(value float64, thresholds [4]float64, direction Direction) float64 {
	var score int
	if direction == BiggerIsBetter {
		// Bigger values are better: score increases with value
		if value >= thresholds[3] {
			score = 5
		} else if value >= thresholds[2] {
			score = 4
		} else if value >= thresholds[1] {
			score = 3
		} else if value >= thresholds[0] {
			score = 2
		} else {
			score = 1
		}
	} else {
		// Smaller values are better: score decreases with value
		if value <= thresholds[0] {
			score = 5
		} else if value <= thresholds[1] {
			score = 4
		} else if value <= thresholds[2] {
			score = 3
		} else if value <= thresholds[3] {
			score = 2
		} else {
			score = 1
		}
	}
	return float64(score)
}

// WeightedAvg computes a weighted average
// values: slice of weighted values (already multiplied by weights)
// totalWeight: sum of all weights
func WeightedAvg(values []float64, totalWeight float64) (float64, error) {
	if totalWeight == 0 {
		return 0, fmt.Errorf("total weight cannot be zero")
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / totalWeight, nil
}

// If implements conditional logic: if condition then trueValue else falseValue
func If(condition bool, trueValue, falseValue float64) float64 {
	if condition {
		return trueValue
	}
	return falseValue
}
