package web

import (
	"strings"
	"testing"

	"github.com/bandesz/printago-buddy/internal/printago"
)

func TestGroupQueueEntries_collapsesSameJob(t *testing.T) {
	red := filament("Red", "#ff0000")
	blue := filament("Blue", "#0000ff")
	entries := []queueEntry{
		{Job: job("job-a", "Trains", "Trains - Plate 1"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red}}}},
		{Job: job("job-b", "Hexagon", "Hexagon bottom"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{blue}}}},
		{Job: job("job-c", "Trains", "Trains - Plate 1"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red}}}},
		{Job: job("job-d", "Trains", "Trains - Plate 1"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{blue}}}},
		{Job: job("job-e", "Trains", "Other label"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red}}}},
	}

	got := groupQueueEntries(entries)
	if len(got) != 4 {
		t.Fatalf("rows = %d, want 4", len(got))
	}
	if got[0].Count != 2 || got[0].Job.ID != "job-a" {
		t.Fatalf("first row = id %s count %d, want job-a x2", got[0].Job.ID, got[0].Count)
	}
	if got[1].Count != 1 || got[1].Job.ID != "job-b" {
		t.Fatalf("second row = id %s count %d, want job-b x1", got[1].Job.ID, got[1].Count)
	}
	if got[2].Count != 1 || got[2].Job.ID != "job-d" {
		t.Fatalf("third row = id %s count %d, want the blue Trains job", got[2].Job.ID, got[2].Count)
	}
	if got[3].Count != 1 || got[3].Job.ID != "job-e" {
		t.Fatalf("fourth row = id %s count %d, want the other label", got[3].Job.ID, got[3].Count)
	}
}

func TestQueueTemplate_showsCountOnGroupedRow(t *testing.T) {
	srv, err := NewServer(nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if srv.scheduler != nil {
		t.Fatal("scheduler = non-nil, want the nil scheduler passed to NewServer")
	}
	red := filament("Red", "#ff0000")
	blue := filament("Blue", "#0000ff")
	data := queuePageData{
		Page: "queue",
		NormalEntries: []queueEntry{
			{Count: 2, Job: job("job-a", "Trains", "Trains - Plate 1"), JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red, blue}}}},
			{Count: 1, Job: job("job-b", "Hexagon", "Hexagon bottom")},
		},
	}
	var buf strings.Builder
	if err := srv.queueTmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Count(html, "Trains") < 1 || !strings.Contains(html, ">x2<") {
		t.Fatalf("grouped row missing x2 label:\n%s", html)
	}
	if strings.Contains(html, ">x1<") {
		t.Fatal("single job should not show a count")
	}
	if !strings.Contains(html, `name="id" value="job-a"`) {
		t.Fatal("actions should target the earliest job in the group")
	}
	if strings.Contains(html, "col-order") || strings.Contains(html, ">#<") {
		t.Fatal("queue position column should not be rendered")
	}
	if strings.Contains(html, "Assignment") {
		t.Fatal("assignment column should not be rendered")
	}
	line := materialLine(html)
	if !strings.Contains(line, "Red") || !strings.Contains(line, "Blue") || !strings.Contains(line, "or") {
		t.Fatalf("alternative colors should share one materials line, got %q", line)
	}
	name := html[strings.Index(html, `class="job-name"`):strings.Index(html, `class="job-label"`)]
	if strings.Contains(name, "filament-chip") || strings.Contains(name, "Red") {
		t.Fatalf("job name should not list materials, got %q", name)
	}
}

func materialLine(html string) string {
	start := strings.Index(html, `class="material-line"`)
	if start < 0 {
		return ""
	}
	rest := html[start:]
	end := strings.Index(rest, "</div>")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func job(id, name, label string) printago.PrintJob {
	return printago.PrintJob{ID: id, PartName: name, Label: label}
}

func filament(name, color string) MatchedFilament {
	return MatchedFilament{Name: name, ColorCSS: variantColorToCSS(color), Tooltip: name}
}
