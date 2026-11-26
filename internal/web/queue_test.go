package web

import (
	"strings"
	"testing"

	"github.com/bandesz/printago-buddy/internal/printago"
)

func TestQueueTemplate_rendersEachJobRow(t *testing.T) {
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
			{
				Job:          job("job-a", "Trains", "Trains - Plate 1"),
				JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red, blue}}},
				Assignment:   "Alpha",
			},
			{
				Job:          job("job-c", "Trains", "Trains - Plate 1"),
				JobMaterials: []JobMaterial{{Colors: []MatchedFilament{red, blue}}},
				Assignment:   "Waiting for a printer",
			},
		},
	}
	var buf strings.Builder
	if err := srv.queueTmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Count(html, ">Trains<") != 2 {
		t.Fatalf("each copy should be its own row:\n%s", html)
	}
	if strings.Contains(html, "job-count") || strings.Contains(html, ">x2<") || strings.Contains(html, ">x1<") {
		t.Fatal("xN badge should be absent")
	}
	if !strings.Contains(html, `name="id" value="job-a"`) || !strings.Contains(html, `name="id" value="job-c"`) {
		t.Fatal("each copy should keep its own actions")
	}
	if !strings.Contains(html, "<th>Assignment</th>") || !strings.Contains(html, ">Alpha<") || !strings.Contains(html, "Waiting for a printer") {
		t.Fatalf("assignment column missing:\n%s", html)
	}
	if strings.Contains(html, "col-order") || strings.Contains(html, ">#<") {
		t.Fatal("queue position column should not be rendered")
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
