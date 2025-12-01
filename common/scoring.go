package common

type Direction bool

const (
	BiggerIsBetter  Direction = true
	SmallerIsBetter Direction = false
)

// ComputeScore calculates a 1-5 score based on value and thresholds
func ComputeScore(nb int64, thresholds [4]int64, direction Direction) int64 {
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
