package smartqueue_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestEnableReadsSettingsSavesMergingSnapshotThenPausesMatching(t *testing.T) {
	cases := []struct {
		name    string
		merging bool
	}{
		{name: "remembers automatic merging", merging: true},
		{name: "absent automatic merging is false", merging: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnableEnv(t)
			env.api.settings = printago.StoreSettings{
				AutomaticQueueMatching: true,
				PlateMerging:           printago.PlateMerging{AutomaticMerging: tc.merging},
			}

			if err := env.sched.Enable(context.Background()); err != nil {
				t.Fatalf("Enable() error = %v", err)
			}

			if env.api.enabledAtGet {
				t.Fatal("state file was enabled before settings were read")
			}
			if !env.api.enabledAtPatch ||
				!env.api.stateAtPatch.HasMergingSnapshot ||
				env.api.stateAtPatch.AutomaticMerging != tc.merging ||
				!slices.Equal(env.api.stateAtPatch.PrioritizedJobIDs, []string{"keep"}) {
				t.Fatalf("state when settings were patched = %+v, want enabled with merging %v and prioritize keep", env.api.stateAtPatch, tc.merging)
			}
			assertCallOrder(t, env.api.calls, "get-settings", "patch-settings")
			if len(env.api.updates) != 1 {
				t.Fatalf("settings patches = %d, want 1", len(env.api.updates))
			}
			assertMatchingPaused(t, env.api.updates[0])
			assertState(t, env.path, smartqueue.State{
				Enabled:            true,
				HasMergingSnapshot: true,
				AutomaticMerging:   tc.merging,
				PrioritizedJobIDs:  []string{"keep"},
			})
			assertEnabled(t, env.sched.Snapshot())
		})
	}
}

func TestEnableWritesReservationsPublishesPlanAndRunsQueue(t *testing.T) {
	t.Run("empty failed check", func(t *testing.T) {
		env := newEnableEnv(t)

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		if snap.Err != "" {
			t.Fatalf("Err = %q, want empty", snap.Err)
		}
		assertRow(t, snap, "old", smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned})
		assertRow(t, snap, "young", smartqueue.Row{Status: smartqueue.StatusWaiting})
		assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
			"old":   {"filament.type": "PLA", "printer.id": "p1"},
			"young": {"filament.type": "PLA", "user.tags": "&=buddy-hold"},
		})
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
		if slices.Contains(env.api.calls, "get-slots") {
			t.Fatal("Enable fetched printer slots")
		}
		if slices.Contains(env.api.calls, "troubleshoot") {
			t.Fatal("Enable troubleshot a group with no tags-match failure")
		}
		if len(env.api.groupIDs) != 1 || !slices.Equal(env.api.groupIDs[0], []string{"young", "old"}) {
			t.Fatalf("group matching ids = %#v, want [young old]", env.api.groupIDs)
		}
		assertCallOrder(t, env.api.calls, "patch-settings", "update-tags")
		assertCallOrder(t, env.api.calls, "update-tags", "run-queue")
	})

	t.Run("staggered-start-hold", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", true, "staggered-start-hold")

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
	})
}

func TestEnableDoesNotRunQueueWhenNoAssignedPrinterIsReady(t *testing.T) {
	t.Run("assigned printer failed ready", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", true, "ready")

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		assertRow(t, snap, "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
			"job-1": {"filament.type": "PLA", "printer.id": "p1"},
		})
		if env.api.runCalls != 0 {
			t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
		}
	})

	t.Run("no assignment", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", false, "material-match")

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		assertRow(t, snap, "job-1", smartqueue.Row{Status: smartqueue.StatusIncompatible})
		assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
			"job-1": {"filament.type": "PLA", "user.tags": "&=buddy-hold"},
		})
		if env.api.runCalls != 0 {
			t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
		}
	})
}

func TestEnableTagsMatchReadyCheckGatesRunQueue(t *testing.T) {
	t.Run("failed ready does not run", func(t *testing.T) {
		env := newEnableEnv(t)
		setTagsMatchReady(env.api, false)

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		assertRow(t, snap, "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
			"job-1": {"filament.type": "PLA", "printer.id": "p1"},
		})
		if !slices.Equal(env.api.troubleIDs, []string{"job-1"}) {
			t.Fatalf("troubleshoot ids = %#v, want [job-1]", env.api.troubleIDs)
		}
		if env.api.runCalls != 0 {
			t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
		}
	})

	t.Run("passed ready runs", func(t *testing.T) {
		env := newEnableEnv(t)
		setTagsMatchReady(env.api, true)

		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}

		assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		assertCallOrder(t, env.api.calls, "update-tags", "run-queue")
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
	})
}

func setTagsMatchReady(api *enableAPI, readyPassed bool) {
	api.setSingleJob("job-1", false, "tags-match")
	api.trouble = printago.MatchingTroubleshootResult{
		Printers: []printago.TroubleshootPrinter{{
			PrinterID: "p1",
			Checks: []printago.TroubleshootCheck{
				{Code: "ready", Passed: readyPassed},
				{
					Code:   "tags-match",
					Passed: false,
					Comparisons: []printago.TroubleshootComparison{
						{Key: "printer.id", Requirement: "p1", Matched: false},
						{Key: "buddy-hold", Requirement: "buddy-hold", Matched: false},
						{Key: "filament.type", Requirement: "PLA", Matched: true},
					},
				},
			},
		}},
	}
}

func TestEnableRunQueueErrorPublishesPlanSetsErr(t *testing.T) {
	env := newEnableEnv(t)
	env.api.setSingleJob("job-1", true, "")
	env.api.runErr = errors.New("queue run failed")

	err := env.sched.Enable(context.Background())
	if !errors.Is(err, env.api.runErr) {
		t.Fatalf("Enable() error = %v, want %v", err, env.api.runErr)
	}

	snap := env.sched.Snapshot()
	assertEnabled(t, snap)
	if snap.Err != err.Error() {
		t.Fatalf("Err = %q, want %q", snap.Err, err.Error())
	}
	assertRow(t, snap, "job-1", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	if len(env.api.tagPatches) != 1 {
		t.Fatalf("tag patches = %d, want 1 before the queue run", len(env.api.tagPatches))
	}
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
	}
}

func TestEnableStateFileErrorLeavesEnabledFalseAndSkipsSettingsPatch(t *testing.T) {
	api := &enableAPI{
		settings: printago.StoreSettings{PlateMerging: printago.PlateMerging{AutomaticMerging: true}},
	}
	api.setSingleJob("job-1", true, "")
	path := blockedStatePath(t)
	sched := smartqueue.NewScheduler(api, smartqueue.Config{HalfLife: time.Hour, StateFile: path})

	err := sched.Enable(context.Background())
	if err == nil {
		t.Fatal("Enable() error = nil, want a state file error")
	}
	if slices.Contains(api.calls, "patch-settings") || slices.Contains(api.calls, "update-tags") || slices.Contains(api.calls, "run-queue") {
		t.Fatalf("calls = %v, want no settings patch, reservation, or queue run", api.calls)
	}
	snap := sched.Snapshot()
	if snap != nil && snap.Enabled {
		t.Fatalf("Snapshot() = %+v, want enabled false", snap)
	}
	if _, loadErr := smartqueue.LoadState(path); loadErr == nil {
		t.Fatal("LoadState() succeeded, want no enabled state file")
	}
}

func TestEnableSettingsPatchErrorLeavesEnabledTrue(t *testing.T) {
	env := newEnableEnv(t)
	env.api.updateErr = errors.New("patch settings")

	err := env.sched.Enable(context.Background())
	if !errors.Is(err, env.api.updateErr) {
		t.Fatalf("Enable() error = %v, want %v", err, env.api.updateErr)
	}

	assertState(t, env.path, smartqueue.State{
		Enabled:            true,
		HasMergingSnapshot: true,
		AutomaticMerging:   true,
		PrioritizedJobIDs:  []string{"keep"},
	})
	snap := env.sched.Snapshot()
	assertEnabled(t, snap)
	if _, ok := snap.Rows["old"]; ok {
		t.Fatalf("rows = %#v, want no new plan", snap.Rows)
	}
	for _, call := range env.api.calls {
		switch call {
		case "get-jobs", "get-printers", "get-slots", "group-matching", "troubleshoot", "update-tags", "run-queue":
			t.Fatalf("calls = %v, want no recompute after the settings patch failed", env.api.calls)
		}
	}
}

func TestEnableSnapshotEnabledAfterStateSaveWhenLaterStepsFail(t *testing.T) {
	t.Run("settings patch", func(t *testing.T) {
		env := newEnableEnv(t)
		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		env.api.updateErr = errors.New("patch settings")
		env.api.printers = []printago.Printer{{ID: "p2", Name: "Beta"}}
		env.api.setSingleJob("beta", true, "")

		err := env.sched.Enable(context.Background())
		if !errors.Is(err, env.api.updateErr) {
			t.Fatalf("Enable() error = %v, want %v", err, env.api.updateErr)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		assertRow(t, snap, "old", smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned})
		if _, ok := snap.Rows["beta"]; ok {
			t.Fatalf("rows = %#v, want the previous plan", snap.Rows)
		}
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
	})

	t.Run("reservation write", func(t *testing.T) {
		env := newEnableEnv(t)
		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		env.api.tagErr = errors.New("tag patch")
		env.api.printers = []printago.Printer{{ID: "p2", Name: "Beta"}}
		env.api.setSingleJob("beta", true, "")
		tagsBefore := len(env.api.tagPatches)

		err := env.sched.Enable(context.Background())
		if !errors.Is(err, env.api.tagErr) {
			t.Fatalf("Enable() error = %v, want %v", err, env.api.tagErr)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		assertRow(t, snap, "old", smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned})
		if _, ok := snap.Rows["beta"]; ok {
			t.Fatalf("rows = %#v, want the previous plan", snap.Rows)
		}
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
		if len(env.api.tagPatches) <= tagsBefore {
			t.Fatal("reservation write was not attempted")
		}
	})

	t.Run("queue run", func(t *testing.T) {
		env := newEnableEnv(t)
		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		env.api.runErr = errors.New("queue run failed")
		env.api.printers = []printago.Printer{{ID: "p2", Name: "Beta"}}
		env.api.setSingleJob("beta", true, "")

		err := env.sched.Enable(context.Background())
		if !errors.Is(err, env.api.runErr) {
			t.Fatalf("Enable() error = %v, want %v", err, env.api.runErr)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		if snap.Err != err.Error() {
			t.Fatalf("Err = %q, want %q", snap.Err, err.Error())
		}
		assertRow(t, snap, "beta", smartqueue.Row{PrinterID: "p2", PrinterName: "Beta", Status: smartqueue.StatusAssigned})
		if _, ok := snap.Rows["old"]; ok {
			t.Fatalf("rows = %#v, want the new plan", snap.Rows)
		}
	})
}

func TestEnableFailureBeforeReserveIsRetriedWhenSignaturesMatch(t *testing.T) {
	cases := []struct {
		name     string
		prepare  func(*enableAPI)
		fail     func(*enableAPI)
		clear    func(*enableAPI)
		checkErr bool
	}{
		{
			name:  "jobs",
			fail:  func(api *enableAPI) { api.jobsErr = errors.New("jobs") },
			clear: func(api *enableAPI) { api.jobsErr = nil },
		},
		{
			name:  "group matching",
			fail:  func(api *enableAPI) { api.groupErr = errors.New("matching") },
			clear: func(api *enableAPI) { api.groupErr = nil },
		},
		{
			name:    "troubleshoot",
			prepare: func(api *enableAPI) { setTagsMatchReady(api, true) },
			fail:    func(api *enableAPI) { api.troubleErr = errors.New("troubleshoot") },
			clear:   func(api *enableAPI) { api.troubleErr = nil },
		},
		{
			name:     "reserve",
			fail:     func(api *enableAPI) { api.tagErr = errors.New("tag patch") },
			clear:    func(api *enableAPI) { api.tagErr = nil },
			checkErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnableEnv(t)
			if tc.prepare != nil {
				tc.prepare(env.api)
			}
			if err := env.sched.Poll(context.Background()); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}
			if len(env.api.tagPatches) != 0 || len(env.api.updates) != 0 {
				t.Fatalf("patches while off: tags %#v settings %#v", env.api.tagPatches, env.api.updates)
			}
			before := env.sched.Snapshot()

			tc.fail(env.api)
			err := env.sched.Enable(context.Background())
			if err == nil {
				t.Fatal("Enable() error = nil, want a failure before reserve succeeded")
			}
			failed := env.sched.Snapshot()
			assertEnabled(t, failed)
			if !maps.Equal(before.Rows, failed.Rows) {
				t.Fatalf("rows = %#v, want %#v", failed.Rows, before.Rows)
			}
			if tc.checkErr && failed.Err != err.Error() {
				t.Fatalf("Err = %q, want %q", failed.Err, err.Error())
			}
			if slices.Contains(env.api.calls, "run-queue") {
				t.Fatalf("calls = %v, want no queue run before reserve succeeds", env.api.calls)
			}

			tc.clear(env.api)
			env.api.resetCalls()
			if err = env.sched.Poll(context.Background()); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}
			if len(env.api.tagPatches) == 0 {
				t.Fatal("reservation diff was not retried")
			}
			if slices.Contains(env.api.calls, "patch-settings") {
				t.Fatalf("calls = %v, want no settings patch", env.api.calls)
			}
		})
	}
}

func TestEnableRunQueueErrorIsRetriedWhenSignaturesMatch(t *testing.T) {
	env := newEnableEnv(t)
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	env.api.runErr = errors.New("queue run failed")
	env.api.resetCalls()

	err := env.sched.Enable(context.Background())
	if !errors.Is(err, env.api.runErr) {
		t.Fatalf("Enable() error = %v, want %v", err, env.api.runErr)
	}
	snap := env.sched.Snapshot()
	assertEnabled(t, snap)
	if snap.Err != err.Error() {
		t.Fatalf("Err = %q, want %q", snap.Err, err.Error())
	}
	assertRow(t, snap, "old", smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned})
	assertRow(t, snap, "young", smartqueue.Row{Status: smartqueue.StatusWaiting})
	if len(env.api.tagPatches) == 0 {
		t.Fatal("Enable did not write reservations")
	}

	env.api.runErr = nil
	env.api.resetCalls()
	if err = env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
	}
	assertNoJobOrSettingsPatches(t, env.api)
	if env.sched.Snapshot().Err != "" {
		t.Fatalf("Err = %q, want empty after the retry", env.sched.Snapshot().Err)
	}
}

func TestEnableSuccessLeavesUnchangedPollIdle(t *testing.T) {
	t.Run("queue run finished", func(t *testing.T) {
		env := newEnableEnv(t)
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}

		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		assertNoPrintagoWrites(t, env.api)
		assertRow(t, env.sched.Snapshot(), "old", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
	})

	t.Run("no assigned printer is ready", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", true, "ready")
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		if err := env.sched.Enable(context.Background()); err != nil {
			t.Fatalf("Enable() error = %v", err)
		}
		if env.api.runCalls != 0 {
			t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
		}

		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		assertNoPrintagoWrites(t, env.api)
		assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
	})
}

func TestEnableTroubleshootsRepresentativeTagsMatch(t *testing.T) {
	env := newEnableEnv(t)
	env.api.setSingleJob("job-1", false, "tags-match")
	env.api.trouble = printago.MatchingTroubleshootResult{
		Printers: []printago.TroubleshootPrinter{{
			PrinterID: "p1",
			Checks: []printago.TroubleshootCheck{{
				Code: "tags-match",
				Comparisons: []printago.TroubleshootComparison{{
					Key:     "filament.type",
					Matched: false,
				}},
			}},
		}},
	}

	if err := env.sched.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !slices.Equal(env.api.troubleIDs, []string{"job-1"}) {
		t.Fatalf("troubleshoot ids = %#v, want [job-1]", env.api.troubleIDs)
	}
	snap := env.sched.Snapshot()
	assertRow(t, snap, "job-1", smartqueue.Row{Status: smartqueue.StatusIncompatible})
	assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
		"job-1": {"filament.type": "PLA", "user.tags": "&=buddy-hold"},
	})
	if env.api.runCalls != 0 {
		t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
	}
}

func TestDisableClearsManagedTagsRestoresMergingThenSavesOff(t *testing.T) {
	cases := []struct {
		name    string
		merging bool
	}{
		{name: "restores automatic merging", merging: true},
		{name: "restores automatic merging off", merging: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := enabledScheduler(t, tc.merging)
			before := env.sched.Snapshot()
			parentID := "plate"
			unreadable := taggedJob("unreadable", map[string]string{
				"filament.type": "PLA",
				"printer.id":    "p9",
				"user.tags":     "&=buddy-hold",
			})
			unreadable.CreatedAt = "not-a-time"
			child := taggedJob("child", map[string]string{
				"filament.type": "PLA",
				"printer.id":    "p-old",
				"user.tags":     "not-an-and-list",
			})
			child.ParentJobID = &parentID
			env.api.jobs = []printago.PrintJob{
				taggedJob("pinned", map[string]string{
					"filament.type": "PLA",
					"nozzle.size":   "0.4",
					"printer.id":    "p-old",
					"user.tags":     "&=nozzle,buddy-hold",
				}),
				taggedJob("parked", map[string]string{
					"filament.type": "PLA",
					"user.tags":     "&=buddy-hold",
				}),
				taggedJob("clean", map[string]string{
					"filament.type": "PLA",
					"user.tags":     "&=nozzle",
				}),
				child,
				unreadable,
			}

			if err := env.sched.Disable(context.Background()); err != nil {
				t.Fatalf("Disable() error = %v", err)
			}

			assertPatchIDs(t, env.api.tagPatches, "pinned", "parked", "unreadable")
			assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
				"pinned": {
					"filament.type": "PLA",
					"nozzle.size":   "0.4",
					"user.tags":     "&=nozzle",
				},
				"parked": {"filament.type": "PLA"},
				"unreadable": {
					"filament.type": "PLA",
				},
			})
			if len(env.api.updates) != 1 {
				t.Fatalf("settings patches = %d, want 1", len(env.api.updates))
			}
			assertMatchingRestored(t, env.api.updates[0], tc.merging)
			assertCallOrder(t, env.api.calls, "update-tags", "patch-settings")
			if !env.api.stateAtPatch.Enabled || !env.api.stateAtPatch.HasMergingSnapshot {
				t.Fatalf("state when settings were patched = %+v, want still enabled with a merging snapshot", env.api.stateAtPatch)
			}
			assertState(t, env.path, smartqueue.State{
				Enabled:            false,
				HasMergingSnapshot: false,
				AutomaticMerging:   tc.merging,
				PrioritizedJobIDs:  []string{"keep"},
			})
			assertDisabledKeepsRows(t, before, env.sched.Snapshot())
			if env.api.runCalls != 0 {
				t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
			}
		})
	}

	t.Run("absent snapshot stays false", func(t *testing.T) {
		env := enabledScheduler(t, true)
		if err := smartqueue.SaveState(env.path, smartqueue.State{
			Enabled:            true,
			HasMergingSnapshot: false,
			AutomaticMerging:   true,
			PrioritizedJobIDs:  []string{"keep"},
		}); err != nil {
			t.Fatal(err)
		}
		before := env.sched.Snapshot()
		env.api.jobs = []printago.PrintJob{
			taggedJob("pinned", map[string]string{"filament.type": "PLA", "printer.id": "p1"}),
		}

		if err := env.sched.Disable(context.Background()); err != nil {
			t.Fatalf("Disable() error = %v", err)
		}

		if len(env.api.updates) != 1 {
			t.Fatalf("settings patches = %d, want 1", len(env.api.updates))
		}
		assertMatchingRestored(t, env.api.updates[0], false)
		assertState(t, env.path, smartqueue.State{
			Enabled:            false,
			HasMergingSnapshot: false,
			AutomaticMerging:   true,
			PrioritizedJobIDs:  []string{"keep"},
		})
		assertDisabledKeepsRows(t, before, env.sched.Snapshot())
	})
}

func TestDisableTagOrSettingsErrorLeavesEnabledTrue(t *testing.T) {
	t.Run("invalid user tags", func(t *testing.T) {
		env := enabledScheduler(t, true)
		before := env.sched.Snapshot()
		env.api.jobs = []printago.PrintJob{
			taggedJob("first", map[string]string{"filament.type": "PLA", "printer.id": "p1"}),
			taggedJob("bad", map[string]string{"filament.type": "PLA", "user.tags": "not-an-and-list"}),
			taggedJob("later", map[string]string{"filament.type": "PLA", "printer.id": "p2"}),
		}

		err := env.sched.Disable(context.Background())
		if !errors.Is(err, smartqueue.ErrUserTags) {
			t.Fatalf("Disable() error = %v, want ErrUserTags", err)
		}

		assertPatchIDs(t, env.api.tagPatches, "first")
		assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
			"first": {"filament.type": "PLA"},
		})
		assertStillEnabled(t, env, before)
	})

	t.Run("tag patch", func(t *testing.T) {
		env := enabledScheduler(t, true)
		before := env.sched.Snapshot()
		env.api.tagErr = errors.New("tag patch")
		env.api.jobs = []printago.PrintJob{
			taggedJob("first", map[string]string{"filament.type": "PLA", "printer.id": "p1"}),
			taggedJob("later", map[string]string{"filament.type": "PLA", "user.tags": "&=buddy-hold"}),
		}

		err := env.sched.Disable(context.Background())
		if !errors.Is(err, env.api.tagErr) {
			t.Fatalf("Disable() error = %v, want %v", err, env.api.tagErr)
		}

		assertPatchIDs(t, env.api.tagPatches, "first")
		assertStillEnabled(t, env, before)
	})

	t.Run("job list", func(t *testing.T) {
		env := enabledScheduler(t, true)
		before := env.sched.Snapshot()
		env.api.jobsErr = errors.New("jobs")

		err := env.sched.Disable(context.Background())
		if !errors.Is(err, env.api.jobsErr) {
			t.Fatalf("Disable() error = %v, want %v", err, env.api.jobsErr)
		}

		assertStillEnabled(t, env, before)
	})

	t.Run("settings patch", func(t *testing.T) {
		env := enabledScheduler(t, true)
		before := env.sched.Snapshot()
		env.api.updateErr = errors.New("patch settings")
		env.api.jobs = []printago.PrintJob{
			taggedJob("pinned", map[string]string{"filament.type": "PLA", "printer.id": "p1"}),
		}

		err := env.sched.Disable(context.Background())
		if !errors.Is(err, env.api.updateErr) {
			t.Fatalf("Disable() error = %v, want %v", err, env.api.updateErr)
		}

		assertPatchIDs(t, env.api.tagPatches, "pinned")
		assertCallOrder(t, env.api.calls, "update-tags", "patch-settings")
		if !env.api.stateAtPatch.Enabled {
			t.Fatalf("state when settings were patched = %+v, want enabled true", env.api.stateAtPatch)
		}
		assertStillEnabled(t, env, before)
	})
}

func enabledScheduler(t *testing.T, merging bool) *enableEnv {
	t.Helper()
	env := newEnableEnv(t)
	env.api.settings.AutomaticQueueMatching = false
	env.api.settings.PlateMerging.AutomaticMerging = merging
	if err := env.sched.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	env.api.resetCalls()
	return env
}

func taggedJob(id string, tags map[string]string) printago.PrintJob {
	job := pendingJob(id, time.Hour)
	job.RequiredPrinterTags = tags
	return job
}

func assertDisabledKeepsRows(t *testing.T, before, after *smartqueue.Plan) {
	t.Helper()
	if after == nil || after.Enabled {
		t.Fatalf("Snapshot() = %+v, want enabled false", after)
	}
	if before == nil || !maps.Equal(before.Rows, after.Rows) {
		t.Fatalf("rows = %#v, want %#v", after, before)
	}
}

func assertStillEnabled(t *testing.T, env *enableEnv, before *smartqueue.Plan) {
	t.Helper()
	if slices.Contains(env.api.calls, "patch-settings") && env.api.updateErr == nil {
		t.Fatalf("calls = %v, want no settings patch", env.api.calls)
	}
	assertState(t, env.path, smartqueue.State{
		Enabled:            true,
		HasMergingSnapshot: true,
		AutomaticMerging:   true,
		PrioritizedJobIDs:  []string{"keep"},
	})
	snap := env.sched.Snapshot()
	assertEnabled(t, snap)
	if before == nil || !maps.Equal(before.Rows, snap.Rows) {
		t.Fatalf("rows = %#v, want %#v", snap, before)
	}
}

func assertPatchIDs(t *testing.T, got []tagPatch, ids ...string) {
	t.Helper()
	gotIDs := make([]string, len(got))
	for i, patch := range got {
		gotIDs[i] = patch.id
	}
	if !slices.Equal(gotIDs, ids) {
		t.Fatalf("patched ids = %#v, want %#v", gotIDs, ids)
	}
}

func assertMatchingRestored(t *testing.T, update printago.StoreSettingsUpdate, merging bool) {
	t.Helper()
	if update.AutomaticQueueMatching == nil || !*update.AutomaticQueueMatching {
		t.Fatalf("automaticQueueMatching = %v, want true", update.AutomaticQueueMatching)
	}
	if update.PlateMerging == nil || update.PlateMerging.AutomaticMerging == nil || *update.PlateMerging.AutomaticMerging != merging {
		t.Fatalf("plateMerging = %#v, want automaticMerging %v", update.PlateMerging, merging)
	}
}

type enableEnv struct {
	api   *enableAPI
	sched *smartqueue.Scheduler
	path  string
}

func newEnableEnv(t *testing.T) *enableEnv {
	t.Helper()
	api := &enableAPI{}
	api.setOlderWins()
	path := filepath.Join(t.TempDir(), "smart-queue.json")
	if err := smartqueue.SaveState(path, smartqueue.State{PrioritizedJobIDs: []string{"keep"}}); err != nil {
		t.Fatal(err)
	}
	api.statePath = path
	sched := smartqueue.NewScheduler(api, smartqueue.Config{
		HalfLife:  30 * time.Minute,
		StateFile: path,
	})
	return &enableEnv{api: api, sched: sched, path: path}
}

func blockedStatePath(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "smart-queue.json")
}

func assertEnabled(t *testing.T, snap *smartqueue.Plan) {
	t.Helper()
	if snap == nil || !snap.Enabled {
		t.Fatalf("Snapshot() = %+v, want enabled", snap)
	}
}

func assertRow(t *testing.T, snap *smartqueue.Plan, id string, want smartqueue.Row) {
	t.Helper()
	if snap == nil {
		t.Fatal("Snapshot() = nil")
	}
	got, ok := snap.Rows[id]
	if !ok || got != want {
		t.Fatalf("row %s = %+v, want %+v in %#v", id, got, want, snap.Rows)
	}
}

func assertTagPatches(t *testing.T, got []tagPatch, want map[string]map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("patches = %#v, want %#v", got, want)
	}
	for _, patch := range got {
		if !maps.Equal(patch.tags, want[patch.id]) {
			t.Fatalf("patch %s = %#v, want %#v", patch.id, patch.tags, want[patch.id])
		}
	}
}

func assertState(t *testing.T, path string, want smartqueue.State) {
	t.Helper()
	got, err := smartqueue.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if got.Enabled != want.Enabled ||
		got.HasMergingSnapshot != want.HasMergingSnapshot ||
		got.AutomaticMerging != want.AutomaticMerging ||
		!slices.Equal(got.PrioritizedJobIDs, want.PrioritizedJobIDs) {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
}

func assertMatchingPaused(t *testing.T, update printago.StoreSettingsUpdate) {
	t.Helper()
	if update.AutomaticQueueMatching == nil || *update.AutomaticQueueMatching {
		t.Fatalf("automaticQueueMatching = %v, want false", update.AutomaticQueueMatching)
	}
	if update.PlateMerging == nil || update.PlateMerging.AutomaticMerging == nil || *update.PlateMerging.AutomaticMerging {
		t.Fatalf("plateMerging = %#v, want automaticMerging false", update.PlateMerging)
	}
}

func assertCallOrder(t *testing.T, calls []string, earlier, later string) {
	t.Helper()
	ie, il := slices.Index(calls, earlier), slices.Index(calls, later)
	if ie < 0 || il < 0 || ie > il {
		t.Fatalf("calls = %v, want %s before %s", calls, earlier, later)
	}
}

type tagPatch struct {
	id   string
	tags map[string]string
}

type enableAPI struct {
	settings           printago.StoreSettings
	settingsErr        error
	updateErr          error
	updates            []printago.StoreSettingsUpdate
	jobs               []printago.PrintJob
	jobsErr            error
	printers           []printago.Printer
	groups             printago.GroupMatchingResult
	groupErr           error
	groupIDs           [][]string
	trouble            printago.MatchingTroubleshootResult
	troubleErr         error
	troubleIDs         []string
	tagPatches         []tagPatch
	tagErr             error
	runErr             error
	runCalls           int
	calls              []string
	statePath          string
	enabledAtGet       bool
	enabledAtPatch     bool
	stateAtPatch       smartqueue.State
	slots              []printago.PrinterSlot
	slotsErr           error
	printersErr        error
	groupScript        []scriptedMatch
	prioritizedAtMatch []string
}

type scriptedMatch struct {
	result printago.GroupMatchingResult
	err    error
}

func (a *enableAPI) resetCalls() {
	a.calls = nil
	a.updates = nil
	a.tagPatches = nil
	a.runCalls = 0
	a.enabledAtPatch = false
	a.stateAtPatch = smartqueue.State{}
}

func (a *enableAPI) setOlderWins() {
	a.settings = printago.StoreSettings{PlateMerging: printago.PlateMerging{AutomaticMerging: true}}
	a.printers = []printago.Printer{{ID: "p1", Name: "Alpha"}}
	a.jobs = []printago.PrintJob{pendingJob("young", time.Hour), pendingJob("old", 2*time.Hour)}
	a.groups = oneGroup("young", []string{"young", "old"}, "p1", true, "")
}

func (a *enableAPI) setSingleJob(id string, matched bool, failed string) {
	if len(a.printers) == 0 {
		a.printers = []printago.Printer{{ID: "p1", Name: "Alpha"}}
	}
	a.jobs = []printago.PrintJob{pendingJob(id, time.Hour)}
	a.groups = oneGroup(id, []string{id}, a.printers[0].ID, matched, failed)
}

func pendingJob(id string, age time.Duration) printago.PrintJob {
	return printago.PrintJob{
		ID:                  id,
		Priority:            100,
		CreatedAt:           time.Now().UTC().Add(-age).Format(time.RFC3339),
		RequiredPrinterTags: map[string]string{"filament.type": "PLA"},
	}
}

func oneGroup(rep string, members []string, printerID string, matched bool, failed string) printago.GroupMatchingResult {
	return printago.GroupMatchingResult{
		Groups: []printago.MatchGroup{{
			RepresentativePrintJobID: rep,
			MemberPrintJobIDs:        members,
			Printers: []printago.MatchPrinterVerdict{{
				PrinterID:        printerID,
				Matched:          matched,
				FirstFailedCheck: failed,
			}},
		}},
	}
}

func (a *enableAPI) GetPrinters(context.Context) ([]printago.Printer, error) {
	a.calls = append(a.calls, "get-printers")
	if a.printersErr != nil {
		return nil, a.printersErr
	}
	return a.printers, nil
}

func (a *enableAPI) GetPrinterSlots(context.Context) ([]printago.PrinterSlot, error) {
	a.calls = append(a.calls, "get-slots")
	if a.slotsErr != nil {
		return nil, a.slotsErr
	}
	return a.slots, nil
}

func (a *enableAPI) GetMaterials(context.Context) ([]printago.Material, error) {
	return nil, nil
}

func (a *enableAPI) GetMaterialVariants(context.Context) ([]printago.MaterialVariant, error) {
	return nil, nil
}

func (a *enableAPI) UpdatePrinterTags(context.Context, string, []string) error {
	return nil
}

func (a *enableAPI) GetPrintJobs(context.Context) ([]printago.PrintJob, error) {
	a.calls = append(a.calls, "get-jobs")
	if a.jobsErr != nil {
		return nil, a.jobsErr
	}
	return a.jobs, nil
}

func (a *enableAPI) GetPartMaterialAssignments(context.Context, string) ([]printago.PartMaterialAssignment, error) {
	return nil, nil
}

func (a *enableAPI) CancelPrintJob(context.Context, string) error { return nil }

func (a *enableAPI) PrioritizePrintJob(context.Context, string) error { return nil }

func (a *enableAPI) UpdatePrintJobTags(_ context.Context, id string, tags map[string]string) error {
	a.calls = append(a.calls, "update-tags")
	a.tagPatches = append(a.tagPatches, tagPatch{id: id, tags: maps.Clone(tags)})
	return a.tagErr
}

func (a *enableAPI) GetStoreSettings(context.Context) (printago.StoreSettings, error) {
	a.calls = append(a.calls, "get-settings")
	if a.statePath != "" {
		if state, err := smartqueue.LoadState(a.statePath); err == nil {
			a.enabledAtGet = state.Enabled
		}
	}
	if a.settingsErr != nil {
		return printago.StoreSettings{}, a.settingsErr
	}
	return a.settings, nil
}

func (a *enableAPI) UpdateStoreSettings(_ context.Context, update printago.StoreSettingsUpdate) error {
	a.calls = append(a.calls, "patch-settings")
	a.updates = append(a.updates, update)
	if a.statePath != "" {
		if state, err := smartqueue.LoadState(a.statePath); err == nil {
			a.enabledAtPatch = state.Enabled
			a.stateAtPatch = state
		}
	}
	return a.updateErr
}

func (a *enableAPI) GroupMatching(_ context.Context, ids []string) (printago.GroupMatchingResult, error) {
	a.calls = append(a.calls, "group-matching")
	a.groupIDs = append(a.groupIDs, append([]string(nil), ids...))
	if a.statePath != "" {
		if state, err := smartqueue.LoadState(a.statePath); err == nil {
			a.prioritizedAtMatch = append([]string(nil), state.PrioritizedJobIDs...)
		}
	}
	if len(a.groupScript) > 0 {
		next := a.groupScript[0]
		a.groupScript = a.groupScript[1:]
		if next.err != nil {
			return printago.GroupMatchingResult{}, next.err
		}
		return next.result, nil
	}
	if a.groupErr != nil {
		return printago.GroupMatchingResult{}, a.groupErr
	}
	return a.groups, nil
}

func (a *enableAPI) MatchingTroubleshoot(_ context.Context, id string) (printago.MatchingTroubleshootResult, error) {
	a.calls = append(a.calls, "troubleshoot")
	a.troubleIDs = append(a.troubleIDs, id)
	if a.troubleErr != nil {
		return printago.MatchingTroubleshootResult{}, a.troubleErr
	}
	return a.trouble, nil
}

func (a *enableAPI) RunQueue(context.Context) error {
	a.calls = append(a.calls, "run-queue")
	a.runCalls++
	return a.runErr
}

func TestStartupMissingStateFileDoesNotCallPrintago(t *testing.T) {
	api := &enableAPI{}
	api.setOlderWins()
	path := filepath.Join(t.TempDir(), "missing", "smart-queue.json")
	sched := smartqueue.NewScheduler(api, smartqueue.Config{HalfLife: 30 * time.Minute, StateFile: path})

	if err := sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	snap := sched.Snapshot()
	if snap == nil || snap.Enabled || snap.Err != "" {
		t.Fatalf("Snapshot() = %+v, want a disabled plan with no error", snap)
	}
	assertRow(t, snap, "old", smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned})
	assertRow(t, snap, "young", smartqueue.Row{Status: smartqueue.StatusWaiting})
	assertPollReads(t, api)
	assertNoPrintagoWrites(t, api)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file stat err = %v, want not exist", err)
	}
}

func TestIncompatibleJobIsParkedWhenEnabled(t *testing.T) {
	codes := []string{
		"material-match",
		"profiles-configured",
		"slots-loaded",
		"routing-rules",
		"filament-profiles",
		"bed-fits",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			env := newEnableEnv(t)
			saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
			env.api.setSingleJob("job-1", false, code)
			env.api.groups.Groups[0].Printers[0].Compatible = true

			if err := env.sched.Poll(context.Background()); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}

			snap := env.sched.Snapshot()
			assertEnabled(t, snap)
			if snap.Err != "" {
				t.Fatalf("Err = %q, want empty", snap.Err)
			}
			assertRow(t, snap, "job-1", smartqueue.Row{Status: smartqueue.StatusIncompatible})
			assertTagPatches(t, env.api.tagPatches, map[string]map[string]string{
				"job-1": {"filament.type": "PLA", "user.tags": "&=buddy-hold"},
			})
			if _, pinned := env.api.tagPatches[0].tags["printer.id"]; pinned {
				t.Fatalf("tags = %#v, want no printer.id", env.api.tagPatches[0].tags)
			}
			if env.api.runCalls != 0 {
				t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
			}
			assertCallOrder(t, env.api.calls, "patch-settings", "update-tags")
			assertMatchingPaused(t, env.api.updates[0])
		})
	}
}

func TestNewJobChangesAssignment(t *testing.T) {
	t.Run("switch off does not patch", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("old", true, "")

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		assertRow(t, env.sched.Snapshot(), "old", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})

		env.api.jobs = []printago.PrintJob{pendingJob("old", time.Hour), pendingJob("older", 3*time.Hour)}
		env.api.groups = oneGroup("older", []string{"older", "old"}, "p1", true, "")
		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		snap := env.sched.Snapshot()
		if snap.Enabled {
			t.Fatal("Snapshot enabled, want off")
		}
		assertRow(t, snap, "older", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Beta", Status: smartqueue.StatusAssigned,
		})
		assertRow(t, snap, "old", smartqueue.Row{Status: smartqueue.StatusWaiting})
		assertNoPrintagoWrites(t, env.api)
	})

	t.Run("switch on reserves before run", func(t *testing.T) {
		env := newEnableEnv(t)
		saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
		env.api.setSingleJob("old", true, "ready")

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		env.api.jobs = []printago.PrintJob{pendingJob("old", time.Hour), pendingJob("older", 3*time.Hour)}
		env.api.groups = oneGroup("older", []string{"older", "old"}, "p1", true, "")
		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertRow(t, snap, "older", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Beta", Status: smartqueue.StatusAssigned,
		})
		assertRow(t, snap, "old", smartqueue.Row{Status: smartqueue.StatusWaiting})
		if len(env.api.tagPatches) == 0 {
			t.Fatal("reservation was not written")
		}
		assertCallOrder(t, env.api.calls, "update-tags", "run-queue")
		if env.api.runCalls != 1 {
			t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
		}
	})
}

func TestReadyChangeRunsQueueWithoutRecompute(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "ready")

	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	if env.api.runCalls != 0 {
		t.Fatalf("RunQueue calls = %d, want 0 before the printer is ready", env.api.runCalls)
	}

	env.api.groups = oneGroup("job-1", []string{"job-1"}, "p1", true, "")
	env.api.printers[0].Name = "Beta"
	env.api.resetCalls()

	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
	}
	assertNoJobOrSettingsPatches(t, env.api)
}

func TestApplyFailureKeepsPublishedPlan(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "")

	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	before := env.sched.Snapshot()

	env.api.jobs = []printago.PrintJob{pendingJob("job-1", time.Hour), pendingJob("job-2", 3*time.Hour)}
	env.api.groups = oneGroup("job-2", []string{"job-2", "job-1"}, "p1", true, "")
	env.api.tagErr = errors.New("tag patch")
	env.api.resetCalls()

	err := env.sched.Poll(context.Background())
	if !errors.Is(err, env.api.tagErr) {
		t.Fatalf("Poll() error = %v, want %v", err, env.api.tagErr)
	}
	failed := env.sched.Snapshot()
	if failed.Err != err.Error() {
		t.Fatalf("Err = %q, want %q", failed.Err, err.Error())
	}
	if !maps.Equal(before.Rows, failed.Rows) {
		t.Fatalf("rows = %#v, want %#v", failed.Rows, before.Rows)
	}
	if env.api.runCalls != 0 {
		t.Fatalf("RunQueue calls = %d, want 0", env.api.runCalls)
	}

	env.api.tagErr = nil
	env.api.printers[0].Name = "Beta"
	env.api.resetCalls()

	if err = env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	snap := env.sched.Snapshot()
	if snap.Err != "" {
		t.Fatalf("Err = %q, want empty", snap.Err)
	}
	assertRow(t, snap, "job-2", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	assertRow(t, snap, "job-1", smartqueue.Row{Status: smartqueue.StatusWaiting})
	if len(env.api.tagPatches) == 0 {
		t.Fatal("reservation diff was not retried")
	}
	if slices.Contains(env.api.calls, "patch-settings") {
		t.Fatalf("calls = %v, want no settings patch", env.api.calls)
	}
	assertCallOrder(t, env.api.calls, "update-tags", "run-queue")
}

func TestPollUnchangedSignatureDoesNoPrintagoWork(t *testing.T) {
	t.Run("switch off", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", true, "")
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		assertPollReads(t, env.api)
		assertNoPrintagoWrites(t, env.api)
	})

	t.Run("switch on", func(t *testing.T) {
		env := newEnableEnv(t)
		saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
		env.api.setSingleJob("job-1", true, "ready")
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		env.api.printers[0].Name = "Beta"
		env.api.resetCalls()
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		assertRow(t, env.sched.Snapshot(), "job-1", smartqueue.Row{
			PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
		})
		assertPollReads(t, env.api)
		assertNoPrintagoWrites(t, env.api)
	})
}

func TestPollPatchesSettingsBeforeRetryWhenSignaturesMatch(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "ready")
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	env.api.jobs = []printago.PrintJob{pendingJob("job-1", time.Hour), pendingJob("job-2", 3*time.Hour)}
	env.api.groups = oneGroup("job-2", []string{"job-2", "job-1"}, "p1", true, "")
	env.api.tagErr = errors.New("tag patch")
	env.api.resetCalls()
	if err := env.sched.Poll(context.Background()); !errors.Is(err, env.api.tagErr) {
		t.Fatalf("Poll() error = %v, want %v", err, env.api.tagErr)
	}

	env.api.updateErr = errors.New("patch settings")
	env.api.resetCalls()
	if err := env.sched.Enable(context.Background()); !errors.Is(err, env.api.updateErr) {
		t.Fatalf("Enable() error = %v, want %v", err, env.api.updateErr)
	}

	env.api.updateErr = nil
	env.api.tagErr = nil
	env.api.printers[0].Name = "Beta"
	env.api.resetCalls()
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	assertCallOrder(t, env.api.calls, "patch-settings", "update-tags")
	assertMatchingPaused(t, env.api.updates[0])
	assertRow(t, env.sched.Snapshot(), "job-2", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	if slices.Contains(env.api.calls, "run-queue") && slices.Index(env.api.calls, "update-tags") > slices.Index(env.api.calls, "run-queue") {
		t.Fatalf("calls = %v, want reservations before queue run", env.api.calls)
	}
}

func TestPollReadErrorKeepsPublishedPlan(t *testing.T) {
	t.Run("first jobs error does not publish", func(t *testing.T) {
		env := newEnableEnv(t)
		saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
		env.api.jobsErr = errors.New("jobs")

		err := env.sched.Poll(context.Background())
		if !errors.Is(err, env.api.jobsErr) {
			t.Fatalf("Poll() error = %v, want %v", err, env.api.jobsErr)
		}
		if env.sched.Snapshot() != nil {
			t.Fatalf("Snapshot() = %+v, want nil", env.sched.Snapshot())
		}
		assertNoPrintagoWrites(t, env.api)
	})

	cases := []struct {
		name string
		set  func(*enableAPI)
	}{
		{name: "jobs", set: func(api *enableAPI) { api.jobsErr = errors.New("jobs") }},
		{name: "printers", set: func(api *enableAPI) { api.printersErr = errors.New("printers") }},
		{name: "slots", set: func(api *enableAPI) { api.slotsErr = errors.New("slots") }},
		{name: "group matching", set: func(api *enableAPI) { api.groupErr = errors.New("matching") }},
		{name: "troubleshoot", set: func(api *enableAPI) {
			api.setSingleJob("job-1", false, "tags-match")
			api.troubleErr = errors.New("troubleshoot")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnableEnv(t)
			saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
			env.api.setSingleJob("job-1", true, "")
			if err := env.sched.Poll(context.Background()); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}
			before := env.sched.Snapshot()
			env.api.resetCalls()
			tc.set(env.api)

			err := env.sched.Poll(context.Background())
			if err == nil {
				t.Fatal("Poll() error = nil, want a read error")
			}
			snap := env.sched.Snapshot()
			if snap.Err != err.Error() {
				t.Fatalf("Err = %q, want %q", snap.Err, err.Error())
			}
			if !maps.Equal(before.Rows, snap.Rows) || snap.Enabled != before.Enabled {
				t.Fatalf("snapshot = %+v, want rows %#v enabled %v", snap, before.Rows, before.Enabled)
			}
			assertNoPrintagoWrites(t, env.api)
		})
	}
}

func TestPollEmptyPendingPublishesNoRows(t *testing.T) {
	t.Run("switch on does not patch jobs", func(t *testing.T) {
		env := newEnableEnv(t)
		saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
		env.api.jobs = nil
		env.api.groups = printago.GroupMatchingResult{}

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}

		snap := env.sched.Snapshot()
		assertEnabled(t, snap)
		if len(snap.Rows) != 0 || snap.Err != "" {
			t.Fatalf("snapshot = %+v, want no rows and no error", snap)
		}
		if len(env.api.tagPatches) != 0 || env.api.runCalls != 0 {
			t.Fatalf("tag patches = %#v, runs = %d, want none", env.api.tagPatches, env.api.runCalls)
		}
		assertMatchingPaused(t, env.api.updates[0])
	})

	t.Run("switch off", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.jobs = nil
		env.api.groups = printago.GroupMatchingResult{}

		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		snap := env.sched.Snapshot()
		if snap == nil || snap.Enabled || len(snap.Rows) != 0 || snap.Err != "" {
			t.Fatalf("snapshot = %+v, want disabled with no rows", snap)
		}
		assertNoPrintagoWrites(t, env.api)
	})
}

func TestPollDropsPrioritizeIDsMissingFromPending(t *testing.T) {
	env := newEnableEnv(t)
	parent := "plate"
	child := pendingJob("child", time.Hour)
	child.ParentJobID = &parent
	saveSchedulerState(t, env.path, smartqueue.State{
		PrioritizedJobIDs: []string{"gone", "keep", "child"},
	})
	env.api.jobs = []printago.PrintJob{pendingJob("keep", time.Hour), child}
	env.api.groupErr = errors.New("matching")

	err := env.sched.Poll(context.Background())
	if !errors.Is(err, env.api.groupErr) {
		t.Fatalf("Poll() error = %v, want %v", err, env.api.groupErr)
	}
	if env.sched.Snapshot() != nil {
		t.Fatalf("Snapshot() = %+v, want nil", env.sched.Snapshot())
	}
	assertState(t, env.path, smartqueue.State{PrioritizedJobIDs: []string{"keep", "child"}})
	if !slices.Equal(env.api.prioritizedAtMatch, []string{"keep", "child"}) {
		t.Fatalf("prioritized ids at matching = %#v, want [keep child]", env.api.prioritizedAtMatch)
	}
	assertCallOrder(t, env.api.calls, "get-jobs", "group-matching")
	assertNoPrintagoWrites(t, env.api)
}

func TestPollTruncationRetriesMissingJobIDs(t *testing.T) {
	env := newEnableEnv(t)
	env.api.printers = []printago.Printer{{ID: "p1", Name: "Alpha"}}
	env.api.jobs = []printago.PrintJob{pendingJob("a", time.Hour), pendingJob("b", 3*time.Hour)}
	env.api.groupScript = []scriptedMatch{
		{result: printago.GroupMatchingResult{
			Truncated: true,
			Groups: []printago.MatchGroup{{
				RepresentativePrintJobID: "a",
				MemberPrintJobIDs:        []string{"a"},
				Printers: []printago.MatchPrinterVerdict{{
					PrinterID: "p1",
					Matched:   true,
				}},
			}},
		}},
		{result: printago.GroupMatchingResult{
			Groups: []printago.MatchGroup{{
				RepresentativePrintJobID: "b",
				MemberPrintJobIDs:        []string{"b"},
				Printers: []printago.MatchPrinterVerdict{{
					PrinterID: "p1",
					Matched:   true,
				}},
			}},
		}},
	}

	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if len(env.api.groupIDs) != 2 ||
		!slices.Equal(env.api.groupIDs[0], []string{"a", "b"}) ||
		!slices.Equal(env.api.groupIDs[1], []string{"b"}) {
		t.Fatalf("group matching ids = %#v, want [a b] then [b]", env.api.groupIDs)
	}
	snap := env.sched.Snapshot()
	assertRow(t, snap, "b", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	assertRow(t, snap, "a", smartqueue.Row{Status: smartqueue.StatusWaiting})
	assertNoPrintagoWrites(t, env.api)
}

func TestPollNamedTruncationKeepsPublishedPlan(t *testing.T) {
	t.Run("keeps the published plan", func(t *testing.T) {
		env := newEnableEnv(t)
		env.api.setSingleJob("job-1", true, "")
		if err := env.sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		before := env.sched.Snapshot()

		env.api.jobs = []printago.PrintJob{pendingJob("job-1", time.Hour), pendingJob("job-2", time.Hour)}
		env.api.groupScript = []scriptedMatch{
			{result: printago.GroupMatchingResult{
				Truncated: true,
				Groups: []printago.MatchGroup{{
					RepresentativePrintJobID: "job-1",
					MemberPrintJobIDs:        []string{"job-1"},
					Printers:                 []printago.MatchPrinterVerdict{{PrinterID: "p1", Matched: true}},
				}},
			}},
			{result: printago.GroupMatchingResult{
				Truncated: true,
				Groups: []printago.MatchGroup{{
					RepresentativePrintJobID: "job-2",
					MemberPrintJobIDs:        []string{"job-2"},
					Printers:                 []printago.MatchPrinterVerdict{{PrinterID: "p1", Matched: true}},
				}},
			}},
		}
		env.api.resetCalls()
		env.api.groupIDs = nil

		err := env.sched.Poll(context.Background())
		if err == nil || !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("Poll() error = %v, want truncation", err)
		}
		snap := env.sched.Snapshot()
		if snap.Err != err.Error() || !maps.Equal(before.Rows, snap.Rows) {
			t.Fatalf("snapshot = %+v, want err and rows %#v", snap, before.Rows)
		}
		if len(env.api.groupIDs) != 2 || !slices.Equal(env.api.groupIDs[1], []string{"job-2"}) {
			t.Fatalf("group matching ids = %#v, want a named retry for job-2", env.api.groupIDs)
		}
		assertNoPrintagoWrites(t, env.api)
	})

	t.Run("does not patch when nothing was published", func(t *testing.T) {
		env := newEnableEnv(t)
		saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
		env.api.setSingleJob("job-1", true, "")
		env.api.groupScript = []scriptedMatch{
			{result: printago.GroupMatchingResult{Truncated: true}},
			{result: printago.GroupMatchingResult{Truncated: true}},
		}

		err := env.sched.Poll(context.Background())
		if err == nil || !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("Poll() error = %v, want truncation", err)
		}
		if env.sched.Snapshot() != nil {
			t.Fatalf("Snapshot() = %+v, want nil", env.sched.Snapshot())
		}
		assertNoPrintagoWrites(t, env.api)
	})
}

func TestPollRunQueueFailurePublishesAndRetries(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "ready")
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	env.api.jobs = []printago.PrintJob{pendingJob("job-1", time.Hour), pendingJob("job-2", 3*time.Hour)}
	env.api.groups = oneGroup("job-2", []string{"job-2", "job-1"}, "p1", true, "")
	env.api.printers[0].Name = "Beta"
	env.api.runErr = errors.New("queue run failed")
	env.api.resetCalls()

	err := env.sched.Poll(context.Background())
	if !errors.Is(err, env.api.runErr) {
		t.Fatalf("Poll() error = %v, want %v", err, env.api.runErr)
	}
	failed := env.sched.Snapshot()
	if failed.Err != err.Error() {
		t.Fatalf("Err = %q, want %q", failed.Err, err.Error())
	}
	assertRow(t, failed, "job-2", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Beta", Status: smartqueue.StatusAssigned,
	})
	assertRow(t, failed, "job-1", smartqueue.Row{Status: smartqueue.StatusWaiting})

	env.api.runErr = nil
	env.api.printers[0].Name = "Gamma"
	env.api.resetCalls()
	if err = env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	snap := env.sched.Snapshot()
	if snap.Err != "" {
		t.Fatalf("Err = %q, want empty", snap.Err)
	}
	assertRow(t, snap, "job-2", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Beta", Status: smartqueue.StatusAssigned,
	})
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1", env.api.runCalls)
	}
	assertNoJobOrSettingsPatches(t, env.api)
}

func TestPollUserTagsStopsReservation(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "ready")
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	before := env.sched.Snapshot()

	env.api.jobs[0].RequiredPrinterTags = map[string]string{
		"filament.type": "PLA",
		"user.tags":     "not-an-and-list",
	}
	env.api.resetCalls()

	err := env.sched.Poll(context.Background())
	if !errors.Is(err, smartqueue.ErrUserTags) {
		t.Fatalf("Poll() error = %v, want ErrUserTags", err)
	}
	failed := env.sched.Snapshot()
	if failed.Err != err.Error() || !maps.Equal(before.Rows, failed.Rows) {
		t.Fatalf("snapshot = %+v, want err and rows %#v", failed, before.Rows)
	}
	if len(env.api.tagPatches) != 0 || env.api.runCalls != 0 {
		t.Fatalf("patches = %#v, runs = %d, want no patch and no queue run", env.api.tagPatches, env.api.runCalls)
	}

	env.api.resetCalls()
	err = env.sched.Poll(context.Background())
	if !errors.Is(err, smartqueue.ErrUserTags) {
		t.Fatalf("Poll() error = %v, want ErrUserTags again", err)
	}
	if !maps.Equal(before.Rows, env.sched.Snapshot().Rows) {
		t.Fatalf("rows = %#v, want %#v", env.sched.Snapshot().Rows, before.Rows)
	}
}

func TestPollSkipsMergedAndUnreadable(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	parent := "plate"
	child := pendingJob("child", time.Hour)
	child.ParentJobID = &parent
	child.RequiredPrinterTags = map[string]string{"filament.type": "PLA", "printer.id": "p-old", "user.tags": "&=buddy-hold"}
	unreadable := pendingJob("bad", time.Hour)
	unreadable.CreatedAt = "not-a-time"
	unreadable.RequiredPrinterTags = map[string]string{"filament.type": "PLA", "printer.id": "p-old"}
	env.api.jobs = []printago.PrintJob{
		pendingJob("job-1", time.Hour),
		child,
		unreadable,
	}
	env.api.groups = oneGroup("job-1", []string{"job-1", "child", "bad"}, "p1", true, "")

	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	snap := env.sched.Snapshot()
	assertRow(t, snap, "job-1", smartqueue.Row{
		PrinterID: "p1", PrinterName: "Alpha", Status: smartqueue.StatusAssigned,
	})
	assertRow(t, snap, "child", smartqueue.Row{Status: smartqueue.StatusMerged})
	assertRow(t, snap, "bad", smartqueue.Row{Status: smartqueue.StatusUnreadable})
	assertPatchIDs(t, env.api.tagPatches, "job-1")
}

func TestPollSlotChangeRecomputes(t *testing.T) {
	env := newEnableEnv(t)
	saveSchedulerState(t, env.path, smartqueue.State{Enabled: true, HasMergingSnapshot: true})
	env.api.setSingleJob("job-1", true, "")
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	env.api.slots = []printago.PrinterSlot{{PrinterID: "p1"}}
	env.api.resetCalls()
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1 after a slot change", env.api.runCalls)
	}

	env.api.printers[0].Tags = []string{"bambu"}
	env.api.resetCalls()
	if err := env.sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if env.api.runCalls != 1 {
		t.Fatalf("RunQueue calls = %d, want 1 after a printer tag change", env.api.runCalls)
	}
}

func saveSchedulerState(t *testing.T, path string, state smartqueue.State) {
	t.Helper()
	if err := smartqueue.SaveState(path, state); err != nil {
		t.Fatal(err)
	}
}

func assertPollReads(t *testing.T, api *enableAPI) {
	t.Helper()
	for _, call := range []string{"get-jobs", "get-printers", "get-slots", "group-matching"} {
		if !slices.Contains(api.calls, call) {
			t.Fatalf("calls = %v, want %s", api.calls, call)
		}
	}
}

func assertNoPrintagoWrites(t *testing.T, api *enableAPI) {
	t.Helper()
	for _, call := range api.calls {
		switch call {
		case "patch-settings", "update-tags", "run-queue":
			t.Fatalf("calls = %v, want no settings patch, job patch, or queue run", api.calls)
		}
	}
}

func assertNoJobOrSettingsPatches(t *testing.T, api *enableAPI) {
	t.Helper()
	for _, call := range api.calls {
		switch call {
		case "patch-settings", "update-tags":
			t.Fatalf("calls = %v, want no settings or job patch", api.calls)
		}
	}
}
