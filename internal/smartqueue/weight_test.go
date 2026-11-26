package smartqueue_test

import (
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestWeight_zeroAge(t *testing.T) {
	got := smartqueue.Weight(0, time.Hour)
	if got != 1 {
		t.Errorf("Weight(0) = %v, want 1", got)
	}
}

func TestWeight_oneHalfLife(t *testing.T) {
	halfLife := time.Hour
	got := smartqueue.Weight(halfLife, halfLife)
	if got != 2 {
		t.Errorf("Weight(one half-life) = %v, want 2", got)
	}
}

func TestWeight_twoHalfLives(t *testing.T) {
	halfLife := time.Hour
	got := smartqueue.Weight(2*halfLife, halfLife)
	if got != 4 {
		t.Errorf("Weight(two half-lives) = %v, want 4", got)
	}
}

func TestWeight_negativeAge(t *testing.T) {
	got := smartqueue.Weight(-time.Minute, time.Hour)
	if got != 1 {
		t.Errorf("Weight(negative age) = %v, want 1", got)
	}
}

func TestScaleWeights_empty(t *testing.T) {
	got := smartqueue.ScaleWeights([]float64{})
	if len(got) != 0 {
		t.Errorf("ScaleWeights(empty) len = %d, want 0", len(got))
	}
}

func TestScaleWeights_dividesByMaximum(t *testing.T) {
	got := smartqueue.ScaleWeights([]float64{2, 4})
	want := []float64{0.5, 1}
	if len(got) != len(want) {
		t.Fatalf("ScaleWeights len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ScaleWeights[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
