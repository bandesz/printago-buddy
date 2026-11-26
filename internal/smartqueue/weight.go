package smartqueue

import (
	"math"
	"time"
)

// Weight is 2^(age/halfLife). Age below zero is floored to zero.
func Weight(age, halfLife time.Duration) float64 {
	if age < 0 {
		age = 0
	}
	return math.Pow(2, float64(age)/float64(halfLife))
}

// ScaleWeights divides each weight by the maximum so the largest becomes 1.
func ScaleWeights(weights []float64) []float64 {
	if len(weights) == 0 {
		return []float64{}
	}
	max := weights[0]
	for _, w := range weights[1:] {
		if w > max {
			max = w
		}
	}
	scaled := make([]float64, len(weights))
	for i, w := range weights {
		scaled[i] = w / max
	}
	return scaled
}
