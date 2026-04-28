package printago_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
