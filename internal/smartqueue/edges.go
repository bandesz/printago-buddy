package smartqueue

// Verdict is one printer row from group matching.
// FirstFailedCheck is empty when that printer failed no check.
type Verdict struct {
	PrinterID        string
	Matched          bool
	Compatible       bool
	FirstFailedCheck string
}

// Check is one troubleshoot step for a printer.
type Check struct {
	Code        string
	Passed      bool
	Comparisons []Comparison
}

// Comparison is one tag requirement inside a troubleshoot check.
type Comparison struct {
	Key         string
	Requirement string
	Matched     bool
}

// Eligible reports whether printerID is a graph vertex.
// A printer missing from verdicts is not eligible.
// enabled, online, available, and in-maintenance remove the printer.
func Eligible(verdicts []Verdict, printerID string) bool {
	v, ok := findVerdict(verdicts, printerID)
	if !ok {
		return false
	}
	return verdictEligible(v)
}

// Edges returns one edge for each eligible printer the job can run on.
// checks maps a printer id to that printer's troubleshoot checks.
// Compatible does not create an edge.
func Edges(jobID string, verdicts []Verdict, checks map[string][]Check) []Edge {
	var out []Edge
	for _, v := range verdicts {
		if verdictEligible(v) && printerHasEdge(v, checks[v.PrinterID]) {
			out = append(out, Edge{JobID: jobID, PrinterID: v.PrinterID})
		}
	}
	return out
}

func findVerdict(verdicts []Verdict, printerID string) (Verdict, bool) {
	for _, v := range verdicts {
		if v.PrinterID == printerID {
			return v, true
		}
	}
	return Verdict{}, false
}

func verdictEligible(v Verdict) bool {
	switch v.FirstFailedCheck {
	case "enabled", "online", "available", "in-maintenance":
		return false
	default:
		return true
	}
}

func printerHasEdge(v Verdict, checks []Check) bool {
	switch {
	case v.Matched || v.FirstFailedCheck == "" || nonBlockingFailure(v.FirstFailedCheck):
		return true
	case v.FirstFailedCheck == "tags-match":
		return tagsMatchEdge(checks)
	default:
		return false
	}
}

// tagsMatchEdge is true when every failing comparison is a reservation
// comparison (printer.id or buddy-hold) and every other check passed or is
// ready, staggered-start-hold, or subscription-slots.
func tagsMatchEdge(checks []Check) bool {
	for _, check := range checks {
		if check.Code != "tags-match" && !check.Passed && !nonBlockingFailure(check.Code) {
			return false
		}
		for _, cmp := range check.Comparisons {
			if !cmp.Matched && !reservationKey(cmp.Key) {
				return false
			}
		}
	}
	return true
}

func nonBlockingFailure(code string) bool {
	switch code {
	case "ready", "staggered-start-hold", "subscription-slots":
		return true
	default:
		return false
	}
}

func reservationKey(key string) bool {
	return key == printerIDKey || key == parkTag
}
