package web_test

import (
	"testing"

	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/web"
)

func ptr(s string) *string { return &s }

func printer(id, name string) printago.Printer {
	return printago.Printer{ID: id, Name: name}
}

func assignment(variantID, materialID string) printago.PartMaterialAssignment {
	return printago.PartMaterialAssignment{VariantID: variantID, MaterialID: materialID, MaterialType: "PLA"}
}

func slotFor(printerID, variantID, materialID string) printago.PrinterSlot {
	s := printago.PrinterSlot{ID: "s-" + printerID, PrinterID: printerID}
	if variantID != "" {
		s.VariantID = ptr(variantID)
	}
	if materialID != "" {
		s.MaterialID = ptr(materialID)
	}
	return s
}

func slotMap(ss ...printago.PrinterSlot) map[string][]printago.PrinterSlot {
	m := make(map[string][]printago.PrinterSlot)
	for _, s := range ss {
		m[s.PrinterID] = append(m[s.PrinterID], s)
	}
	return m
}

func TestRankPrinters_fullMatch(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{assignment("var-transparent", "mat-pla-plus")}
	printers := []printago.Printer{printer("p1", "P1S003"), printer("p2", "P1S001")}
	sbp := slotMap(
		slotFor("p1", "var-transparent", "mat-pla-plus"),
		slotFor("p2", "", ""),
	)

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if got[0].Printer.ID != "p1" {
		t.Errorf("got printer %q, want p1", got[0].Printer.ID)
	}
	if !got[0].IsFull() {
		t.Error("expected full match")
	}
}

func TestRankPrinters_partialMatch(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{
		assignment("var-a", "mat-a"),
		assignment("var-b", "mat-b"),
	}
	printers := []printago.Printer{printer("p1", "Alpha")}
	sbp := slotMap(slotFor("p1", "var-a", "mat-a"))

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if got[0].IsFull() {
		t.Error("expected partial match, got full")
	}
	if got[0].Matched != 1 || got[0].Total != 2 {
		t.Errorf("matched=%d total=%d, want 1/2", got[0].Matched, got[0].Total)
	}
}

func TestRankPrinters_noRequirements(t *testing.T) {
	printers := []printago.Printer{
		printer("p4", "Delta"), printer("p1", "Alpha"),
		printer("p3", "Beta"), printer("p2", "Gamma"),
	}

	got := web.RankPrinters(nil, printers, nil, nil, nil)

	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3", len(got))
	}
	for _, c := range got {
		if !c.IsFull() {
			t.Errorf("printer %q: expected full match", c.Printer.Name)
		}
	}
	want := []string{"Alpha", "Beta", "Delta"}
	for i, w := range want {
		if got[i].Printer.Name != w {
			t.Errorf("position %d: got %q, want %q", i, got[i].Printer.Name, w)
		}
	}
}

func TestRankPrinters_maxThree(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{assignment("var-a", "mat-a")}
	printers := []printago.Printer{
		printer("p1", "A"), printer("p2", "B"), printer("p3", "C"), printer("p4", "D"),
	}
	sbp := slotMap(
		slotFor("p1", "var-a", "mat-a"), slotFor("p2", "var-a", "mat-a"),
		slotFor("p3", "var-a", "mat-a"), slotFor("p4", "var-a", "mat-a"),
	)

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) > 3 {
		t.Errorf("got %d candidates, want at most 3", len(got))
	}
}

func TestRankPrinters_fullBeforePartial(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{
		assignment("var-a", "mat-a"),
		assignment("var-b", "mat-b"),
	}
	printers := []printago.Printer{printer("p1", "Partial"), printer("p2", "Full")}
	sbp := slotMap(
		slotFor("p1", "var-a", "mat-a"),
		slotFor("p2", "var-a", "mat-a"), slotFor("p2", "var-b", "mat-b"),
	)

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got))
	}
	if got[0].Printer.ID != "p2" {
		t.Errorf("first should be full match (p2), got %q", got[0].Printer.ID)
	}
}

func TestRankPrinters_noMatch(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{assignment("var-a", "mat-a")}
	printers := []printago.Printer{printer("p1", "Alpha")}
	sbp := slotMap(slotFor("p1", "var-other", "mat-other"))

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 0 {
		t.Errorf("got %d candidates, want 0", len(got))
	}
}

// When a variantId is specified in the assignment the printer slot must carry
// that exact variant; materialId alone is not sufficient.
func TestRankPrinters_variantIDRequiredWhenSet(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{assignment("var-a", "mat-a")}
	printers := []printago.Printer{printer("p1", "Alpha")}
	sbp := slotMap(slotFor("p1", "", "mat-a")) // slot has materialId but not variantId

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 0 {
		t.Errorf("got %d candidates, want 0: variantId mismatch should not fall back to materialId", len(got))
	}
}

// When no variantId is set in the assignment, materialId serves as the
// coarser fallback (brand + type, any color).
func TestRankPrinters_materialIDFallbackWhenNoVariant(t *testing.T) {
	assignments := []printago.PartMaterialAssignment{assignment("", "mat-a")}
	printers := []printago.Printer{printer("p1", "Alpha")}
	sbp := slotMap(slotFor("p1", "", "mat-a"))

	got := web.RankPrinters(assignments, printers, sbp, nil, nil)

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if !got[0].IsFull() {
		t.Error("expected full match via materialId fallback")
	}
}
