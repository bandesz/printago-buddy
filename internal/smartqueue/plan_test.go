package smartqueue_test

import (
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestSnapshotReturnsNilBeforePublish(t *testing.T) {
	var cache smartqueue.PlanCache

	if got := cache.Snapshot(); got != nil {
		t.Fatalf("Snapshot() = %#v, want a nil plan", got)
	}
}

func TestSnapshotCopyDoesNotChangePublishedPlan(t *testing.T) {
	var cache smartqueue.PlanCache
	published := smartqueue.Plan{
		Enabled: true,
		Err:     "poll failed",
		Rows: map[string]smartqueue.Row{
			"job-1": {PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"},
			"job-2": {Status: "waiting"},
		},
	}
	cache.Publish(published)

	snap := cache.Snapshot()
	if snap == nil || !snap.Enabled || snap.Err != "poll failed" || snap.Rows["job-1"].PrinterName != "Alpha" {
		t.Fatalf("Snapshot() = %#v, want a copy of the published plan", snap)
	}
	snap.Enabled = false
	snap.Err = "changed"
	snap.Rows["job-1"] = smartqueue.Row{PrinterID: "other", PrinterName: "changed", Status: "incompatible"}
	delete(snap.Rows, "job-2")
	snap.Rows["job-3"] = smartqueue.Row{Status: "merged"}
	published.Enabled = false
	published.Err = "changed"
	published.Rows["job-1"] = smartqueue.Row{Status: "unreadable"}
	delete(published.Rows, "job-2")

	got := cache.Snapshot()
	if got == nil {
		t.Fatal("Snapshot() = nil, want the published plan")
	}
	if !got.Enabled || got.Err != "poll failed" {
		t.Errorf("plan = enabled %v err %q, want enabled true and %q", got.Enabled, got.Err, "poll failed")
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %#v, want the two published rows", got.Rows)
	}
	wantJob1 := smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}
	if got.Rows["job-1"] != wantJob1 {
		t.Errorf("job-1 = %+v, want %+v", got.Rows["job-1"], wantJob1)
	}
	wantJob2 := smartqueue.Row{Status: "waiting"}
	if got.Rows["job-2"] != wantJob2 {
		t.Errorf("job-2 = %+v, want %+v", got.Rows["job-2"], wantJob2)
	}
}

func TestBuildPlanAssignedUsesPrinterName(t *testing.T) {
	plan := smartqueue.BuildPlan(planInput(
		[]smartqueue.PlanJob{job("job-1", 100, ago(time.Hour))},
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("job-1", []string{"job-1"}, "p1")},
	))

	want := smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}
	if plan.Rows["job-1"] != want {
		t.Errorf("row = %+v, want %+v", plan.Rows["job-1"], want)
	}
}

func TestBuildPlanAssignedMissingNameUsesPrinterID(t *testing.T) {
	cases := []struct {
		name  string
		names map[string]string
	}{
		{name: "empty", names: map[string]string{"p1": ""}},
		{name: "absent", names: map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := smartqueue.BuildPlan(planInput(
				[]smartqueue.PlanJob{job("job-1", 100, ago(time.Hour))},
				tc.names,
				[]smartqueue.PlanGroup{matchedGroup("job-1", []string{"job-1"}, "p1")},
			))

			want := smartqueue.Row{PrinterID: "p1", PrinterName: "p1", Status: "assigned"}
			if plan.Rows["job-1"] != want {
				t.Errorf("row = %+v, want %+v", plan.Rows["job-1"], want)
			}
		})
	}
}

func TestBuildPlanWaitingWhenMatchIsLost(t *testing.T) {
	// Younger is first, so an equal-weight tie would assign the younger job.
	plan := smartqueue.BuildPlan(planInput(
		[]smartqueue.PlanJob{
			job("young", 100, ago(0)),
			job("old", 100, ago(2*time.Hour)),
		},
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("young", []string{"young", "old"}, "p1")},
	))

	if plan.Rows["old"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
		t.Errorf("old = %+v, want assigned to Alpha", plan.Rows["old"])
	}
	if plan.Rows["young"] != (smartqueue.Row{Status: "waiting"}) {
		t.Errorf("young = %+v, want waiting", plan.Rows["young"])
	}
}

func TestBuildPlanIncompatibleWhenNoEdge(t *testing.T) {
	t.Run("material-match", func(t *testing.T) {
		plan := smartqueue.BuildPlan(planInput(
			[]smartqueue.PlanJob{job("job-1", 100, ago(time.Hour))},
			map[string]string{"p1": "Alpha"},
			[]smartqueue.PlanGroup{{
				MemberIDs:        []string{"job-1"},
				RepresentativeID: "job-1",
				Verdicts: []smartqueue.Verdict{{
					PrinterID:        "p1",
					Matched:          false,
					Compatible:       true,
					FirstFailedCheck: "material-match",
				}},
			}},
		))

		if plan.Rows["job-1"] != (smartqueue.Row{Status: "incompatible"}) {
			t.Errorf("row = %+v, want incompatible", plan.Rows["job-1"])
		}
	})

	t.Run("tags-match blocked by checks", func(t *testing.T) {
		plan := smartqueue.BuildPlan(planInput(
			[]smartqueue.PlanJob{job("job-1", 100, ago(time.Hour))},
			map[string]string{"p1": "Alpha"},
			[]smartqueue.PlanGroup{{
				MemberIDs:        []string{"job-1"},
				RepresentativeID: "job-1",
				Verdicts: []smartqueue.Verdict{{
					PrinterID:        "p1",
					FirstFailedCheck: "tags-match",
				}},
				Checks: map[string][]smartqueue.Check{
					"p1": {{
						Code: "tags-match",
						Comparisons: []smartqueue.Comparison{{
							Key:     "filament.type",
							Matched: false,
						}},
					}},
				},
			}},
		))

		if plan.Rows["job-1"] != (smartqueue.Row{Status: "incompatible"}) {
			t.Errorf("row = %+v, want incompatible", plan.Rows["job-1"])
		}
	})
}

func TestBuildPlanMergedJobIsNotAVertex(t *testing.T) {
	cases := []struct {
		name      string
		createdAt string
	}{
		{name: "parseable", createdAt: ago(2 * time.Hour)},
		{name: "unparseable", createdAt: "not-a-timestamp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The child is older and listed first, so a vertex would take the printer.
			plan := smartqueue.BuildPlan(planInput(
				[]smartqueue.PlanJob{
					{ID: "child", ParentJobID: "parent", Priority: 100, CreatedAt: tc.createdAt},
					job("managed", 100, ago(0)),
				},
				map[string]string{"p1": "Alpha"},
				[]smartqueue.PlanGroup{matchedGroup("child", []string{"child", "managed"}, "p1")},
			))

			if plan.Rows["child"] != (smartqueue.Row{Status: "merged"}) {
				t.Errorf("child = %+v, want merged", plan.Rows["child"])
			}
			if plan.Rows["managed"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
				t.Errorf("managed = %+v, want assigned to Alpha", plan.Rows["managed"])
			}
		})
	}
}

func TestBuildPlanUnreadableJobIsNotAVertex(t *testing.T) {
	// Listed first with an edge. A vertex would take the only printer or land on waiting.
	plan := smartqueue.BuildPlan(planInput(
		[]smartqueue.PlanJob{
			job("bad", 100, "not-a-timestamp"),
			job("good", 100, ago(0)),
		},
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("bad", []string{"bad", "good"}, "p1")},
	))

	if plan.Rows["bad"] != (smartqueue.Row{Status: "unreadable"}) {
		t.Errorf("bad = %+v, want unreadable", plan.Rows["bad"])
	}
	if plan.Rows["good"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
		t.Errorf("good = %+v, want assigned to Alpha", plan.Rows["good"])
	}
}

func TestBuildPlanGroupMembersShareEdgesAndKeepWeights(t *testing.T) {
	// The representative is younger and listed first. Its verdict is the only edge source.
	plan := smartqueue.BuildPlan(planInput(
		[]smartqueue.PlanJob{
			job("rep", 100, ago(0)),
			job("old", 100, ago(2*time.Hour)),
		},
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("rep", []string{"rep", "old"}, "p1")},
	))

	if plan.Rows["old"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
		t.Errorf("old = %+v, want assigned to Alpha", plan.Rows["old"])
	}
	if plan.Rows["rep"] != (smartqueue.Row{Status: "waiting"}) {
		t.Errorf("rep = %+v, want waiting", plan.Rows["rep"])
	}
}

func TestBuildPlanPrioritizedMemberKeepsItsMark(t *testing.T) {
	plan := smartqueue.BuildPlan(prioritizedInput(
		[]smartqueue.PlanJob{
			job("old", 100, ago(2*time.Hour)),
			job("young", 100, ago(0)),
		},
		map[string]struct{}{"young": {}},
	))

	if plan.Rows["young"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
		t.Errorf("young = %+v, want assigned to Alpha", plan.Rows["young"])
	}
	if plan.Rows["old"] != (smartqueue.Row{Status: "waiting"}) {
		t.Errorf("old = %+v, want waiting", plan.Rows["old"])
	}
}

func TestBuildPlanLowPriorityWaitsForNormal(t *testing.T) {
	plan := smartqueue.BuildPlan(planInput(
		[]smartqueue.PlanJob{
			job("low", 1000, ago(2*time.Hour)),
			job("normal", 100, ago(0)),
		},
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("low", []string{"low", "normal"}, "p1")},
	))

	if plan.Rows["normal"] != (smartqueue.Row{PrinterID: "p1", PrinterName: "Alpha", Status: "assigned"}) {
		t.Errorf("normal = %+v, want assigned to Alpha", plan.Rows["normal"])
	}
	if plan.Rows["low"] != (smartqueue.Row{Status: "waiting"}) {
		t.Errorf("low = %+v, want waiting", plan.Rows["low"])
	}
}

func planInput(jobs []smartqueue.PlanJob, names map[string]string, groups []smartqueue.PlanGroup) smartqueue.PlanInput {
	return smartqueue.PlanInput{
		Jobs:         jobs,
		PrinterNames: names,
		Groups:       groups,
		HalfLife:     time.Hour,
		Now:          planNow,
	}
}

func prioritizedInput(jobs []smartqueue.PlanJob, prioritized map[string]struct{}) smartqueue.PlanInput {
	in := planInput(
		jobs,
		map[string]string{"p1": "Alpha"},
		[]smartqueue.PlanGroup{matchedGroup("old", []string{"old", "young"}, "p1")},
	)
	in.Prioritized = prioritized
	return in
}

func job(id string, priority int, createdAt string) smartqueue.PlanJob {
	return smartqueue.PlanJob{ID: id, Priority: priority, CreatedAt: createdAt}
}

func matchedGroup(representative string, members []string, printerID string) smartqueue.PlanGroup {
	return smartqueue.PlanGroup{
		MemberIDs:        members,
		RepresentativeID: representative,
		Verdicts: []smartqueue.Verdict{{
			PrinterID: printerID,
			Matched:   true,
		}},
	}
}

var planNow = time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)

func ago(age time.Duration) string {
	return planNow.Add(-age).Format(time.RFC3339)
}
