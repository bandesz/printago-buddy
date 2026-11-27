package smartqueue_test

import (
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestAssignmentSignatureEqualInputs(t *testing.T) {
	base := sampleInput()
	base.PrioritizedJobIDs = []string{"job-b", "job-a"}

	reordered := sampleInput()
	reordered.Jobs[0], reordered.Jobs[1] = reordered.Jobs[1], reordered.Jobs[0]
	reordered.Jobs[0].RequiredTags = map[string]string{
		"filament.type": "PETG",
	}
	reordered.Jobs[1].RequiredTags = map[string]string{}
	reordered.Jobs[1].RequiredTags["user.tags"] = "&=pla"
	reordered.Jobs[1].RequiredTags["filament.type"] = "PLA"
	reordered.Jobs[1].RequiredTags["printer.modelName"] = "X1 Carbon"
	reordered.Jobs[1].RequiredTags["printer.nozzleDiameter"] = "0.4"
	reordered.Printers[0], reordered.Printers[1] = reordered.Printers[1], reordered.Printers[0]
	reordered.Printers[1].Tags = []string{"ams", "bambu"}
	reordered.Printers[1].Slots = []smartqueue.SignatureSlot{
		{MaterialID: "mat-9", VariantID: "var-9"},
		{MaterialID: "mat-1", VariantID: "var-1"},
	}
	reordered.Printers[1].Checks = []smartqueue.Check{
		{Code: "ready", Passed: true},
		{Code: "available", Passed: true},
		{Code: "enabled", Passed: true},
		{Code: "in-maintenance", Passed: true},
		{Code: "online", Passed: true},
	}
	reordered.PrioritizedJobIDs = []string{"job-a", "job-b", "job-a"}

	got := smartqueue.AssignmentSignature(base)
	if got == "" {
		t.Fatal("assignment signature is empty")
	}
	if got != smartqueue.AssignmentSignature(reordered) {
		t.Fatalf("assignment signatures differ for equal inputs")
	}
	dispatch := smartqueue.DispatchSignature(base)
	if dispatch == "" {
		t.Fatal("dispatch signature is empty")
	}
	if dispatch != smartqueue.DispatchSignature(reordered) {
		t.Fatalf("dispatch signatures differ for equal inputs")
	}
}

func TestAssignmentSignatureIgnoresPrinterIDAndBuddyHold(t *testing.T) {
	base := sampleInput()
	baseSig := smartqueue.AssignmentSignature(base)

	changed := sampleInput()
	changed.Jobs[0].RequiredTags["filament.type"] = "ABS"
	if baseSig == smartqueue.AssignmentSignature(changed) {
		t.Fatal("required tag change did not change the assignment signature")
	}

	for _, tags := range []map[string]string{
		{
			"filament.type":          "PLA",
			"printer.modelName":      "X1 Carbon",
			"printer.nozzleDiameter": "0.4",
			"user.tags":              "&=pla",
		},
		{
			"printer.id":             "printer-a",
			"filament.type":          "PLA",
			"printer.modelName":      "X1 Carbon",
			"printer.nozzleDiameter": "0.4",
			"user.tags":              "&=pla",
		},
		{
			"printer.id":             "printer-b",
			"filament.type":          "PLA",
			"printer.modelName":      "X1 Carbon",
			"printer.nozzleDiameter": "0.4",
			"user.tags":              "&=pla,buddy-hold",
		},
		{
			"filament.type":          "PLA",
			"printer.modelName":      "X1 Carbon",
			"printer.nozzleDiameter": "0.4",
			"user.tags":              "&=buddy-hold,pla",
		},
	} {
		in := sampleInput()
		in.Jobs[0].RequiredTags = tags
		if smartqueue.AssignmentSignature(in) != baseSig {
			t.Errorf("tags %#v changed the assignment signature", tags)
		}
	}

	parked := sampleInput()
	parked.Jobs[0].RequiredTags = map[string]string{
		"filament.type":          "PLA",
		"printer.modelName":      "X1 Carbon",
		"printer.nozzleDiameter": "0.4",
		"user.tags":              "&=a,buddy-hold,b",
	}
	cleared := sampleInput()
	cleared.Jobs[0].RequiredTags = map[string]string{
		"printer.id":             "printer-z",
		"filament.type":          "PLA",
		"printer.modelName":      "X1 Carbon",
		"printer.nozzleDiameter": "0.4",
		"user.tags":              "&=a,b",
	}
	if smartqueue.AssignmentSignature(parked) != smartqueue.AssignmentSignature(cleared) {
		t.Fatal("adding or removing buddy-hold changed the assignment signature")
	}

	onlyHold := sampleInput()
	onlyHold.Jobs[0].RequiredTags = map[string]string{
		"printer.id": "printer-a",
		"user.tags":  "&=buddy-hold",
	}
	noTags := sampleInput()
	noTags.Jobs[0].RequiredTags = nil
	if smartqueue.AssignmentSignature(onlyHold) != smartqueue.AssignmentSignature(noTags) {
		t.Fatal("removing printer.id and buddy-hold changed the assignment signature")
	}

	original := map[string]string{
		"printer.id": "printer-a",
		"user.tags":  "&=pla,buddy-hold",
	}
	in := sampleInput()
	in.Jobs[0].RequiredTags = original
	smartqueue.AssignmentSignature(in)
	if original["printer.id"] != "printer-a" || original["user.tags"] != "&=pla,buddy-hold" {
		t.Fatalf("signature mutated required tags: %#v", original)
	}
}

func TestAssignmentSignatureChanges(t *testing.T) {
	base := smartqueue.AssignmentSignature(sampleInput())
	tests := []struct {
		name   string
		mutate func(*smartqueue.SignatureInput)
	}{
		{
			name: "new job",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Jobs = append(in.Jobs, smartqueue.SignatureJob{
					ID:        "job-2",
					Priority:  100,
					CreatedAt: "2026-03-01T00:00:00Z",
				})
			},
		},
		{
			name: "missing job",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Jobs = in.Jobs[:1]
			},
		},
		{
			name: "priority",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Jobs[0].Priority = 200
			},
		},
		{
			name: "createdAt",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Jobs[0].CreatedAt = "2026-06-01T00:00:00Z"
			},
		},
		{
			name: "required tag",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Jobs[0].RequiredTags["filament.type"] = "ABS"
			},
		},
		{
			name: "slot material",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Printers[0].Slots[0].MaterialID = "mat-other"
			},
		},
		{
			name: "slot variant",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Printers[0].Slots[0].VariantID = "var-other"
			},
		},
		{
			name: "printer tag",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Printers[0].Tags = append(in.Printers[0].Tags, "hardened")
			},
		},
		{
			name: "printer",
			mutate: func(in *smartqueue.SignatureInput) {
				in.Printers = append(in.Printers, smartqueue.SignaturePrinter{ID: "printer-c"})
			},
		},
		{
			name: "online",
			mutate: func(in *smartqueue.SignatureInput) {
				setPassed(in, 0, "online", false)
			},
		},
		{
			name: "maintenance",
			mutate: func(in *smartqueue.SignatureInput) {
				setPassed(in, 0, "in-maintenance", false)
			},
		},
		{
			name: "enabled",
			mutate: func(in *smartqueue.SignatureInput) {
				setPassed(in, 0, "enabled", false)
			},
		},
		{
			name: "available",
			mutate: func(in *smartqueue.SignatureInput) {
				setPassed(in, 0, "available", false)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := sampleInput()
			tc.mutate(&in)
			if smartqueue.AssignmentSignature(in) == base {
				t.Fatal("assignment signature unchanged")
			}
		})
	}
}

func TestAssignmentSignaturePrioritizeSet(t *testing.T) {
	base := sampleInput()
	added := sampleInput()
	added.PrioritizedJobIDs = []string{"job-1"}
	if smartqueue.AssignmentSignature(base) == smartqueue.AssignmentSignature(added) {
		t.Fatal("adding a prioritize id did not change the assignment signature")
	}
}

func TestDispatchSignatureReadyOnly(t *testing.T) {
	base := sampleInput()
	flipped := sampleInput()
	setPassed(&flipped, 0, "ready", false)

	if smartqueue.AssignmentSignature(base) != smartqueue.AssignmentSignature(flipped) {
		t.Fatal("ready-only change changed the assignment signature")
	}
	if smartqueue.DispatchSignature(base) == smartqueue.DispatchSignature(flipped) {
		t.Fatal("ready-only change did not change the dispatch signature")
	}
}

func TestAssignmentSignatureEdgeCheckFlip(t *testing.T) {
	for _, code := range []string{
		"material-match",
		"slots-loaded",
		"bed-fits",
		"profiles-configured",
		"routing-rules",
		"filament-profiles",
	} {
		t.Run(code, func(t *testing.T) {
			passing := sampleInput()
			setPassed(&passing, 0, code, true)
			failing := sampleInput()
			setPassed(&failing, 0, code, false)
			if smartqueue.AssignmentSignature(passing) == smartqueue.AssignmentSignature(failing) {
				t.Fatalf("flipping %s did not change the assignment signature", code)
			}
		})
	}
}

func TestAssignmentSignatureTagsMatch(t *testing.T) {
	base := sampleInput()
	baseAssign := smartqueue.AssignmentSignature(base)

	blocking := withTagsMatch(false, []smartqueue.Comparison{
		{Key: "printer.id", Requirement: "printer-a", Matched: false},
		{Key: "printer.modelName", Requirement: "X1 Carbon", Matched: false},
		{Key: "filament.type", Requirement: "PLA", Matched: false},
	})
	blockingAssign := smartqueue.AssignmentSignature(blocking)
	if blockingAssign == baseAssign {
		t.Fatal("tags-match failure with filament.type did not change the assignment signature")
	}

	otherKey := withTagsMatch(false, []smartqueue.Comparison{
		{Key: "printer.modelName", Requirement: "X1 Carbon", Matched: false},
	})
	if smartqueue.AssignmentSignature(otherKey) == blockingAssign {
		t.Fatal("a different unmatched comparison key did not change the assignment signature")
	}

	otherRequirement := withTagsMatch(false, []smartqueue.Comparison{
		{Key: "filament.type", Requirement: "PETG", Matched: false},
	})
	if smartqueue.AssignmentSignature(otherRequirement) == blockingAssign {
		t.Fatal("a different unmatched comparison requirement did not change the assignment signature")
	}

	reordered := withTagsMatch(false, []smartqueue.Comparison{
		{Key: "filament.type", Requirement: "PLA", Matched: false},
		{Key: "printer.id", Requirement: "printer-a", Matched: false},
		{Key: "printer.modelName", Requirement: "X1 Carbon", Matched: false},
	})
	if smartqueue.AssignmentSignature(reordered) != blockingAssign {
		t.Fatal("comparison order changed the assignment signature")
	}

	withReservation := withTagsMatch(false, []smartqueue.Comparison{
		{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
		{Key: "filament.type", Requirement: "PLA", Matched: false},
		{Key: "printer.modelName", Requirement: "X1 Carbon", Matched: false},
		{Key: "printer.id", Requirement: "printer-b", Matched: false},
	})
	if smartqueue.AssignmentSignature(withReservation) != blockingAssign {
		t.Fatal("printer.id or buddy-hold changed a blocking tags-match signature")
	}

	passedFlag := withTagsMatch(true, []smartqueue.Comparison{
		{Key: "printer.id", Requirement: "printer-a", Matched: false},
		{Key: "filament.type", Requirement: "PLA", Matched: false},
		{Key: "printer.modelName", Requirement: "X1 Carbon", Matched: false},
	})
	if smartqueue.AssignmentSignature(passedFlag) != blockingAssign {
		t.Fatal("tags-match Passed flag changed the assignment signature")
	}

	for _, comparisons := range [][]smartqueue.Comparison{
		{{Key: "printer.id", Requirement: "printer-z", Matched: false}},
		{{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false}},
		{
			{Key: "printer.id", Requirement: "printer-z", Matched: false},
			{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
		},
		{
			{Key: "filament.type", Requirement: "PLA", Matched: true},
			{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
		},
	} {
		in := withTagsMatch(false, comparisons)
		if smartqueue.AssignmentSignature(in) != baseAssign {
			t.Errorf("tags-match comparisons %#v changed the assignment signature", comparisons)
		}
	}
}

func TestAssignmentSignatureIgnoresStaggeredStartAndSubscriptionSlots(t *testing.T) {
	for _, code := range []string{"staggered-start-hold", "subscription-slots"} {
		t.Run(code, func(t *testing.T) {
			passing := sampleInput()
			setPassed(&passing, 0, code, true)
			failing := sampleInput()
			setPassed(&failing, 0, code, false)
			if smartqueue.AssignmentSignature(passing) != smartqueue.AssignmentSignature(failing) {
				t.Fatalf("flipping %s changed the assignment signature", code)
			}
			if smartqueue.DispatchSignature(passing) != smartqueue.DispatchSignature(failing) {
				t.Fatalf("flipping %s changed the dispatch signature", code)
			}
		})
	}
}

func TestDispatchSignatureIgnoresIneligibleReady(t *testing.T) {
	eligible := sampleInput()
	eligibleOff := sampleInput()
	setPassed(&eligibleOff, 0, "ready", false)
	if smartqueue.DispatchSignature(eligible) == smartqueue.DispatchSignature(eligibleOff) {
		t.Fatal("eligible ready change did not change the dispatch signature")
	}

	blocked := sampleInput()
	setPassed(&blocked, 0, "online", false)
	blockedOff := sampleInput()
	setPassed(&blockedOff, 0, "online", false)
	setPassed(&blockedOff, 0, "ready", false)
	if smartqueue.AssignmentSignature(blocked) != smartqueue.AssignmentSignature(blockedOff) {
		t.Fatal("ineligible ready change changed the assignment signature")
	}
	if smartqueue.DispatchSignature(blocked) != smartqueue.DispatchSignature(blockedOff) {
		t.Fatal("ineligible ready change changed the dispatch signature")
	}
}

func sampleInput() smartqueue.SignatureInput {
	return smartqueue.SignatureInput{
		Jobs: []smartqueue.SignatureJob{
			{
				ID:        "job-1",
				Priority:  100,
				CreatedAt: "2026-01-02T03:04:05Z",
				RequiredTags: map[string]string{
					"printer.nozzleDiameter": "0.4",
					"printer.modelName":      "X1 Carbon",
					"filament.type":          "PLA",
					"user.tags":              "&=pla",
				},
			},
			{
				ID:        "child-1",
				Priority:  1000,
				CreatedAt: "2026-01-02T04:00:00Z",
				RequiredTags: map[string]string{
					"filament.type": "PETG",
				},
			},
		},
		Printers: []smartqueue.SignaturePrinter{
			{
				ID:   "printer-b",
				Tags: []string{"bambu", "ams"},
				Slots: []smartqueue.SignatureSlot{
					{MaterialID: "mat-1", VariantID: "var-1"},
					{MaterialID: "mat-9", VariantID: "var-9"},
				},
				Checks: []smartqueue.Check{
					{Code: "online", Passed: true},
					{Code: "in-maintenance", Passed: true},
					{Code: "enabled", Passed: true},
					{Code: "available", Passed: true},
					{Code: "ready", Passed: true},
				},
			},
			{
				ID:   "printer-a",
				Tags: []string{"bambu"},
				Slots: []smartqueue.SignatureSlot{
					{MaterialID: "mat-2", VariantID: "var-2"},
				},
				Checks: []smartqueue.Check{
					{Code: "online", Passed: true},
					{Code: "ready", Passed: false},
				},
			},
		},
	}
}

func withTagsMatch(passed bool, comparisons []smartqueue.Comparison) smartqueue.SignatureInput {
	in := sampleInput()
	in.Printers[0].Checks = append(in.Printers[0].Checks, smartqueue.Check{
		Code:        "tags-match",
		Passed:      passed,
		Comparisons: comparisons,
	})
	return in
}

func setPassed(in *smartqueue.SignatureInput, printer int, code string, passed bool) {
	checks := in.Printers[printer].Checks
	for i := range checks {
		if checks[i].Code == code {
			checks[i].Passed = passed
			return
		}
	}
	in.Printers[printer].Checks = append(in.Printers[printer].Checks, smartqueue.Check{Code: code, Passed: passed})
}

func TestAssignmentSignatureHalfLife(t *testing.T) {
	base := sampleInput()
	changed := sampleInput()
	changed.HalfLife = time.Hour
	if smartqueue.AssignmentSignature(base) == smartqueue.AssignmentSignature(changed) {
		t.Fatal("assignment signature ignored half-life")
	}
	same := sampleInput()
	same.HalfLife = 60 * time.Minute
	if smartqueue.AssignmentSignature(changed) != smartqueue.AssignmentSignature(same) {
		t.Fatal("1h and 60m produced different assignment signatures")
	}
}
