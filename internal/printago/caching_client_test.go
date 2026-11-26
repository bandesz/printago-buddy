package printago_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
)

// stubClient is a minimal ClientInterface implementation that counts calls and
// returns preset data. It is used to verify that the caching layer hits the
// underlying client only when expected.
type stubClient struct {
	printerCalls         int
	printerSlotCalls     int
	materialCalls        int
	materialVariantCalls int
	partAssignmentCalls  map[string]int
	storeSettingsCalls   int
	storeSettingsUpdates []printago.StoreSettingsUpdate
	printJobTagUpdates   []printJobTagUpdate
	groupMatchingCalls   [][]string
	groupMatching        printago.GroupMatchingResult

	printers         []printago.Printer
	printerSlots     []printago.PrinterSlot
	materials        []printago.Material
	materialVariants []printago.MaterialVariant
	partAssignments  map[string][]printago.PartMaterialAssignment
}

func (s *stubClient) GetPrinters(_ context.Context) ([]printago.Printer, error) {
	s.printerCalls++
	return s.printers, nil
}
func (s *stubClient) GetPrinterSlots(_ context.Context) ([]printago.PrinterSlot, error) {
	s.printerSlotCalls++
	return s.printerSlots, nil
}
func (s *stubClient) UpdatePrinterTags(_ context.Context, _ string, _ []string) error {
	return nil
}
func (s *stubClient) GetPrintJobs(_ context.Context) ([]printago.PrintJob, error) {
	return nil, nil
}

func (s *stubClient) GetMaterials(_ context.Context) ([]printago.Material, error) {
	s.materialCalls++
	return s.materials, nil
}

func (s *stubClient) GetMaterialVariants(_ context.Context) ([]printago.MaterialVariant, error) {
	s.materialVariantCalls++
	return s.materialVariants, nil
}

func (s *stubClient) GetPartMaterialAssignments(_ context.Context, partID string) ([]printago.PartMaterialAssignment, error) {
	if s.partAssignmentCalls == nil {
		s.partAssignmentCalls = make(map[string]int)
	}
	s.partAssignmentCalls[partID]++
	return s.partAssignments[partID], nil
}
func (s *stubClient) CancelPrintJob(_ context.Context, _ string) error     { return nil }
func (s *stubClient) PrioritizePrintJob(_ context.Context, _ string) error { return nil }
func (s *stubClient) GetStoreSettings(_ context.Context) (printago.StoreSettings, error) {
	s.storeSettingsCalls++
	return printago.StoreSettings{AutomaticQueueMatching: true}, nil
}
func (s *stubClient) UpdateStoreSettings(_ context.Context, update printago.StoreSettingsUpdate) error {
	s.storeSettingsUpdates = append(s.storeSettingsUpdates, update)
	return nil
}

type printJobTagUpdate struct {
	jobID string
	tags  map[string]string
}

func (s *stubClient) UpdatePrintJobTags(_ context.Context, jobID string, tags map[string]string) error {
	s.printJobTagUpdates = append(s.printJobTagUpdates, printJobTagUpdate{jobID: jobID, tags: tags})
	return nil
}

func (s *stubClient) GroupMatching(_ context.Context, printJobIDs []string) (printago.GroupMatchingResult, error) {
	s.groupMatchingCalls = append(s.groupMatchingCalls, printJobIDs)
	return s.groupMatching, nil
}

// ---- GetPrinters ----

func TestCachingClient_GetPrinters_cachesResult(t *testing.T) {
	want := []printago.Printer{{ID: "p1", Name: "Printer A", Tags: []string{"foo"}}}
	stub := &stubClient{printers: want}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetPrinters(context.Background())
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetPrinters(context.Background())
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.printerCalls != 1 {
		t.Errorf("underlying client called %d times, want 1", stub.printerCalls)
	}
	if !reflect.DeepEqual(got1, want) {
		t.Errorf("first result = %+v, want %+v", got1, want)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("second result = %+v, want %+v", got2, want)
	}
}

func TestCachingClient_GetPrinters_expiredCacheRefetches(t *testing.T) {
	stub := &stubClient{printers: []printago.Printer{{ID: "p1"}}}
	c := printago.NewCachingClientWithTTL(stub, time.Millisecond)

	if _, err := c.GetPrinters(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetPrinters(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if stub.printerCalls != 2 {
		t.Errorf("underlying client called %d times after expiry, want 2", stub.printerCalls)
	}
}

// ---- GetPrinterSlots ----

func TestCachingClient_GetPrinterSlots_cachesResult(t *testing.T) {
	matID := "mat1"
	want := []printago.PrinterSlot{{ID: "s1", PrinterID: "p1", MaterialID: &matID}}
	stub := &stubClient{printerSlots: want}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetPrinterSlots(context.Background())
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetPrinterSlots(context.Background())
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.printerSlotCalls != 1 {
		t.Errorf("underlying client called %d times, want 1", stub.printerSlotCalls)
	}
	if !reflect.DeepEqual(got1, want) {
		t.Errorf("first result = %+v, want %+v", got1, want)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("second result = %+v, want %+v", got2, want)
	}
}

func TestCachingClient_GetPrinterSlots_expiredCacheRefetches(t *testing.T) {
	stub := &stubClient{printerSlots: []printago.PrinterSlot{{ID: "s1"}}}
	c := printago.NewCachingClientWithTTL(stub, time.Millisecond)

	if _, err := c.GetPrinterSlots(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetPrinterSlots(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if stub.printerSlotCalls != 2 {
		t.Errorf("underlying client called %d times after expiry, want 2", stub.printerSlotCalls)
	}
}

// ---- GetMaterials ----

func TestCachingClient_GetMaterials_cachesResult(t *testing.T) {
	want := []printago.Material{{ID: "m1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"}}
	stub := &stubClient{materials: want}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetMaterials(context.Background())
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetMaterials(context.Background())
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.materialCalls != 1 {
		t.Errorf("underlying client called %d times, want 1", stub.materialCalls)
	}
	if !reflect.DeepEqual(got1, want) {
		t.Errorf("first result = %+v, want %+v", got1, want)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("second result = %+v, want %+v", got2, want)
	}
}

func TestCachingClient_GetMaterials_expiredCacheRefetches(t *testing.T) {
	stub := &stubClient{materials: []printago.Material{{ID: "m1"}}}
	c := printago.NewCachingClientWithTTL(stub, time.Millisecond)

	if _, err := c.GetMaterials(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetMaterials(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if stub.materialCalls != 2 {
		t.Errorf("underlying client called %d times after expiry, want 2", stub.materialCalls)
	}
}

// ---- GetMaterialVariants ----

func TestCachingClient_GetMaterialVariants_cachesResult(t *testing.T) {
	want := []printago.MaterialVariant{{ID: "v1", MaterialID: "m1", Name: "Magenta"}}
	stub := &stubClient{materialVariants: want}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetMaterialVariants(context.Background())
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetMaterialVariants(context.Background())
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.materialVariantCalls != 1 {
		t.Errorf("underlying client called %d times, want 1", stub.materialVariantCalls)
	}
	if !reflect.DeepEqual(got1, want) {
		t.Errorf("first result = %+v, want %+v", got1, want)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("second result = %+v, want %+v", got2, want)
	}
}

func TestCachingClient_GetMaterialVariants_expiredCacheRefetches(t *testing.T) {
	stub := &stubClient{materialVariants: []printago.MaterialVariant{{ID: "v1"}}}
	c := printago.NewCachingClientWithTTL(stub, time.Millisecond)

	if _, err := c.GetMaterialVariants(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetMaterialVariants(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if stub.materialVariantCalls != 2 {
		t.Errorf("underlying client called %d times after expiry, want 2", stub.materialVariantCalls)
	}
}

// ---- GetPartMaterialAssignments ----

func TestCachingClient_GetPartMaterialAssignments_cachesResult(t *testing.T) {
	want := []printago.PartMaterialAssignment{{ID: "a1", PartID: "part1", MaterialID: "m1", VariantID: "v1"}}
	stub := &stubClient{
		partAssignments: map[string][]printago.PartMaterialAssignment{"part1": want},
	}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetPartMaterialAssignments(context.Background(), "part1")
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetPartMaterialAssignments(context.Background(), "part1")
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.partAssignmentCalls["part1"] != 1 {
		t.Errorf("underlying client called %d times for part1, want 1", stub.partAssignmentCalls["part1"])
	}
	if !reflect.DeepEqual(got1, want) {
		t.Errorf("first result = %+v, want %+v", got1, want)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("second result = %+v, want %+v", got2, want)
	}
}

func TestCachingClient_GetPartMaterialAssignments_separateCachePerPartID(t *testing.T) {
	stub := &stubClient{
		partAssignments: map[string][]printago.PartMaterialAssignment{
			"part1": {{ID: "a1", PartID: "part1"}},
			"part2": {{ID: "a2", PartID: "part2"}},
		},
	}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	if _, err := c.GetPartMaterialAssignments(context.Background(), "part1"); err != nil {
		t.Fatalf("part1 first call: %v", err)
	}
	if _, err := c.GetPartMaterialAssignments(context.Background(), "part2"); err != nil {
		t.Fatalf("part2 first call: %v", err)
	}
	// Second calls should be served from cache.
	if _, err := c.GetPartMaterialAssignments(context.Background(), "part1"); err != nil {
		t.Fatalf("part1 second call: %v", err)
	}
	if _, err := c.GetPartMaterialAssignments(context.Background(), "part2"); err != nil {
		t.Fatalf("part2 second call: %v", err)
	}

	if stub.partAssignmentCalls["part1"] != 1 {
		t.Errorf("part1: underlying client called %d times, want 1", stub.partAssignmentCalls["part1"])
	}
	if stub.partAssignmentCalls["part2"] != 1 {
		t.Errorf("part2: underlying client called %d times, want 1", stub.partAssignmentCalls["part2"])
	}
}

func TestCachingClient_GetPartMaterialAssignments_expiredCacheRefetches(t *testing.T) {
	stub := &stubClient{
		partAssignments: map[string][]printago.PartMaterialAssignment{
			"part1": {{ID: "a1"}},
		},
	}
	c := printago.NewCachingClientWithTTL(stub, time.Millisecond)

	if _, err := c.GetPartMaterialAssignments(context.Background(), "part1"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := c.GetPartMaterialAssignments(context.Background(), "part1"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if stub.partAssignmentCalls["part1"] != 2 {
		t.Errorf("underlying client called %d times after expiry, want 2", stub.partAssignmentCalls["part1"])
	}
}

func TestStoreSettings_CachingClientForwardsEveryGet(t *testing.T) {
	stub := &stubClient{}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GetStoreSettings(context.Background())
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GetStoreSettings(context.Background())
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if stub.storeSettingsCalls != 2 {
		t.Errorf("underlying client called %d times, want 2", stub.storeSettingsCalls)
	}
	want := printago.StoreSettings{AutomaticQueueMatching: true}
	if !reflect.DeepEqual(got1, want) || !reflect.DeepEqual(got2, want) {
		t.Errorf("got %+v and %+v, want %+v", got1, got2, want)
	}
}

func TestStoreSettings_CachingClientForwardsEveryUpdate(t *testing.T) {
	stub := &stubClient{}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)
	off := false
	update := printago.StoreSettingsUpdate{AutomaticQueueMatching: &off}

	if err := c.UpdateStoreSettings(context.Background(), update); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := c.UpdateStoreSettings(context.Background(), update); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if len(stub.storeSettingsUpdates) != 2 {
		t.Fatalf("underlying client called %d times, want 2", len(stub.storeSettingsUpdates))
	}
	if !reflect.DeepEqual(stub.storeSettingsUpdates[0], update) || !reflect.DeepEqual(stub.storeSettingsUpdates[1], update) {
		t.Errorf("updates = %+v, want both %+v", stub.storeSettingsUpdates, update)
	}
}

func TestGroupMatching_CachingClientForwardsEveryCall(t *testing.T) {
	want := printago.GroupMatchingResult{
		Truncated: true,
		Groups: []printago.MatchGroup{{
			MemberPrintJobIDs:        []string{"job-a"},
			RepresentativePrintJobID: "job-a",
			Printers: []printago.MatchPrinterVerdict{{
				PrinterID:        "printer-1",
				Matched:          true,
				FirstFailedCheck: "ready",
			}},
		}},
	}
	stub := &stubClient{groupMatching: want}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)

	got1, err := c.GroupMatching(context.Background(), []string{"job-a", "job-b"})
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	got2, err := c.GroupMatching(context.Background(), []string{"job-c"})
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if len(stub.groupMatchingCalls) != 2 {
		t.Fatalf("underlying client called %d times, want 2", len(stub.groupMatchingCalls))
	}
	if !reflect.DeepEqual(stub.groupMatchingCalls[0], []string{"job-a", "job-b"}) || !reflect.DeepEqual(stub.groupMatchingCalls[1], []string{"job-c"}) {
		t.Errorf("printJobIds = %#v, want [job-a job-b] then [job-c]", stub.groupMatchingCalls)
	}
	if !reflect.DeepEqual(got1, want) || !reflect.DeepEqual(got2, want) {
		t.Errorf("got %+v and %+v, want %+v", got1, got2, want)
	}
}

func TestUpdatePrintJobTags_CachingClientForwardsEveryCall(t *testing.T) {
	stub := &stubClient{}
	c := printago.NewCachingClientWithTTL(stub, time.Minute)
	tags := map[string]string{
		"printer.id":    "printer-a",
		"filament.type": "PLA",
	}

	if err := c.UpdatePrintJobTags(context.Background(), "job-1", tags); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := c.UpdatePrintJobTags(context.Background(), "job-2", tags); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}

	if len(stub.printJobTagUpdates) != 2 {
		t.Fatalf("underlying client called %d times, want 2", len(stub.printJobTagUpdates))
	}
	if stub.printJobTagUpdates[0].jobID != "job-1" || stub.printJobTagUpdates[1].jobID != "job-2" {
		t.Errorf("job ids = %q and %q, want job-1 and job-2", stub.printJobTagUpdates[0].jobID, stub.printJobTagUpdates[1].jobID)
	}
	if !reflect.DeepEqual(stub.printJobTagUpdates[0].tags, tags) || !reflect.DeepEqual(stub.printJobTagUpdates[1].tags, tags) {
		t.Errorf("tags = %#v and %#v, want both %#v", stub.printJobTagUpdates[0].tags, stub.printJobTagUpdates[1].tags, tags)
	}
}
