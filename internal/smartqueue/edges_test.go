package smartqueue_test

import (
	"testing"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestEligibleReadyYieldsEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, false, "ready")}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", true)
}

func TestEligibleAvailabilityChecksYieldNoEdge(t *testing.T) {
	for _, code := range []string{"enabled", "online", "available", "in-maintenance"} {
		t.Run(code, func(t *testing.T) {
			// enabled is also matched, so a match does not restore a removed printer.
			matched := code == "enabled"
			verdicts := []smartqueue.Verdict{verdict("p1", matched, true, code)}

			assertEligible(t, verdicts, "p1", false)
			assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", false)
		})
	}
}

func TestEdgeMaterialMatchYieldsNoEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, true, "material-match")}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", false)
}

func TestEdgeNonMatchingChecksYieldNoEdge(t *testing.T) {
	for _, code := range []string{"profiles-configured", "slots-loaded", "routing-rules", "filament-profiles", "bed-fits"} {
		t.Run(code, func(t *testing.T) {
			verdicts := []smartqueue.Verdict{verdict("p1", false, true, code)}

			assertEligible(t, verdicts, "p1", true)
			assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", false)
		})
	}
}

func TestEdgeStaggeredStartAndSubscriptionYieldEdge(t *testing.T) {
	for _, code := range []string{"staggered-start-hold", "subscription-slots"} {
		t.Run(code, func(t *testing.T) {
			verdicts := []smartqueue.Verdict{verdict("p1", false, false, code)}

			assertEligible(t, verdicts, "p1", true)
			assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", true)
		})
	}
}

func TestEdgeMatchedTrueYieldsEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", true, false, "material-match")}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", true)
}

func TestEdgeEmptyFirstFailedCheckYieldsEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, false, "")}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "p1", true)
}

func TestEdgeTagsMatchReservationComparisons(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, false, "tags-match")}
	checks := map[string][]smartqueue.Check{
		"p1": {
			{Code: "enabled", Passed: true},
			{Code: "online", Passed: true},
			{Code: "available", Passed: true},
			{Code: "ready", Passed: false},
			{Code: "staggered-start-hold", Passed: false},
			{Code: "subscription-slots", Passed: false},
			{Code: "material-match", Passed: true},
			{Code: "bed-fits", Passed: true},
			{
				Code:   "tags-match",
				Passed: false,
				Comparisons: []smartqueue.Comparison{
					{Key: "printer.id", Requirement: "printer-9", Matched: false},
					{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
					{Key: "filament.type", Requirement: "PLA", Matched: true},
				},
			},
		},
	}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, checks), "job-1", "p1", true)
}

func TestEdgeTagsMatchOtherTagYieldsNoEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, true, "tags-match")}
	checks := map[string][]smartqueue.Check{
		"p1": {
			{Code: "ready", Passed: true},
			{Code: "material-match", Passed: true},
			{
				Code:   "tags-match",
				Passed: false,
				Comparisons: []smartqueue.Comparison{
					{Key: "printer.id", Requirement: "printer-9", Matched: false},
					{Key: "filament.type", Requirement: "PLA", Matched: false},
				},
			},
		},
	}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, checks), "job-1", "p1", false)
}

func TestEdgeTagsMatchOtherFailedCheckYieldsNoEdge(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("p1", false, false, "tags-match")}
	checks := map[string][]smartqueue.Check{
		"p1": {
			{Code: "ready", Passed: false},
			{Code: "material-match", Passed: false},
			{
				Code:   "tags-match",
				Passed: false,
				Comparisons: []smartqueue.Comparison{
					{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
				},
			},
		},
	}

	assertEligible(t, verdicts, "p1", true)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, checks), "job-1", "p1", false)
}

func TestEligibleAbsentPrinter(t *testing.T) {
	verdicts := []smartqueue.Verdict{verdict("other", true, true, "")}

	assertEligible(t, verdicts, "missing", false)
	assertEdge(t, smartqueue.Edges("job-1", verdicts, nil), "job-1", "missing", false)
}

func verdict(printerID string, matched, compatible bool, firstFailed string) smartqueue.Verdict {
	return smartqueue.Verdict{
		PrinterID:        printerID,
		Matched:          matched,
		Compatible:       compatible,
		FirstFailedCheck: firstFailed,
	}
}

func assertEligible(t *testing.T, verdicts []smartqueue.Verdict, printerID string, want bool) {
	t.Helper()
	if got := smartqueue.Eligible(verdicts, printerID); got != want {
		t.Errorf("Eligible(%q) = %v, want %v", printerID, got, want)
	}
}

func assertEdge(t *testing.T, edges []smartqueue.Edge, jobID, printerID string, want bool) {
	t.Helper()
	got := false
	for _, edge := range edges {
		if edge.JobID == jobID && edge.PrinterID == printerID {
			got = true
			break
		}
	}
	if got != want {
		t.Errorf("edge %s→%s = %v, want %v; edges=%v", jobID, printerID, got, want, edges)
	}
}
