package smartqueue_test

import (
	"math"
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestHeavyJobDoesNotTakeTheOnlyPrinterOfALighterJob(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "H", Priority: 100, Weight: 4},
		{ID: "L", Priority: 100, Weight: 1},
	}
	edges := []smartqueue.Edge{
		{JobID: "H", PrinterID: "A"},
		{JobID: "H", PrinterID: "B"},
		{JobID: "L", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["L"] != "A" || got["H"] != "B" {
		t.Errorf("assignment = %v, want L on A and H on B", got)
	}
}

func TestHeavyJobOverflowingAgeDoesNotTakeTheOnlyPrinterOfALighterJob(t *testing.T) {
	heavy := overflowingAgeWeight(t)
	jobs := []smartqueue.Job{
		{ID: "H", Priority: 100, Weight: heavy},
		{ID: "L", Priority: 100, Weight: 1},
	}
	edges := []smartqueue.Edge{
		{JobID: "H", PrinterID: "A"},
		{JobID: "H", PrinterID: "B"},
		{JobID: "L", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["L"] != "A" || got["H"] != "B" {
		t.Errorf("assignment = %v, want L on A and H on B", got)
	}
}

func TestLaneAssignsFreePrinterWithPositiveWeight(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "J", Priority: 100, Weight: 1},
	}
	edges := []smartqueue.Edge{
		{JobID: "J", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["J"] != "A" {
		t.Errorf("assignment = %v, want J→A", got)
	}
}

func TestLaneOverflowingAgeWeightGetsFreePrinter(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "J", Priority: 100, Weight: overflowingAgeWeight(t)},
	}
	edges := []smartqueue.Edge{
		{JobID: "J", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["J"] != "A" {
		t.Errorf("assignment = %v, want J→A", got)
	}
}

func TestLaneJobWithNoEdgeStaysUnmatched(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "orphan", Priority: 100, Weight: 4},
		{ID: "connected", Priority: 100, Weight: 1},
	}
	edges := []smartqueue.Edge{
		{JobID: "connected", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if _, ok := got["orphan"]; ok {
		t.Errorf("orphan assigned %q, want unmatched", got["orphan"])
	}
	if got["connected"] != "A" {
		t.Errorf("connected assigned %q, want A", got["connected"])
	}
}

func TestLaneNormalJobKeepsPrinterALowJobCanUse(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "normal", Priority: 100, Weight: 1},
		{ID: "low", Priority: 1000, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "normal", PrinterID: "A"},
		{JobID: "low", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["normal"] != "A" {
		t.Errorf("normal assigned %q, want A", got["normal"])
	}
	if _, ok := got["low"]; ok {
		t.Errorf("low assigned %q, want unmatched", got["low"])
	}
}

func TestLaneOverflowingNormalJobDoesNotLeaveSharedPrinterToLowJob(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "normal", Priority: 100, Weight: overflowingAgeWeight(t)},
		{ID: "low", Priority: 1000, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "normal", PrinterID: "A"},
		{JobID: "low", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["normal"] != "A" {
		t.Errorf("normal assigned %q, want A", got["normal"])
	}
	if _, ok := got["low"]; ok {
		t.Errorf("low assigned %q, want unmatched", got["low"])
	}
}

func TestLanePriorityOtherThan1000IsNormal(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "other", Priority: 200, Weight: 1},
		{ID: "low", Priority: 1000, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "other", PrinterID: "A"},
		{JobID: "low", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["other"] != "A" {
		t.Errorf("other assigned %q, want A", got["other"])
	}
	if _, ok := got["low"]; ok {
		t.Errorf("low assigned %q, want unmatched", got["low"])
	}
}

func TestLaneAssignsLowJobWhenNormalCannotUsePrinter(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "normal", Priority: 100, Weight: 1},
		{ID: "low", Priority: 1000, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "normal", PrinterID: "A"},
		{JobID: "low", PrinterID: "B"},
	}

	got := smartqueue.Assign(jobs, edges, nil)

	if got["normal"] != "A" {
		t.Errorf("normal assigned %q, want A", got["normal"])
	}
	if got["low"] != "B" {
		t.Errorf("low assigned %q, want B", got["low"])
	}
}

func TestPrioritizeNormalJobBeforeUnprioritized(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "marked", Priority: 100, Weight: 1},
		{ID: "plain", Priority: 100, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "marked", PrinterID: "A"},
		{JobID: "plain", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, map[string]struct{}{"marked": {}})

	if got["marked"] != "A" {
		t.Errorf("marked assigned %q, want A", got["marked"])
	}
	if _, ok := got["plain"]; ok {
		t.Errorf("plain assigned %q, want unmatched", got["plain"])
	}
}

func TestPrioritizeLowJobDoesNotTakePrinterANormalJobCanUse(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "normal", Priority: 100, Weight: 1},
		{ID: "low", Priority: 1000, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "normal", PrinterID: "A"},
		{JobID: "low", PrinterID: "A"},
	}

	got := smartqueue.Assign(jobs, edges, map[string]struct{}{"low": {}})

	if got["normal"] != "A" {
		t.Errorf("normal assigned %q, want A", got["normal"])
	}
	if _, ok := got["low"]; ok {
		t.Errorf("low assigned %q, want unmatched", got["low"])
	}
}

func TestPrioritizeHeavierNormalJobGetsContestedPrinter(t *testing.T) {
	jobs := []smartqueue.Job{
		{ID: "light", Priority: 100, Weight: 1},
		{ID: "heavy", Priority: 100, Weight: 8},
	}
	edges := []smartqueue.Edge{
		{JobID: "light", PrinterID: "A"},
		{JobID: "heavy", PrinterID: "A"},
	}
	prioritized := map[string]struct{}{
		"light": {},
		"heavy": {},
	}

	got := smartqueue.Assign(jobs, edges, prioritized)

	if got["heavy"] != "A" {
		t.Errorf("heavy assigned %q, want A", got["heavy"])
	}
	if _, ok := got["light"]; ok {
		t.Errorf("light assigned %q, want unmatched", got["light"])
	}
}

func overflowingAgeWeight(t *testing.T) float64 {
	t.Helper()
	weight := smartqueue.Weight(1024*time.Hour, time.Hour)
	if !math.IsInf(weight, 1) {
		t.Fatalf("Weight(1024 half-lives) = %v, want +Inf", weight)
	}
	return weight
}
