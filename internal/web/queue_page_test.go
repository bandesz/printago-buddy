package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestQueuePageDoesNotMatchOnReload(t *testing.T) {
	api := queueFixture()
	srv, sched := newQueuePage(t, api)
	if err := sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	snap := sched.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot() = nil, want a published plan")
	}
	if snap.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	assertSnapRow(t, snap, "copy-old", smartqueue.StatusAssigned, "Alpha")
	assertSnapRow(t, snap, "copy-young", smartqueue.StatusWaiting, "")
	assertSnapRow(t, snap, "nameless", smartqueue.StatusAssigned, "printer-blank")
	assertSnapRow(t, snap, "incompatible", smartqueue.StatusIncompatible, "")
	assertSnapRow(t, snap, "child", smartqueue.StatusMerged, "")
	assertSnapRow(t, snap, "unreadable", smartqueue.StatusUnreadable, "")

	api.resetCalls()
	html := renderQueue(t, srv)
	assertRenderDidNotMatch(t, api.calls)

	if !strings.Contains(html, "<th>Assignment</th>") {
		t.Fatalf("column header Assignment missing:\n%s", html)
	}
	if !strings.Contains(html, "Best Matching Printers") {
		t.Fatalf("best-matching printers column missing while smart queue is off:\n%s", html)
	}
	if !strings.Contains(html, ">Prioritize<") || !strings.Contains(html, ">Cancel<") {
		t.Fatalf("Prioritize or Cancel missing:\n%s", html)
	}
	if !strings.Contains(html, "10000") {
		t.Fatalf("10 second reload missing:\n%s", html)
	}

	got := assignmentCells(t, html)
	want := []string{
		"Alpha",
		"Waiting for a printer",
		"printer-blank",
		"No compatible printer",
		"Part of a merged plate",
		"Unreadable job age",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("assignment cells = %#v, want %#v\n%s", got, want, html)
	}
	if strings.Count(html, ">Widget<") != 2 || strings.Count(html, ">Plate 1<") != 2 || strings.Count(html, ">Crimson<") != 2 {
		t.Fatalf("copies that share a part, label, and color were collapsed:\n%s", html)
	}
	if strings.Contains(html, "job-count") || strings.Contains(html, ">x2<") || strings.Contains(html, ">x1<") {
		t.Fatalf("xN badge present:\n%s", html)
	}
}

func TestQueuePageNilSnapshotShowsCalculating(t *testing.T) {
	api := &recordingClient{jobs: []printago.PrintJob{
		sharedCopy("copy-old", 2*time.Hour),
		sharedCopy("copy-young", time.Hour),
	}}
	srv, sched := newQueuePage(t, api)
	if sched.Snapshot() != nil {
		t.Fatalf("Snapshot() = %+v, want nil", sched.Snapshot())
	}

	html := renderQueue(t, srv)
	assertRenderDidNotMatch(t, api.calls)
	got := assignmentCells(t, html)
	want := []string{"Calculating…", "Calculating…"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("assignment cells = %#v, want %#v\n%s", got, want, html)
	}
	if strings.Contains(got[0], "...") {
		t.Fatal("calculating text uses three dots, want …")
	}
}

func TestEmptyQueuePage(t *testing.T) {
	api := &recordingClient{printers: []printago.Printer{{ID: "p-alpha", Name: "Alpha"}}}
	srv, sched := newQueuePage(t, api)
	if err := sched.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	snap := sched.Snapshot()
	if snap == nil || len(snap.Rows) != 0 || snap.Enabled {
		t.Fatalf("Snapshot() = %+v, want an empty disabled plan", snap)
	}

	api.resetCalls()
	html := renderQueue(t, srv)
	assertRenderDidNotMatch(t, api.calls)
	if !strings.Contains(html, "No pending print jobs.") {
		t.Fatalf("empty queue text missing:\n%s", html)
	}
	if strings.Contains(html, "Calculating") || strings.Contains(html, "<th>Assignment</th>") {
		t.Fatalf("empty queue rendered an assignment column:\n%s", html)
	}
}

func newQueuePage(t *testing.T, api *recordingClient) (*Server, *smartqueue.Scheduler) {
	t.Helper()
	sched := smartqueue.NewScheduler(api, smartqueue.Config{
		HalfLife:  time.Hour,
		StateFile: filepath.Join(t.TempDir(), "smart-queue.json"),
	})
	srv, err := NewServer(api, sched, 0)
	if err != nil {
		t.Fatal(err)
	}
	return srv, sched
}

func renderQueue(t *testing.T, srv *Server) string {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/queue", nil)
	rec := httptest.NewRecorder()
	srv.handleQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /queue status = %d, body:\n%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func assignmentCells(t *testing.T, html string) []string {
	t.Helper()
	const open = `<td class="assignment">`
	var cells []string
	rest := html
	for {
		i := strings.Index(rest, open)
		if i < 0 {
			break
		}
		rest = rest[i+len(open):]
		end := strings.Index(rest, "</td>")
		if end < 0 {
			t.Fatalf("assignment cell is not closed:\n%s", html)
		}
		cells = append(cells, strings.TrimSpace(rest[:end]))
		rest = rest[end+len("</td>"):]
	}
	return cells
}

func assertSnapRow(t *testing.T, snap *smartqueue.Plan, id string, status smartqueue.Status, printerName string) {
	t.Helper()
	row, ok := snap.Rows[id]
	if !ok || row.Status != status || row.PrinterName != printerName {
		t.Fatalf("row %s = %+v, want status %s printer %q", id, row, status, printerName)
	}
}

func assertRenderDidNotMatch(t *testing.T, calls []string) {
	t.Helper()
	forbidden := map[string]struct{}{
		"group-matching":      {},
		"troubleshoot":        {},
		"get-settings":        {},
		"patch-settings":      {},
		"update-tags":         {},
		"update-printer-tags": {},
		"run-queue":           {},
	}
	var hit []string
	for _, call := range calls {
		if _, bad := forbidden[call]; bad {
			hit = append(hit, call)
		}
	}
	if len(hit) > 0 {
		t.Fatalf("render calls = %v, want no group matching, troubleshoot, settings, tag update, or queue run", calls)
	}
}

func queueFixture() *recordingClient {
	color := "#CC0000FF"
	parent := "parent-plate"
	return &recordingClient{
		jobs: []printago.PrintJob{
			sharedCopy("copy-old", 2*time.Hour),
			sharedCopy("copy-young", time.Hour),
			{
				ID:        "nameless",
				PartName:  "Nameless",
				Priority:  100,
				CreatedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			},
			{
				ID:        "incompatible",
				PartName:  "Lonely",
				Priority:  100,
				CreatedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			},
			{
				ID:          "child",
				ParentJobID: &parent,
				PartName:    "Child plate",
				Priority:    100,
				CreatedAt:   time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			},
			{
				ID:        "unreadable",
				PartName:  "Bad clock",
				Priority:  100,
				CreatedAt: "not-a-time",
			},
		},
		printers: []printago.Printer{
			{ID: "p-alpha", Name: "Alpha"},
			{ID: "printer-blank", Name: ""},
		},
		variants: []printago.MaterialVariant{{
			ID:         "var-crimson",
			MaterialID: "mat-pla",
			Name:       "Crimson",
			Color:      &color,
		}},
		materials: []printago.Material{{
			ID:    "mat-pla",
			Name:  "PLA Basic",
			Brand: "Bambu",
			Type:  "PLA",
		}},
		groups: printago.GroupMatchingResult{
			Groups: []printago.MatchGroup{
				{
					RepresentativePrintJobID: "copy-old",
					MemberPrintJobIDs:        []string{"copy-old", "copy-young"},
					Printers: []printago.MatchPrinterVerdict{{
						PrinterID: "p-alpha",
						Matched:   true,
					}},
				},
				{
					RepresentativePrintJobID: "nameless",
					MemberPrintJobIDs:        []string{"nameless"},
					Printers: []printago.MatchPrinterVerdict{{
						PrinterID: "printer-blank",
						Matched:   true,
					}},
				},
			},
		},
	}
}

func sharedCopy(id string, age time.Duration) printago.PrintJob {
	return printago.PrintJob{
		ID:        id,
		PartName:  "Widget",
		Label:     "Plate 1",
		Priority:  100,
		CreatedAt: time.Now().UTC().Add(-age).Format(time.RFC3339),
		MaterialAssignments: printago.MaterialAssignments{
			"0": {{VariantID: "var-crimson", MaterialID: "mat-pla"}},
		},
	}
}

// recordingClient is a Printago client that records calls and serves one queue fixture.
type recordingClient struct {
	calls     []string
	jobs      []printago.PrintJob
	printers  []printago.Printer
	slots     []printago.PrinterSlot
	materials []printago.Material
	variants  []printago.MaterialVariant
	groups    printago.GroupMatchingResult
}

func (c *recordingClient) resetCalls() { c.calls = nil }

func (c *recordingClient) GetPrinters(context.Context) ([]printago.Printer, error) {
	c.calls = append(c.calls, "get-printers")
	return c.printers, nil
}

func (c *recordingClient) GetPrinterSlots(context.Context) ([]printago.PrinterSlot, error) {
	c.calls = append(c.calls, "get-slots")
	return c.slots, nil
}

func (c *recordingClient) GetMaterials(context.Context) ([]printago.Material, error) {
	c.calls = append(c.calls, "get-materials")
	return c.materials, nil
}

func (c *recordingClient) GetMaterialVariants(context.Context) ([]printago.MaterialVariant, error) {
	c.calls = append(c.calls, "get-variants")
	return c.variants, nil
}

func (c *recordingClient) UpdatePrinterTags(context.Context, string, []string) error {
	c.calls = append(c.calls, "update-printer-tags")
	return nil
}

func (c *recordingClient) GetPrintJobs(context.Context) ([]printago.PrintJob, error) {
	c.calls = append(c.calls, "get-jobs")
	return c.jobs, nil
}

func (c *recordingClient) GetPartMaterialAssignments(context.Context, string) ([]printago.PartMaterialAssignment, error) {
	c.calls = append(c.calls, "get-part-assignments")
	return nil, nil
}

func (c *recordingClient) CancelPrintJob(context.Context, string) error {
	c.calls = append(c.calls, "cancel")
	return nil
}

func (c *recordingClient) PrioritizePrintJob(context.Context, string) error {
	c.calls = append(c.calls, "prioritize")
	return nil
}

func (c *recordingClient) UpdatePrintJobTags(context.Context, string, map[string]string) error {
	c.calls = append(c.calls, "update-tags")
	return nil
}

func (c *recordingClient) GetStoreSettings(context.Context) (printago.StoreSettings, error) {
	c.calls = append(c.calls, "get-settings")
	return printago.StoreSettings{}, nil
}

func (c *recordingClient) UpdateStoreSettings(context.Context, printago.StoreSettingsUpdate) error {
	c.calls = append(c.calls, "patch-settings")
	return nil
}

func (c *recordingClient) GroupMatching(context.Context, []string) (printago.GroupMatchingResult, error) {
	c.calls = append(c.calls, "group-matching")
	return c.groups, nil
}

func (c *recordingClient) MatchingTroubleshoot(context.Context, string) (printago.MatchingTroubleshootResult, error) {
	c.calls = append(c.calls, "troubleshoot")
	return printago.MatchingTroubleshootResult{}, nil
}

func (c *recordingClient) RunQueue(context.Context) error {
	c.calls = append(c.calls, "run-queue")
	return nil
}
