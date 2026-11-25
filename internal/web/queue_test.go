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
		{Index: 1, Job: job("job-a", "Trains", "Trains - Plate 1"), JobFilaments: []MatchedFilament{red}},
		{Index: 2, Job: job("job-b", "Hexagon", "Hexagon bottom"), JobFilaments: []MatchedFilament{blue}},
		{Index: 4, Job: job("job-c", "Trains", "Trains - Plate 1"), JobFilaments: []MatchedFilament{red}},
		{Index: 5, Job: job("job-d", "Trains", "Trains - Plate 1"), JobFilaments: []MatchedFilament{blue}},
		{Index: 6, Job: job("job-e", "Trains", "Other label"), JobFilaments: []MatchedFilament{red}},
	}

	got := groupQueueEntries(entries)
	if len(got) != 4 {
		t.Fatalf("rows = %d, want 4", len(got))
	}
	if got[0].Count != 2 || got[0].Job.ID != "job-a" || got[0].Index != 1 {
		t.Fatalf("first row = id %s count %d index %d, want job-a x2 at index 1", got[0].Job.ID, got[0].Count, got[0].Index)
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
	srv, err := NewServer(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	data := queuePageData{
		Page: "queue",
		NormalEntries: []queueEntry{
			{Index: 1, Count: 2, Job: job("job-a", "Trains", "Trains - Plate 1")},
			{Index: 3, Count: 1, Job: job("job-b", "Hexagon", "Hexagon bottom")},
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
}

func job(id, name, label string) printago.PrintJob {
	return printago.PrintJob{ID: id, PartName: name, Label: label}
}

func filament(name, color string) MatchedFilament {
	return MatchedFilament{Name: name, ColorCSS: variantColorToCSS(color), Tooltip: name}
}
