package smartqueue_test

import (
	"errors"
	"maps"
	"testing"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

type patchCall struct {
	id   string
	tags map[string]string
}

type patchLog struct {
	calls []patchCall
	fail  map[string]error
}

func (p *patchLog) patch(id string, tags map[string]string) error {
	p.calls = append(p.calls, patchCall{id: id, tags: maps.Clone(tags)})
	if err, ok := p.fail[id]; ok {
		return err
	}
	return nil
}

func assertPatches(t *testing.T, got []patchCall, want []patchCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("patches = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].id != want[i].id || !maps.Equal(got[i].tags, want[i].tags) {
			t.Fatalf("patch %d = {id:%s tags:%#v}, want {id:%s tags:%#v}", i, got[i].id, got[i].tags, want[i].id, want[i].tags)
		}
	}
}

func TestReserveAssignedPatchesOnlyWhenPinDiffers(t *testing.T) {
	plan := smartqueue.Plan{Rows: map[string]smartqueue.Row{
		"same":          {Status: smartqueue.StatusAssigned, PrinterID: "printer-1"},
		"wrong-printer": {Status: smartqueue.StatusAssigned, PrinterID: "printer-9"},
		"held":          {Status: smartqueue.StatusAssigned, PrinterID: "printer-2"},
		"unpinned":      {Status: smartqueue.StatusAssigned, PrinterID: "printer-3"},
	}}
	jobs := []smartqueue.ReserveJob{
		{
			ID: "same",
			RequiredTags: map[string]string{
				"printer.id":    "printer-1",
				"filament.type": "PLA",
				"user.tags":     "&=nozzle",
			},
		},
		{
			ID: "wrong-printer",
			RequiredTags: map[string]string{
				"printer.id":    "printer-1",
				"filament.type": "PLA",
				"user.tags":     "&=nozzle",
			},
		},
		{
			ID: "held",
			RequiredTags: map[string]string{
				"printer.id":    "printer-2",
				"filament.type": "PETG",
				"user.tags":     "&=nozzle,buddy-hold",
			},
		},
		{
			ID: "unpinned",
			RequiredTags: map[string]string{
				"filament.type": "ABS",
				"user.tags":     "&=buddy-hold",
			},
		},
	}
	before := cloneJobs(jobs)

	var log patchLog
	err := smartqueue.Reserve(plan, jobs, log.patch)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if !jobsEqual(jobs, before) {
		t.Fatalf("Reserve() mutated jobs: %#v", jobs)
	}
	assertPatches(t, log.calls, []patchCall{
		{id: "wrong-printer", tags: map[string]string{
			"printer.id":    "printer-9",
			"filament.type": "PLA",
			"user.tags":     "&=nozzle",
		}},
		{id: "held", tags: map[string]string{
			"printer.id":    "printer-2",
			"filament.type": "PETG",
			"user.tags":     "&=nozzle",
		}},
		{id: "unpinned", tags: map[string]string{
			"printer.id":    "printer-3",
			"filament.type": "ABS",
		}},
	})
}

func TestReserveUnassignedPatchesOnlyWhenParkDiffers(t *testing.T) {
	plan := smartqueue.Plan{Rows: map[string]smartqueue.Row{
		"waiting-parked":      {Status: smartqueue.StatusWaiting},
		"waiting-pinned":      {Status: smartqueue.StatusWaiting},
		"incompatible-open":   {Status: smartqueue.StatusIncompatible},
		"incompatible-parked": {Status: smartqueue.StatusIncompatible},
		"waiting-needs-hold":  {Status: smartqueue.StatusWaiting},
	}}
	jobs := []smartqueue.ReserveJob{
		{
			ID: "waiting-parked",
			RequiredTags: map[string]string{
				"filament.type": "PLA",
				"user.tags":     "&=a,buddy-hold",
			},
		},
		{
			ID: "waiting-pinned",
			RequiredTags: map[string]string{
				"printer.id":    "printer-1",
				"filament.type": "PLA",
				"user.tags":     "&=a,buddy-hold",
			},
		},
		{
			ID: "incompatible-open",
			RequiredTags: map[string]string{
				"filament.type": "PETG",
				"user.tags":     "&=a,b",
			},
		},
		{
			ID: "incompatible-parked",
			RequiredTags: map[string]string{
				"user.tags": "&=buddy-hold",
			},
		},
		{
			ID: "waiting-needs-hold",
			RequiredTags: map[string]string{
				"printer.modelName": "X1 Carbon",
			},
		},
	}

	var log patchLog
	err := smartqueue.Reserve(plan, jobs, log.patch)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	assertPatches(t, log.calls, []patchCall{
		{id: "waiting-pinned", tags: map[string]string{
			"filament.type": "PLA",
			"user.tags":     "&=a,buddy-hold",
		}},
		{id: "incompatible-open", tags: map[string]string{
			"filament.type": "PETG",
			"user.tags":     "&=a,b,buddy-hold",
		}},
		{id: "waiting-needs-hold", tags: map[string]string{
			"printer.modelName": "X1 Carbon",
			"user.tags":         "&=buddy-hold",
		}},
	})
}

func TestReserveSkipsMergedAndUnreadable(t *testing.T) {
	plan := smartqueue.Plan{Rows: map[string]smartqueue.Row{
		"child":    {Status: smartqueue.StatusMerged},
		"bad-time": {Status: smartqueue.StatusUnreadable},
		"assigned": {Status: smartqueue.StatusAssigned, PrinterID: "printer-1"},
	}}
	jobs := []smartqueue.ReserveJob{
		{
			ID: "child",
			RequiredTags: map[string]string{
				"printer.id": "printer-9",
				"user.tags":  "not-an-and-list",
			},
		},
		{
			ID: "bad-time",
			RequiredTags: map[string]string{
				"printer.id": "printer-8",
				"user.tags":  "|=or",
			},
		},
		{
			ID: "assigned",
			RequiredTags: map[string]string{
				"filament.type": "PLA",
			},
		},
	}

	var log patchLog
	err := smartqueue.Reserve(plan, jobs, log.patch)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	assertPatches(t, log.calls, []patchCall{
		{id: "assigned", tags: map[string]string{
			"printer.id":    "printer-1",
			"filament.type": "PLA",
		}},
	})
}

func TestReserveTagExpressionStopsWithoutPatchOrQueueRun(t *testing.T) {
	plan := smartqueue.Plan{Rows: map[string]smartqueue.Row{
		"before": {Status: smartqueue.StatusAssigned, PrinterID: "printer-1"},
		"bad":    {Status: smartqueue.StatusWaiting},
		"after":  {Status: smartqueue.StatusIncompatible},
	}}
	jobs := []smartqueue.ReserveJob{
		{
			ID: "before",
			RequiredTags: map[string]string{
				"filament.type": "PLA",
			},
		},
		{
			ID: "bad",
			RequiredTags: map[string]string{
				"printer.id":    "printer-2",
				"filament.type": "PETG",
				"user.tags":     "plain",
			},
		},
		{
			ID: "after",
			RequiredTags: map[string]string{
				"filament.type": "ABS",
			},
		},
	}

	var log patchLog
	err := smartqueue.Reserve(plan, jobs, log.patch)
	if !errors.Is(err, smartqueue.ErrUserTags) {
		t.Fatalf("Reserve() error = %v, want ErrUserTags", err)
	}
	assertPatches(t, log.calls, []patchCall{
		{id: "before", tags: map[string]string{
			"printer.id":    "printer-1",
			"filament.type": "PLA",
		}},
	})
}

func TestReservePatchErrorStopsLaterJobs(t *testing.T) {
	plan := smartqueue.Plan{Rows: map[string]smartqueue.Row{
		"first":  {Status: smartqueue.StatusWaiting},
		"failed": {Status: smartqueue.StatusAssigned, PrinterID: "printer-4"},
		"later":  {Status: smartqueue.StatusIncompatible},
	}}
	jobs := []smartqueue.ReserveJob{
		{
			ID: "first",
			RequiredTags: map[string]string{
				"filament.type": "PLA",
			},
		},
		{
			ID: "failed",
			RequiredTags: map[string]string{
				"filament.type": "PETG",
			},
		},
		{
			ID: "later",
			RequiredTags: map[string]string{
				"printer.id": "printer-9",
			},
		},
	}
	patchErr := errors.New("patch failed")
	log := patchLog{fail: map[string]error{"failed": patchErr}}

	err := smartqueue.Reserve(plan, jobs, log.patch)
	if !errors.Is(err, patchErr) {
		t.Fatalf("Reserve() error = %v, want patch failed", err)
	}
	assertPatches(t, log.calls, []patchCall{
		{id: "first", tags: map[string]string{
			"filament.type": "PLA",
			"user.tags":     "&=buddy-hold",
		}},
		{id: "failed", tags: map[string]string{
			"printer.id":    "printer-4",
			"filament.type": "PETG",
		}},
	})
}

func cloneJobs(jobs []smartqueue.ReserveJob) []smartqueue.ReserveJob {
	out := make([]smartqueue.ReserveJob, len(jobs))
	for i, job := range jobs {
		out[i] = smartqueue.ReserveJob{ID: job.ID, RequiredTags: maps.Clone(job.RequiredTags)}
	}
	return out
}

func jobsEqual(a, b []smartqueue.ReserveJob) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || !maps.Equal(a[i].RequiredTags, b[i].RequiredTags) {
			return false
		}
	}
	return true
}
