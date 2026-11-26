package smartqueue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// SignatureJob is a managed job or a merged-plate child in the signature input.
type SignatureJob struct {
	ID           string
	Priority     int
	CreatedAt    string
	RequiredTags map[string]string
}

// SignatureSlot is one printer slot's loaded material and variant.
type SignatureSlot struct {
	MaterialID string
	VariantID  string
}

// SignaturePrinter is one printer in the signature input.
// Checks are that printer's eligibility checks. The check coded ready is the ready bit.
type SignaturePrinter struct {
	ID     string
	Tags   []string
	Slots  []SignatureSlot
	Checks []Check
}

// SignatureInput is the snapshot covered by the assignment and dispatch signatures.
type SignatureInput struct {
	Jobs              []SignatureJob
	Printers          []SignaturePrinter
	PrioritizedJobIDs []string
}

// AssignmentSignature digests the inputs that change job-to-printer pairs.
// printer.id, buddy-hold, and each printer's ready bit are left out.
// Map and slice order do not change the digest.
func AssignmentSignature(in SignatureInput) string {
	return signatureDigest(assignmentDoc(in))
}

// DispatchSignature digests the assignment inputs plus each eligible printer's ready bit.
func DispatchSignature(in SignatureInput) string {
	doc := assignmentDoc(in)
	doc.Ready = readyBits(in.Printers)
	return signatureDigest(doc)
}

type digestDoc struct {
	Jobs        []digestJob     `json:"jobs"`
	Printers    []digestPrinter `json:"printers"`
	Prioritized []string        `json:"prioritized,omitempty"`
	Ready       []digestReady   `json:"ready,omitempty"`
}

type digestJob struct {
	ID           string            `json:"id"`
	Priority     int               `json:"priority"`
	CreatedAt    string            `json:"createdAt"`
	RequiredTags map[string]string `json:"requiredTags,omitempty"`
}

type digestPrinter struct {
	ID     string        `json:"id"`
	Tags   []string      `json:"tags,omitempty"`
	Slots  []digestSlot  `json:"slots,omitempty"`
	Checks []digestCheck `json:"checks,omitempty"`
}

type digestSlot struct {
	MaterialID string `json:"materialId"`
	VariantID  string `json:"variantId"`
}

type digestCheck struct {
	Code      string             `json:"code"`
	Passed    bool               `json:"passed"`
	Unmatched []digestComparison `json:"unmatched,omitempty"`
}

type digestComparison struct {
	Key         string `json:"key"`
	Requirement string `json:"requirement"`
}

type digestReady struct {
	ID    string `json:"id"`
	Ready bool   `json:"ready"`
}

func assignmentDoc(in SignatureInput) digestDoc {
	return digestDoc{
		Jobs:        digestJobs(in.Jobs),
		Printers:    digestPrinters(in.Printers),
		Prioritized: canonicalIDs(in.PrioritizedJobIDs),
	}
}

func signatureDigest(doc digestDoc) string {
	buf, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

func digestJobs(jobs []SignatureJob) []digestJob {
	out := make([]digestJob, len(jobs))
	for i, job := range jobs {
		out[i] = digestJob{
			ID:           job.ID,
			Priority:     job.Priority,
			CreatedAt:    job.CreatedAt,
			RequiredTags: withoutReservationTags(job.RequiredTags),
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

func digestPrinters(printers []SignaturePrinter) []digestPrinter {
	out := make([]digestPrinter, len(printers))
	for i, printer := range printers {
		out[i] = digestPrinter{
			ID:     printer.ID,
			Tags:   sortedCopy(printer.Tags),
			Slots:  digestSlots(printer.Slots),
			Checks: eligibilityChecks(printer.Checks),
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func digestSlots(slots []SignatureSlot) []digestSlot {
	if len(slots) == 0 {
		return nil
	}
	out := make([]digestSlot, len(slots))
	for i, slot := range slots {
		out[i] = digestSlot(slot)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MaterialID != out[j].MaterialID {
			return out[i].MaterialID < out[j].MaterialID
		}
		return out[i].VariantID < out[j].VariantID
	})
	return out
}

// eligibilityChecks keeps checks that change which printer a job can use.
// ready, staggered-start-hold, and subscription-slots stay out.
// tags-match is included only for unmatched comparisons other than printer.id and buddy-hold.
func eligibilityChecks(checks []Check) []digestCheck {
	out := make([]digestCheck, 0, len(checks))
	for _, check := range checks {
		digested, ok := digestEligibilityCheck(check)
		if !ok {
			continue
		}
		out = append(out, digested)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		if out[i].Passed != out[j].Passed {
			return !out[i].Passed && out[j].Passed
		}
		return digestComparisonsLess(out[i].Unmatched, out[j].Unmatched)
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func digestEligibilityCheck(check Check) (digestCheck, bool) {
	if nonBlockingFailure(check.Code) {
		return digestCheck{}, false
	}
	if check.Code == "tags-match" {
		unmatched := blockingComparisons(check.Comparisons)
		if len(unmatched) == 0 {
			return digestCheck{}, false
		}
		return digestCheck{Code: check.Code, Passed: false, Unmatched: unmatched}, true
	}
	return digestCheck{Code: check.Code, Passed: check.Passed}, true
}

func blockingComparisons(comparisons []Comparison) []digestComparison {
	out := make([]digestComparison, 0, len(comparisons))
	for _, cmp := range comparisons {
		if cmp.Matched || reservationKey(cmp.Key) {
			continue
		}
		out = append(out, digestComparison{Key: cmp.Key, Requirement: cmp.Requirement})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Requirement < out[j].Requirement
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func digestComparisonsLess(a, b []digestComparison) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i].Key != b[i].Key {
			return a[i].Key < b[i].Key
		}
		if a[i].Requirement != b[i].Requirement {
			return a[i].Requirement < b[i].Requirement
		}
	}
	return len(a) < len(b)
}

func readyBits(printers []SignaturePrinter) []digestReady {
	out := make([]digestReady, 0, len(printers))
	for _, printer := range printers {
		if !signatureEligible(printer.Checks) {
			continue
		}
		out = append(out, digestReady{ID: printer.ID, Ready: readyBit(printer.Checks)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func signatureEligible(checks []Check) bool {
	for _, check := range checks {
		if check.Passed {
			continue
		}
		if !verdictEligible(Verdict{FirstFailedCheck: check.Code}) {
			return false
		}
	}
	return true
}

func readyBit(checks []Check) bool {
	found := false
	for _, check := range checks {
		if check.Code != "ready" {
			continue
		}
		found = true
		if !check.Passed {
			return false
		}
	}
	return found
}

func canonicalIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := append([]string(nil), ids...)
	sort.Strings(out)
	n := 0
	for _, id := range out {
		if n > 0 && out[n-1] == id {
			continue
		}
		out[n] = id
		n++
	}
	return out[:n]
}

func sortedCopy(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := append([]string(nil), tags...)
	sort.Strings(out)
	return out
}

func withoutReservationTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		if key == printerIDKey {
			continue
		}
		if key == userTagsKey {
			value = withoutParkTag(value)
			if value == "" {
				continue
			}
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func withoutParkTag(raw string) string {
	list, err := parseAndList(raw)
	if err != nil {
		return raw
	}
	formatted, ok := formatAndList(removeTag(list, parkTag))
	if !ok {
		return ""
	}
	return formatted
}
