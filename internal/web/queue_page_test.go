package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestQueueBanner(t *testing.T) {
	api := &recordingClient{jobs: []printago.PrintJob{sharedCopy("copy-old", time.Hour)}}

	t.Run("plan error", func(t *testing.T) {
		srv := newBannerServer(t, api, &smartqueue.Plan{Err: "poll failed"})
		html := renderQueue(t, srv)
		assertBannerTexts(t, html, "poll failed")
	})

	t.Run("query error", func(t *testing.T) {
		srv := newBannerServer(t, api, &smartqueue.Plan{})
		html := renderQueueURL(t, srv, "/queue?error="+url.QueryEscape("disk full"))
		assertBannerTexts(t, html, "disk full")
	})

	t.Run("both differ", func(t *testing.T) {
		srv := newBannerServer(t, api, &smartqueue.Plan{Err: "poll failed"})
		html := renderQueueURL(t, srv, "/queue?error="+url.QueryEscape("disk full"))
		assertBannerTexts(t, html, "disk full", "poll failed")
	})

	t.Run("both equal", func(t *testing.T) {
		srv := newBannerServer(t, api, &smartqueue.Plan{Err: "disk full"})
		html := renderQueueURL(t, srv, "/queue?error="+url.QueryEscape("disk full"))
		assertBannerTexts(t, html, "disk full")
	})

	t.Run("nil snapshot query", func(t *testing.T) {
		srv := newBannerServer(t, api, nil)
		html := renderQueueURL(t, srv, "/queue?error="+url.QueryEscape("disk full"))
		assertBannerTexts(t, html, "disk full")
	})
}

const (
	confirmTurnOn  = "Turn on smart queue? Printago will stop automatic matching, and Buddy will choose which job each printer runs."
	confirmTurnOff = "Turn off smart queue? This removes printer assignments and park tags and turns Printago automatic matching back on."
)

func TestSmartQueueToggle(t *testing.T) {
	t.Run("nil snapshot", func(t *testing.T) {
		api := queueFixture()
		srv, sched := newQueuePage(t, api)
		if sched.Snapshot() != nil {
			t.Fatalf("Snapshot() = %+v, want nil", sched.Snapshot())
		}
		html := renderQueue(t, srv)
		assertSmartQueueToggle(t, html, "Smart queue: Off", "on", confirmTurnOn)
		if !strings.Contains(html, "Best Matching Printers") || !strings.Contains(html, ">Prioritize<") {
			t.Fatalf("best-matching printers or Prioritize missing while smart queue is off:\n%s", html)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		api := queueFixture()
		srv, sched := newQueuePage(t, api)
		if err := sched.Poll(context.Background()); err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		snap := sched.Snapshot()
		if snap == nil || snap.Enabled {
			t.Fatalf("Snapshot() = %+v, want disabled", snap)
		}
		html := renderQueue(t, srv)
		assertSmartQueueToggle(t, html, "Smart queue: Off", "on", confirmTurnOn)
		if !strings.Contains(html, "Best Matching Printers") {
			t.Fatalf("best-matching printers column missing while smart queue is off:\n%s", html)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		srv, sched := enabledQueuePage(t, queueFixture())
		if snap := sched.Snapshot(); snap == nil || !snap.Enabled {
			t.Fatalf("Snapshot() = %+v, want enabled", snap)
		}
		html := renderQueue(t, srv)
		assertSmartQueueToggle(t, html, "Smart queue: On", "off", confirmTurnOff)
	})
}

func TestSmartQueuePost(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, nil)
		rec := postSmartQueue(t, srv, "on")
		assertSeeOther(t, rec, "/queue")
		if stub.enables != 1 || stub.disables != 0 {
			t.Fatalf("Enable calls = %d, Disable calls = %d, want Enable once", stub.enables, stub.disables)
		}
		assertNoPrintago(t, api)
	})

	t.Run("off", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, nil)
		rec := postSmartQueue(t, srv, "off")
		assertSeeOther(t, rec, "/queue")
		if stub.disables != 1 || stub.enables != 0 {
			t.Fatalf("Enable calls = %d, Disable calls = %d, want Disable once", stub.enables, stub.disables)
		}
		assertNoPrintago(t, api)
	})

	t.Run("invalid", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, nil)
		rec := postSmartQueue(t, srv, "maybe")
		assertSeeOther(t, rec, "/queue?error=invalid%20smart%20queue%20value")
		if stub.enables != 0 || stub.disables != 0 {
			t.Fatalf("Enable calls = %d, Disable calls = %d, want neither", stub.enables, stub.disables)
		}
		assertNoPrintago(t, api)
	})

	t.Run("enable error", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, errors.New("disk full"))
		rec := postSmartQueue(t, srv, "on")
		assertSeeOther(t, rec, "/queue?error=Smart%20queue%20update%20failed%3A%20disk%20full")
		if stub.enables != 1 || stub.disables != 0 {
			t.Fatalf("Enable calls = %d, Disable calls = %d, want Enable once", stub.enables, stub.disables)
		}
		assertNoPrintago(t, api)
	})

	t.Run("disable error", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, errors.New("tags stuck"))
		rec := postSmartQueue(t, srv, "off")
		assertSeeOther(t, rec, "/queue?error=Smart%20queue%20update%20failed%3A%20tags%20stuck")
		if stub.disables != 1 || stub.enables != 0 {
			t.Fatalf("Enable calls = %d, Disable calls = %d, want Disable once", stub.enables, stub.disables)
		}
		assertNoPrintago(t, api)
	})

	t.Run("nil scheduler", func(t *testing.T) {
		api := &recordingClient{jobs: []printago.PrintJob{sharedCopy("copy-old", time.Hour)}}
		srv, err := NewServer(api, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		html := renderQueue(t, srv)
		got := assignmentCells(t, html)
		if len(got) != 1 || got[0] != "Calculating…" {
			t.Fatalf("assignment cells = %#v, want Calculating…", got)
		}
	})
}

func TestPrioritizePost(t *testing.T) {
	const jobID = "abcdef0123456789abcdef01"

	t.Run("valid", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, nil)
		rec := postPrioritize(t, srv, jobID)
		assertSeeOther(t, rec, "/queue")
		if len(stub.prioritized) != 1 || stub.prioritized[0] != jobID {
			t.Fatalf("Prioritize calls = %v, want [%s]", stub.prioritized, jobID)
		}
		assertNoPrintago(t, api)
	})

	t.Run("error", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, errors.New("disk full"))
		rec := postPrioritize(t, srv, jobID)
		assertSeeOther(t, rec, "/queue?error=Failed%20to%20prioritize%20job%3A%20disk%20full")
		if len(stub.prioritized) != 1 || stub.prioritized[0] != jobID {
			t.Fatalf("Prioritize calls = %v, want [%s]", stub.prioritized, jobID)
		}
		assertNoPrintago(t, api)
	})

	t.Run("invalid", func(t *testing.T) {
		api, stub, srv := smartQueuePostFixture(t, nil)
		rec := postPrioritize(t, srv, "nope")
		assertSeeOther(t, rec, "/queue?error=invalid+job+id")
		if len(stub.prioritized) != 0 {
			t.Fatalf("Prioritize calls = %v, want none", stub.prioritized)
		}
		assertNoPrintago(t, api)
	})
}

type stubScheduler struct {
	enables     int
	disables    int
	prioritized []string
	err         error
	plan        *smartqueue.Plan
	halfLife    string
}

func (s *stubScheduler) Snapshot() *smartqueue.Plan { return s.plan }

func (s *stubScheduler) Enable(context.Context) error {
	s.enables++
	return s.err
}

func (s *stubScheduler) Disable(context.Context) error {
	s.disables++
	return s.err
}

func (s *stubScheduler) Prioritize(_ context.Context, jobID string) error {
	s.prioritized = append(s.prioritized, jobID)
	return s.err
}

func (s *stubScheduler) HalfLife() string {
	if s.halfLife == "" {
		return "1h"
	}
	return s.halfLife
}

func (s *stubScheduler) SetHalfLife(_ context.Context, raw string) error {
	s.halfLife = raw
	return s.err
}

func smartQueuePostFixture(t *testing.T, updateErr error) (*recordingClient, *stubScheduler, *Server) {
	t.Helper()
	api := &recordingClient{}
	stub := &stubScheduler{err: updateErr}
	return api, stub, newSwitchServer(t, api, stub)
}

func assertSeeOther(t *testing.T, rec *httptest.ResponseRecorder, loc string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body:\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != loc {
		t.Fatalf("Location = %q, want %q", got, loc)
	}
}

func assertNoPrintago(t *testing.T, api *recordingClient) {
	t.Helper()
	if len(api.calls) != 0 {
		t.Fatalf("Printago calls = %v, want none", api.calls)
	}
}

func newSwitchServer(t *testing.T, api printago.ClientInterface, sched queueScheduler) *Server {
	t.Helper()
	srv, err := NewServer(api, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	srv.scheduler = sched
	return srv
}

func postSmartQueue(t *testing.T, srv *Server, enabled string) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(t, srv, "/smart-queue", url.Values{"enabled": {enabled}})
}

func postPrioritize(t *testing.T, srv *Server, jobID string) *httptest.ResponseRecorder {
	t.Helper()
	return postForm(t, srv, "/prioritize-job", url.Values{"id": {jobID}})
}

func postForm(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	return rec
}

func TestBestMatching(t *testing.T) {
	api := queueFixture()
	api.jobs = append(api.jobs, printago.PrintJob{
		ID:        "low-job",
		PartName:  "Filler",
		Priority:  1000,
		CreatedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})
	srv, sched := enabledQueuePage(t, api)
	if snap := sched.Snapshot(); snap == nil || !snap.Enabled {
		t.Fatalf("Snapshot() = %+v, want enabled", snap)
	}

	html := renderQueue(t, srv)
	split := strings.Index(html, "Low priority")
	if split < 0 || !strings.Contains(html[:split], ">Widget<") || !strings.Contains(html[split:], ">Filler<") {
		t.Fatalf("normal or low-priority section missing:\n%s", html)
	}
	for _, section := range []string{html[:split], html[split:]} {
		if strings.Contains(section, "Best Matching Printers") || strings.Contains(section, `class="printer-list"`) {
			t.Fatalf("best-matching printers column present while smart queue is on:\n%s", html)
		}
		if !strings.Contains(section, ">Prioritize<") {
			t.Fatalf("Prioritize button missing while smart queue is on:\n%s", html)
		}
	}
}

func enabledQueuePage(t *testing.T, api *recordingClient) (*Server, *smartqueue.Scheduler) {
	t.Helper()
	srv, sched := newQueuePage(t, api)
	if err := sched.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	return srv, sched
}

func assertSmartQueueToggle(t *testing.T, html, label, enabled, confirm string) {
	t.Helper()
	form := elementByID(t, html, "smart-queue-toggle")
	if !strings.Contains(form, `method="POST"`) || !strings.Contains(form, `action="/smart-queue"`) {
		t.Fatalf("smart queue toggle form = %s, want POST /smart-queue", form)
	}
	if !strings.Contains(form, label) {
		t.Fatalf("smart queue toggle = %s, want label %q", form, label)
	}
	other := "Smart queue: On"
	if label == other {
		other = "Smart queue: Off"
	}
	if strings.Contains(form, other) {
		t.Fatalf("smart queue toggle contains %q:\n%s", other, form)
	}
	hidden := `<input type="hidden" name="enabled" value="` + enabled + `">`
	if !strings.Contains(form, hidden) {
		t.Fatalf("hidden enabled input missing %s:\n%s", hidden, form)
	}
	if !strings.Contains(form, "confirm('"+confirm+"')") {
		t.Fatalf("confirm text = %s, want %q", form, confirm)
	}
}

func elementByID(t *testing.T, html, id string) string {
	t.Helper()
	marker := `id="` + id + `"`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("%s missing:\n%s", marker, html)
	}
	start := strings.LastIndex(html[:i], "<form")
	if start < 0 {
		t.Fatalf("%s is not on a form:\n%s", marker, html)
	}
	endRel := strings.Index(html[i:], "</form>")
	if endRel < 0 {
		t.Fatalf("%s form is not closed:\n%s", marker, html)
	}
	return html[start : i+endRel+len("</form>")]
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
	return renderQueueURL(t, srv, "/queue")
}

func renderQueueURL(t *testing.T, srv *Server, rawURL string) string {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	rec := httptest.NewRecorder()
	srv.handleQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body:\n%s", rawURL, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func newBannerServer(t *testing.T, api *recordingClient, plan *smartqueue.Plan) *Server {
	t.Helper()
	return newSwitchServer(t, api, &stubScheduler{plan: plan})
}

func assertBannerTexts(t *testing.T, html string, texts ...string) {
	t.Helper()
	if n := strings.Count(html, `<div class="error">`); n != 1 {
		t.Fatalf(".error count = %d, want 1:\n%s", n, html)
	}
	banner := errorBanner(t, html)
	for _, text := range texts {
		if strings.Count(banner, text) != 1 {
			t.Fatalf("banner = %q, want %q once", banner, text)
		}
	}
}

func errorBanner(t *testing.T, html string) string {
	t.Helper()
	const open = `<div class="error">`
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatalf("error banner missing:\n%s", html)
	}
	rest := html[i+len(open):]
	end := strings.Index(rest, "</div>")
	if end < 0 {
		t.Fatalf("error banner is not closed:\n%s", html)
	}
	return rest[:end]
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

func TestQueueHalfLife(t *testing.T) {
	api := &recordingClient{}
	srv, sched := newQueuePage(t, api)

	html := renderQueue(t, srv)
	form := elementByID(t, html, "half-life")
	if !strings.Contains(form, `action="/half-life"`) || !strings.Contains(form, `value="1h"`) {
		t.Fatalf("half-life form = %s, want action /half-life and value 1h", form)
	}

	rec := postForm(t, srv, "/half-life", url.Values{"halfLife": {"30m"}})
	assertSeeOther(t, rec, "/queue")
	if sched.HalfLife() != "30m" {
		t.Fatalf("HalfLife() = %q, want 30m", sched.HalfLife())
	}
	html = renderQueue(t, srv)
	if !strings.Contains(elementByID(t, html, "half-life"), `value="30m"`) {
		t.Fatalf("saved half-life missing from page:\n%s", html)
	}

	rec = postForm(t, srv, "/half-life", url.Values{"halfLife": {"abc"}})
	assertSeeOther(t, rec, "/queue?error=Invalid%20half-life%3A%20abc")
	if sched.HalfLife() != "30m" {
		t.Fatalf("HalfLife() = %q, want 30m after invalid save", sched.HalfLife())
	}
	html = renderQueueURL(t, srv, rec.Header().Get("Location"))
	assertBannerTexts(t, html, "Invalid half-life: abc")
	if !strings.Contains(elementByID(t, html, "half-life"), `value="30m"`) {
		t.Fatalf("invalid half-life changed the saved value:\n%s", html)
	}

	for _, raw := range []string{"", "0", "-1s"} {
		rec = postForm(t, srv, "/half-life", url.Values{"halfLife": {raw}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("POST halfLife=%q status = %d, want 303", raw, rec.Code)
		}
		if sched.HalfLife() != "30m" {
			t.Fatalf("HalfLife() = %q after %q, want 30m", sched.HalfLife(), raw)
		}
	}
}
