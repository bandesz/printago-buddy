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

// overflowAgeStandIn is finite and strictly above every finite Weight result.
// math.Pow(2, x) overflows to +Inf before it can reach MaxFloat64.
const overflowAgeStandIn = math.MaxFloat64

// finiteAgeStandIn replaces every non-finite weight with overflowAgeStandIn.
// All overflowing ages therefore compare equal, and heavier than any finite age.
func finiteAgeStandIn(weight float64) float64 {
	if math.IsNaN(weight) || math.IsInf(weight, 0) {
		return overflowAgeStandIn
	}
	return weight
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
