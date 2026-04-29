package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bandesz/printago-buddy/internal/jobs"
	"github.com/bandesz/printago-buddy/internal/printago"
)

// ---- mock client --------------------------------------------------------

type updateTagsCall struct {
	printerID string
	tags      []string
}

type mockClient struct {
	printers        []printago.Printer
	slots           []printago.PrinterSlot
	materials       []printago.Material
	variants        []printago.MaterialVariant
	updateTagsCalls []updateTagsCall
	// Set to force an error on the nth UpdatePrinterTags call (0-indexed).
	updateTagsErrOn map[int]error
}

func (m *mockClient) GetPrinters(_ context.Context) ([]printago.Printer, error) {
	return m.printers, nil
}
func (m *mockClient) GetPrinterSlots(_ context.Context) ([]printago.PrinterSlot, error) {
	return m.slots, nil
}
func (m *mockClient) GetMaterials(_ context.Context) ([]printago.Material, error) {
	return m.materials, nil
}
func (m *mockClient) GetMaterialVariants(_ context.Context) ([]printago.MaterialVariant, error) {
	return m.variants, nil
}
func (m *mockClient) GetPrintJobs(_ context.Context) ([]printago.PrintJob, error) {
	return nil, nil
}
func (m *mockClient) GetPartMaterialAssignments(_ context.Context, _ string) ([]printago.PartMaterialAssignment, error) {
	return nil, nil
}
func (m *mockClient) CancelPrintJob(_ context.Context, _ string) error     { return nil }
func (m *mockClient) PrioritizePrintJob(_ context.Context, _ string) error { return nil }
func (m *mockClient) UpdatePrinterTags(_ context.Context, printerID string, tags []string) error {
	idx := len(m.updateTagsCalls)
	m.updateTagsCalls = append(m.updateTagsCalls, updateTagsCall{printerID: printerID, tags: tags})
	if err, ok := m.updateTagsErrOn[idx]; ok {
		return err
	}
	return nil
}

// ptr returns a pointer to a string literal, handy for optional IDs in tests.
func ptr(s string) *string { return &s }

// ---- NormaliseTag --------------------------------------------------------
// normaliseTag is an unexported function, so we test it indirectly through
// the observable tag output of Run().

// ---- Run() ---------------------------------------------------------------

func TestRun_updatesTags(t *testing.T) {
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	call := mc.updateTagsCalls[0]
	if call.printerID != "p1" {
		t.Errorf("printerID = %q, want p1", call.printerID)
	}
	if len(call.tags) != 1 || call.tags[0] != "filament_pla_basic_magenta" {
		t.Errorf("tags = %v, want [filament_pla_basic_magenta]", call.tags)
	}
}

func TestRun_skipsUpdateWhenTagsUnchanged(t *testing.T) {
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{"filament_pla_basic_magenta"}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 0 {
		t.Errorf("UpdatePrinterTags called %d times, want 0 (no change)", len(mc.updateTagsCalls))
	}
}

func TestRun_preservesNonFilamentTags(t *testing.T) {
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{"custom_tag", "another_tag"}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	tags := mc.updateTagsCalls[0].tags
	hasCustom := false
	hasAnother := false
	hasFilament := false
	for _, tag := range tags {
		switch tag {
		case "custom_tag":
			hasCustom = true
		case "another_tag":
			hasAnother = true
		case "filament_pla_basic_magenta":
			hasFilament = true
		}
	}
	if !hasCustom || !hasAnother {
		t.Errorf("non-filament tags not preserved; got %v", tags)
	}
	if !hasFilament {
		t.Errorf("filament tag missing; got %v", tags)
	}
}

func TestRun_removesStaleFilamentTags(t *testing.T) {
	// Printer currently tagged with an old filament; slot now has a different one.
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{"filament_petg_basic_white"}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	tags := mc.updateTagsCalls[0].tags
	for _, tag := range tags {
		if tag == "filament_petg_basic_white" {
			t.Errorf("stale tag filament_petg_basic_white should have been removed; got %v", tags)
		}
	}
}

func TestRun_deduplicatesFilamentTags(t *testing.T) {
	// Two AMS slots loaded with the same filament — should produce a single tag.
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1"), AmsIndex: 0, SlotIndex: 0},
			{ID: "s2", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1"), AmsIndex: 0, SlotIndex: 1},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	if len(mc.updateTagsCalls[0].tags) != 1 {
		t.Errorf("expected 1 deduplicated tag, got %v", mc.updateTagsCalls[0].tags)
	}
}

func TestRun_skipsSlotWithNoMaterial(t *testing.T) {
	// Slot has no materialId or variantId — should produce no filament tag.
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: nil, VariantID: nil},
		},
		materials: []printago.Material{},
		variants:  []printago.MaterialVariant{},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	// No tags to set, so no update should happen.
	if len(mc.updateTagsCalls) != 0 {
		t.Errorf("UpdatePrinterTags called %d times, want 0 (no material in slot)", len(mc.updateTagsCalls))
	}
}

func TestRun_continuesAfterPerPrinterError(t *testing.T) {
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
			{ID: "p2", Name: "Printer 2", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
			{ID: "s2", PrinterID: "p2", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
		// First PATCH fails; the job should still attempt the second printer.
		updateTagsErrOn: map[int]error{0: errors.New("API error")},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 2 {
		t.Errorf("UpdatePrinterTags called %d times, want 2 (both printers attempted)", len(mc.updateTagsCalls))
	}
}

func TestRun_materialOnlySlot(t *testing.T) {
	// Slot has a materialId but no variantId — tag should derive from material name only.
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: nil},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "ABS", Brand: "Generic", Type: "ABS"},
		},
		variants: []printago.MaterialVariant{},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	tags := mc.updateTagsCalls[0].tags
	if len(tags) != 1 || tags[0] != "filament_abs" {
		t.Errorf("tags = %v, want [filament_abs]", tags)
	}
}

func TestRun_normalisesSpecialCharacters(t *testing.T) {
	// "PLA+" should normalise to "filament_pla" (the + collapses to _).
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: nil},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA+", Brand: "Generic", Type: "PLA"},
		},
		variants: []printago.MaterialVariant{},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 1 {
		t.Fatalf("UpdatePrinterTags called %d times, want 1", len(mc.updateTagsCalls))
	}
	tags := mc.updateTagsCalls[0].tags
	if len(tags) != 1 || tags[0] != "filament_pla" {
		t.Errorf("tags = %v, want [filament_pla]", tags)
	}
}

func TestRun_multiplePrinters(t *testing.T) {
	mc := &mockClient{
		printers: []printago.Printer{
			{ID: "p1", Name: "Printer 1", Tags: []string{}},
			{ID: "p2", Name: "Printer 2", Tags: []string{}},
		},
		slots: []printago.PrinterSlot{
			{ID: "s1", PrinterID: "p1", MaterialID: ptr("mat1"), VariantID: ptr("var1")},
			{ID: "s2", PrinterID: "p2", MaterialID: ptr("mat2"), VariantID: nil},
		},
		materials: []printago.Material{
			{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
			{ID: "mat2", Name: "PETG HF", Brand: "Bambu", Type: "PETG"},
		},
		variants: []printago.MaterialVariant{
			{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
		},
	}

	jobs.NewFilamentTaggerJob(mc).Run()

	if len(mc.updateTagsCalls) != 2 {
		t.Fatalf("UpdatePrinterTags called %d times, want 2", len(mc.updateTagsCalls))
	}

	byPrinter := map[string][]string{}
	for _, c := range mc.updateTagsCalls {
		byPrinter[c.printerID] = c.tags
	}

	if tags := byPrinter["p1"]; len(tags) != 1 || tags[0] != "filament_pla_basic_magenta" {
		t.Errorf("p1 tags = %v, want [filament_pla_basic_magenta]", tags)
	}
	if tags := byPrinter["p2"]; len(tags) != 1 || tags[0] != "filament_petg_hf" {
		t.Errorf("p2 tags = %v, want [filament_petg_hf]", tags)
	}
}
