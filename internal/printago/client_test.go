package printago_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/bandesz/printago-buddy/internal/printago"
)

// newTestClient creates a Client pointed at the given httptest server.
// It replaces the private baseURL by wrapping the server so we can reach it
// without exporting internals: we instead expose a constructor that accepts
// a base URL for testing purposes via a thin helper in the same package
// (see client_testonly_test.go).  Because we are in the external test package
// we use the exported NewClientWithBaseURL helper defined below.
func newTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *printago.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := printago.NewClientWithBaseURL("test-key", "test-store", srv.URL)
	return srv, client
}

// authHandler wraps a handler and asserts the required auth headers are present.
func authHandler(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("authorization"); got != "ApiKey test-key" {
			t.Errorf("authorization header = %q, want %q", got, "ApiKey test-key")
		}
		if got := r.Header.Get("x-printago-storeid"); got != "test-store" {
			t.Errorf("x-printago-storeid header = %q, want %q", got, "test-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

// ---- GetPrinters ----

func TestGetPrinters_success(t *testing.T) {
	want := []printago.Printer{
		{ID: "abc123", Name: "Printer A", Tags: []string{"foo"}},
		{ID: "def456", Name: "Printer B", Tags: nil},
	}

	_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/printers" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, want)
	})))

	got, err := client.GetPrinters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetPrinters_apiError(t *testing.T) {
	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	_, err := client.GetPrinters(context.Background())
	if err == nil {
		t.Fatal("expected error on 401 response, got nil")
	}
}

// ---- GetPrinterSlots ----

func TestGetPrinterSlots_success(t *testing.T) {
	matID := "mat111"
	varID := "var222"
	instID := "inst333"
	want := []printago.PrinterSlot{
		{
			ID:         "slot1",
			PrinterID:  "abc123",
			MaterialID: &matID,
			VariantID:  &varID,
			InstanceID: &instID,
			AmsIndex:   0,
			SlotIndex:  1,
		},
	}

	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/printer-slots" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, want)
	}))

	got, err := client.GetPrinterSlots(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d slots, want 1", len(got))
	}
	if got[0].PrinterID != "abc123" {
		t.Errorf("PrinterID = %q, want %q", got[0].PrinterID, "abc123")
	}
	if got[0].VariantID == nil || *got[0].VariantID != varID {
		t.Errorf("VariantID = %v, want %q", got[0].VariantID, varID)
	}
}

// ---- GetMaterials ----

func TestGetMaterials_success(t *testing.T) {
	want := []printago.Material{
		{ID: "mat1", Name: "PLA Basic", Brand: "Bambu", Type: "PLA"},
	}

	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/materials" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, want)
	}))

	got, err := client.GetMaterials(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// ---- GetMaterialVariants ----

func TestGetMaterialVariants_success(t *testing.T) {
	want := []printago.MaterialVariant{
		{ID: "var1", MaterialID: "mat1", Name: "Magenta"},
	}

	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/materials/variants" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, want)
	}))

	got, err := client.GetMaterialVariants(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// ---- UpdatePrinterTags ----

func TestUpdatePrinterTags_sendsCorrectRequest(t *testing.T) {
	var gotBody map[string]any
	var gotPath string

	_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})))

	tags := []string{"filament_pla_basic_magenta", "custom_tag"}
	if err := client.UpdatePrinterTags(context.Background(), "printer123", tags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/v1/printers/printer123" {
		t.Errorf("path = %q, want /v1/printers/printer123", gotPath)
	}

	rawTags, ok := gotBody["tags"].([]any)
	if !ok {
		t.Fatalf("body[tags] missing or wrong type: %v", gotBody)
	}
	if len(rawTags) != 2 {
		t.Errorf("got %d tags in body, want 2", len(rawTags))
	}
}

func TestUpdatePrinterTags_apiError(t *testing.T) {
	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))

	err := client.UpdatePrinterTags(context.Background(), "p1", []string{"tag"})
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
}

// Archived jobs stay status=pending after they are cleared from the Printago
// queue; the queue view only includes hidden=false.
func TestGetPrintJobs_excludesArchivedJobs(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		writeJSON(t, w, []printago.PrintJob{})
	}))

	if _, err := client.GetPrintJobs(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/v1/print-jobs" {
		t.Errorf("path = %q, want /v1/print-jobs", gotPath)
	}
	if gotQuery.Get("status") != "pending" {
		t.Errorf("status = %q, want pending", gotQuery.Get("status"))
	}
	if gotQuery.Get("hidden") != "false" {
		t.Errorf("hidden = %q, want false", gotQuery.Get("hidden"))
	}
	if gotQuery.Get("include") != "partBuild" {
		t.Errorf("include = %q, want partBuild", gotQuery.Get("include"))
	}
}

// A full page is followed by the next offset. A shorter page ends the list,
// and the result is page one followed by page two.
func TestGetPrintJobsPages(t *testing.T) {
	const pageSize = 200

	pageOne := make([]printago.PrintJob, pageSize)
	for i := range pageOne {
		pageOne[i] = printago.PrintJob{ID: fmt.Sprintf("page1-%d", i)}
	}
	pageTwo := []printago.PrintJob{{ID: "page2-0"}}

	var queries []url.Values
	_, client := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/v1/print-jobs" {
			t.Errorf("path = %q, want /v1/print-jobs", r.URL.Path)
		}
		queries = append(queries, r.URL.Query())
		switch r.URL.Query().Get("offset") {
		case "", "0":
			writeJSON(t, w, pageOne)
		case "200":
			writeJSON(t, w, pageTwo)
		default:
			t.Errorf("unexpected offset %q", r.URL.Query().Get("offset"))
			writeJSON(t, w, []printago.PrintJob{})
		}
	}))

	got, err := client.GetPrintJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("got %d requests, want 2 (full page then a short page)", len(queries))
	}
	for i, q := range queries {
		if q.Get("status") != "pending" {
			t.Errorf("request %d status = %q, want pending", i, q.Get("status"))
		}
		if q.Get("hidden") != "false" {
			t.Errorf("request %d hidden = %q, want false", i, q.Get("hidden"))
		}
		if q.Get("sort") != "queueOrder" {
			t.Errorf("request %d sort = %q, want queueOrder", i, q.Get("sort"))
		}
		if q.Get("include") != "partBuild" {
			t.Errorf("request %d include = %q, want partBuild", i, q.Get("include"))
		}
		if q.Get("limit") != "200" {
			t.Errorf("request %d limit = %q, want 200", i, q.Get("limit"))
		}
	}
	if queries[1].Get("offset") != "200" {
		t.Errorf("second offset = %q, want 200", queries[1].Get("offset"))
	}

	want := append(append([]printago.PrintJob{}, pageOne...), pageTwo...)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %d jobs, want page one then page two (%d)", len(got), len(want))
	}
}

func TestPrintJob_decodesSKUMaterialAssignments(t *testing.T) {
	raw := []byte(`{
		"id": "aaaaaaaaaaaaaaaaaaaaaaaa",
		"materialAssignments": {"0": [{"variantId": "bbbbbbbbbbbbbbbbbbbbbbbb"}]},
		"partBuild": {
			"id": "cccccccccccccccccccccccc",
			"materialAssignments": {"1": [{"variantId": "dddddddddddddddddddddddd"}]}
		}
	}`)
	var job printago.PrintJob
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	if got := job.MaterialAssignments["0"][0].VariantID; got != "bbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("job variant = %q", got)
	}
	if job.PartBuild == nil {
		t.Fatal("partBuild was not decoded")
	}
	if got := job.PartBuild.MaterialAssignments["1"][0].VariantID; got != "dddddddddddddddddddddddd" {
		t.Errorf("part build variant = %q", got)
	}
}

func TestPrintJob_PreviewURI_prefersBuildThumbnail(t *testing.T) {
	raw := []byte(`{
		"thumbnailUri": "https://cdn.example/part.png",
		"buildThumbnailUri": "https://cdn.example/build.png",
		"coloredThumbnailUri": "https://cdn.example/colored.png"
	}`)
	var job printago.PrintJob
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	if got := job.PreviewURI(); got != "https://cdn.example/build.png" {
		t.Errorf("preview = %q, want buildThumbnailUri", got)
	}
}

func TestPrintJob_PreviewURI_fallsBackToPartThumbnail(t *testing.T) {
	part := "https://cdn.example/part.png"
	empty := ""
	job := printago.PrintJob{ThumbnailURI: &part, BuildThumbnailURI: &empty}
	if got := job.PreviewURI(); got != part {
		t.Errorf("preview = %q, want the part thumbnail", got)
	}
	if got := (printago.PrintJob{}).PreviewURI(); got != "" {
		t.Errorf("preview = %q, want empty", got)
	}
}

func boolPtr(v bool) *bool { return &v }

func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("decode body %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("decode want %s: %v", want, err)
	}
	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestStoreSettingsGet(t *testing.T) {
	cases := []struct {
		name string
		body string
		want printago.StoreSettings
	}{
		{
			name: "both fields",
			body: `{"automaticQueueMatching":true,"plateMerging":{"automaticMerging":true}}`,
			want: printago.StoreSettings{
				AutomaticQueueMatching: true,
				PlateMerging:           printago.PlateMerging{AutomaticMerging: true},
			},
		},
		{
			name: "plateMerging absent",
			body: `{"automaticQueueMatching":true}`,
			want: printago.StoreSettings{AutomaticQueueMatching: true},
		},
		{
			name: "automaticMerging absent",
			body: `{"automaticQueueMatching":true,"plateMerging":{}}`,
			want: printago.StoreSettings{AutomaticQueueMatching: true},
		},
		{
			name: "explicit false",
			body: `{"automaticQueueMatching":false,"plateMerging":{"automaticMerging":false}}`,
			want: printago.StoreSettings{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Method != http.MethodGet {
					t.Errorf("method = %q, want GET", r.Method)
				}
				if r.URL.Path != "/v1/settings/store" {
					t.Errorf("path = %q, want /v1/settings/store", r.URL.Path)
				}
				w.Header().Set("content-type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})))

			got, err := client.GetStoreSettings(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !called {
				t.Fatal("GET /v1/settings/store was not called")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestStoreSettingsUpdate(t *testing.T) {
	off := false
	on := true
	cases := []struct {
		name   string
		update printago.StoreSettingsUpdate
		want   string
	}{
		{
			name: "automatic merging only",
			update: printago.StoreSettingsUpdate{
				PlateMerging: &printago.PlateMergingUpdate{AutomaticMerging: boolPtr(off)},
			},
			want: `{"plateMerging":{"automaticMerging":false}}`,
		},
		{
			name: "automatic queue matching only",
			update: printago.StoreSettingsUpdate{
				AutomaticQueueMatching: boolPtr(off),
			},
			want: `{"automaticQueueMatching":false}`,
		},
		{
			name: "both",
			update: printago.StoreSettingsUpdate{
				AutomaticQueueMatching: boolPtr(off),
				PlateMerging:           &printago.PlateMergingUpdate{AutomaticMerging: boolPtr(on)},
			},
			want: `{"automaticQueueMatching":false,"plateMerging":{"automaticMerging":true}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Errorf("method = %q, want PATCH", r.Method)
				}
				if r.URL.Path != "/v1/settings/store" {
					t.Errorf("path = %q, want /v1/settings/store", r.URL.Path)
				}
				var err error
				body, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusOK)
			})))

			if err := client.UpdateStoreSettings(context.Background(), tc.update); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body == nil {
				t.Fatal("PATCH /v1/settings/store was not called")
			}
			assertJSON(t, body, tc.want)
		})
	}
}

func TestUpdatePrintJobTags(t *testing.T) {
	cases := []struct {
		name string
		tags map[string]string
		want string
	}{
		{
			name: "sends only requiredPrinterTags",
			tags: map[string]string{
				"printer.id":             "printer-a",
				"user.tags":              "&=pla",
				"printer.nozzleDiameter": "0.4",
				"printer.modelName":      "X1C",
				"filament.type":          "PLA",
			},
			want: `{
				"requiredPrinterTags": {
					"printer.id": "printer-a",
					"user.tags": "&=pla",
					"printer.nozzleDiameter": "0.4",
					"printer.modelName": "X1C",
					"filament.type": "PLA"
				}
			}`,
		},
		{
			name: "omits absent tag keys",
			tags: map[string]string{"filament.type": "PETG"},
			want: `{"requiredPrinterTags":{"filament.type":"PETG"}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var body []byte
			_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				var err error
				body, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusOK)
			})))

			if err := client.UpdatePrintJobTags(context.Background(), "job123", tc.tags); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body == nil {
				t.Fatal("PATCH /v1/print-jobs/{id} was not called")
			}
			if gotMethod != http.MethodPatch {
				t.Errorf("method = %q, want PATCH", gotMethod)
			}
			if gotPath != "/v1/print-jobs/job123" {
				t.Errorf("path = %q, want /v1/print-jobs/job123", gotPath)
			}
			assertJSON(t, body, tc.want)
		})
	}
}

func TestUpdatePrintJobTags_decodesParentJobIDAndGroupID(t *testing.T) {
	parent := "parent-1"
	group := "group-1"
	cases := []struct {
		name       string
		raw        string
		wantParent *string
		wantGroup  *string
	}{
		{
			name:       "both set",
			raw:        `{"parentJobId":"parent-1","groupId":"group-1"}`,
			wantParent: &parent,
			wantGroup:  &group,
		},
		{
			name: "null",
			raw:  `{"parentJobId":null,"groupId":null}`,
		},
		{
			name: "absent",
			raw:  `{"id":"job-1"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var job printago.PrintJob
			if err := json.Unmarshal([]byte(tc.raw), &job); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(job.ParentJobID, tc.wantParent) {
				t.Errorf("parentJobId = %v, want %v", job.ParentJobID, tc.wantParent)
			}
			if !reflect.DeepEqual(job.GroupID, tc.wantGroup) {
				t.Errorf("groupId = %v, want %v", job.GroupID, tc.wantGroup)
			}
			encoded, err := json.Marshal(job)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if bytes.Contains(encoded, []byte(`"ParentJobID"`)) || bytes.Contains(encoded, []byte(`"GroupID"`)) {
				t.Errorf("encoded = %s, want parentJobId and groupId", encoded)
			}
			if tc.wantParent != nil && !bytes.Contains(encoded, []byte(`"parentJobId":"parent-1"`)) {
				t.Errorf("encoded = %s, want parentJobId", encoded)
			}
			if tc.wantGroup != nil && !bytes.Contains(encoded, []byte(`"groupId":"group-1"`)) {
				t.Errorf("encoded = %s, want groupId", encoded)
			}
		})
	}
}

func TestUpdatePrintJobTags_decodesEveryStringKey(t *testing.T) {
	raw := []byte(`{
		"requiredPrinterTags": {
			"printer.id": "printer-a",
			"user.tags": "&=pla",
			"printer.nozzleDiameter": "0.4",
			"printer.modelName": "X1C",
			"filament.type": "PLA",
			"filament.color": "magenta"
		}
	}`)
	var job printago.PrintJob
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertPrinterTags(t, job.RequiredPrinterTags, map[string]string{
		"printer.id":             "printer-a",
		"user.tags":              "&=pla",
		"printer.nozzleDiameter": "0.4",
		"printer.modelName":      "X1C",
		"filament.type":          "PLA",
		"filament.color":         "magenta",
	})
}

func TestUpdatePrintJobTags_absentTagKeysStayAbsent(t *testing.T) {
	raw := []byte(`{"requiredPrinterTags":{"printer.id":"printer-a","filament.type":"PLA"}}`)
	var job printago.PrintJob
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertPrinterTags(t, job.RequiredPrinterTags, map[string]string{
		"printer.id":    "printer-a",
		"filament.type": "PLA",
	})
}

func TestGroupMatching(t *testing.T) {
	cases := []struct {
		name     string
		ids      []string
		response string
		wantBody string
		want     printago.GroupMatchingResult
	}{
		{
			name: "sends direct ids and keeps verdict fields",
			ids:  []string{"job-a", "job-b"},
			response: `{
				"truncated": true,
				"groups": [{
					"memberPrintJobIds": ["job-a", "job-b"],
					"representativePrintJobId": "job-a",
					"printers": [{
						"printerId": "printer-1",
						"matched": true,
						"compatible": false,
						"firstFailedCheck": "tags-match"
					}]
				}]
			}`,
			wantBody: `{"use":"direct","printJobIds":["job-a","job-b"]}`,
			want: printago.GroupMatchingResult{
				Truncated: true,
				Groups: []printago.MatchGroup{{
					MemberPrintJobIDs:        []string{"job-a", "job-b"},
					RepresentativePrintJobID: "job-a",
					Printers: []printago.MatchPrinterVerdict{{
						PrinterID:        "printer-1",
						Matched:          true,
						Compatible:       false,
						FirstFailedCheck: "tags-match",
					}},
				}},
			},
		},
		{
			name: "null or absent firstFailedCheck is empty",
			ids:  []string{"job-c"},
			response: `{
				"truncated": false,
				"groups": [{
					"memberPrintJobIds": ["job-c"],
					"representativePrintJobID": "job-c",
					"printers": [
						{
							"printerId": "printer-2",
							"matched": false,
							"compatible": true,
							"firstFailedCheck": null
						},
						{
							"printerId": "printer-3",
							"matched": false,
							"compatible": false
						}
					]
				}]
			}`,
			wantBody: `{"use":"direct","printJobIds":["job-c"]}`,
			want: printago.GroupMatchingResult{
				Groups: []printago.MatchGroup{{
					MemberPrintJobIDs:        []string{"job-c"},
					RepresentativePrintJobID: "job-c",
					Printers: []printago.MatchPrinterVerdict{
						{PrinterID: "printer-2", Compatible: true},
						{PrinterID: "printer-3"},
					},
				}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var body []byte
			_, client := newTestServer(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				var err error
				body, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				w.Header().Set("content-type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			})))

			got, err := client.GroupMatching(context.Background(), tc.ids)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body == nil {
				t.Fatal("POST /v1/print-jobs/queue/group-matching was not called")
			}
			if gotMethod != http.MethodPost {
				t.Errorf("method = %q, want POST", gotMethod)
			}
			if gotPath != "/v1/print-jobs/queue/group-matching" {
				t.Errorf("path = %q, want /v1/print-jobs/queue/group-matching", gotPath)
			}
			assertJSON(t, body, tc.wantBody)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func assertPrinterTags(t *testing.T, tags any, want map[string]string) {
	t.Helper()
	encoded, err := json.Marshal(tags)
	if err != nil {
		t.Fatalf("encode tags: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode tags %s: %v", encoded, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %s, want %#v", encoded, want)
	}
}
