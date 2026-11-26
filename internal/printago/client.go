// Package printago provides an HTTP client for the Printago REST API.
package printago

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.printago.io"

// ClientInterface is the interface satisfied by Client. Accepting this
// interface instead of the concrete type makes callers straightforward to
// test with a simple test double.
type ClientInterface interface {
	GetPrinters(ctx context.Context) ([]Printer, error)
	GetPrinterSlots(ctx context.Context) ([]PrinterSlot, error)
	GetMaterials(ctx context.Context) ([]Material, error)
	GetMaterialVariants(ctx context.Context) ([]MaterialVariant, error)
	UpdatePrinterTags(ctx context.Context, printerID string, tags []string) error
	GetPrintJobs(ctx context.Context) ([]PrintJob, error)
	GetPartMaterialAssignments(ctx context.Context, partID string) ([]PartMaterialAssignment, error)
	CancelPrintJob(ctx context.Context, jobID string) error
	PrioritizePrintJob(ctx context.Context, jobID string) error
	UpdatePrintJobTags(ctx context.Context, jobID string, tags map[string]string) error
	GetStoreSettings(ctx context.Context) (StoreSettings, error)
	UpdateStoreSettings(ctx context.Context, update StoreSettingsUpdate) error
	GroupMatching(ctx context.Context, printJobIDs []string) (GroupMatchingResult, error)
	MatchingTroubleshoot(ctx context.Context, printJobID string) (MatchingTroubleshootResult, error)
	RunQueue(ctx context.Context) error
}

// Client is an HTTP client for the Printago REST API.
type Client struct {
	apiKey     string
	storeID    string
	base       string
	httpClient *http.Client
}

// NewClient creates a new Printago API client.
func NewClient(apiKey, storeID string) *Client {
	return NewClientWithBaseURL(apiKey, storeID, baseURL)
}

// NewClientWithBaseURL creates a Client that sends requests to the given base
// URL instead of the default production endpoint. Useful for tests.
func NewClientWithBaseURL(apiKey, storeID, url string) *Client {
	return &Client{
		apiKey:  apiKey,
		storeID: storeID,
		base:    url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do executes an HTTP request with the required auth headers and decodes the
// JSON response into dst. Pass nil for dst to discard the response body.
func (c *Client) do(ctx context.Context, method, path string, body any, dst any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("authorization", "ApiKey "+c.apiKey)
	req.Header.Set("x-printago-storeid", c.storeID)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d for %s %s: %s", resp.StatusCode, method, path, string(b))
	}

	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return fmt.Errorf("decode response from %s %s: %w", method, path, err)
		}
	}

	return nil
}

// GetPrinters returns all printers in the store.
func (c *Client) GetPrinters(ctx context.Context) ([]Printer, error) {
	var printers []Printer
	if err := c.do(ctx, http.MethodGet, "/v1/printers", nil, &printers); err != nil {
		return nil, err
	}
	return printers, nil
}

// GetPrinterSlots returns all printer filament slots across all printers.
func (c *Client) GetPrinterSlots(ctx context.Context) ([]PrinterSlot, error) {
	var slots []PrinterSlot
	if err := c.do(ctx, http.MethodGet, "/v1/printer-slots", nil, &slots); err != nil {
		return nil, err
	}
	return slots, nil
}

// GetMaterials returns all materials defined in the store.
func (c *Client) GetMaterials(ctx context.Context) ([]Material, error) {
	var materials []Material
	if err := c.do(ctx, http.MethodGet, "/v1/materials", nil, &materials); err != nil {
		return nil, err
	}
	return materials, nil
}

// GetMaterialVariants returns all material variants defined in the store.
func (c *Client) GetMaterialVariants(ctx context.Context) ([]MaterialVariant, error) {
	var variants []MaterialVariant
	if err := c.do(ctx, http.MethodGet, "/v1/materials/variants", nil, &variants); err != nil {
		return nil, err
	}
	return variants, nil
}

// UpdatePrinterTags replaces the full tag list on a printer.
func (c *Client) UpdatePrinterTags(ctx context.Context, printerID string, tags []string) error {
	body := map[string]any{"tags": tags}
	return c.do(ctx, http.MethodPatch, "/v1/printers/"+printerID, body, nil)
}

// GetPartMaterialAssignments returns the material assignments for a part,
// describing what filament is required in each slot.
func (c *Client) GetPartMaterialAssignments(ctx context.Context, partID string) ([]PartMaterialAssignment, error) {
	var assignments []PartMaterialAssignment
	if err := c.do(ctx, http.MethodGet, "/v1/part-material-assignments?partId="+partID, nil, &assignments); err != nil {
		return nil, err
	}
	return assignments, nil
}

// GetPrintJobs returns every pending print job that is still visible in the
// Printago queue, sorted by queue order. Each request uses limit=200 and the
// next request follows with offset until a page is shorter than that. Cleared
// jobs stay status=pending and are marked hidden, so hidden jobs are excluded.
// partBuild is included because a SKU variant's resolved filament colors live
// on the build.
func (c *Client) GetPrintJobs(ctx context.Context) ([]PrintJob, error) {
	const pageSize = 200
	var jobs []PrintJob
	for offset := 0; ; offset += pageSize {
		path := "/v1/print-jobs?status=pending&hidden=false&limit=200&sort=queueOrder&include=partBuild"
		if offset > 0 {
			path += fmt.Sprintf("&offset=%d", offset)
		}
		var page []PrintJob
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		jobs = append(jobs, page...)
		if len(page) < pageSize {
			return jobs, nil
		}
	}
}

// CancelPrintJob cancels a single queued or in-progress print job.
func (c *Client) CancelPrintJob(ctx context.Context, jobID string) error {
	body := map[string]any{"printJobIds": []string{jobID}}
	return c.do(ctx, http.MethodPatch, "/v1/print-jobs/cancel", body, nil)
}

// PrioritizePrintJob moves a single print job to the front of the queue.
func (c *Client) PrioritizePrintJob(ctx context.Context, jobID string) error {
	body := map[string]any{"printJobIds": []string{jobID}}
	return c.do(ctx, http.MethodPatch, "/v1/print-jobs/move-to-queue-front", body, nil)
}

// UpdatePrintJobTags patches one print job, sending only requiredPrinterTags.
func (c *Client) UpdatePrintJobTags(ctx context.Context, jobID string, tags map[string]string) error {
	body := struct {
		RequiredPrinterTags map[string]string `json:"requiredPrinterTags"`
	}{RequiredPrinterTags: tags}
	return c.do(ctx, http.MethodPatch, "/v1/print-jobs/"+jobID, body, nil)
}

// GetStoreSettings returns store settings from GET /v1/settings/store.
func (c *Client) GetStoreSettings(ctx context.Context) (StoreSettings, error) {
	var settings StoreSettings
	if err := c.do(ctx, http.MethodGet, "/v1/settings/store", nil, &settings); err != nil {
		return StoreSettings{}, err
	}
	return settings, nil
}

// UpdateStoreSettings patches store settings with only the fields set on update.
func (c *Client) UpdateStoreSettings(ctx context.Context, update StoreSettingsUpdate) error {
	return c.do(ctx, http.MethodPatch, "/v1/settings/store", update, nil)
}

// GroupMatching posts direct queue group matching for the given print job ids.
func (c *Client) GroupMatching(ctx context.Context, printJobIDs []string) (GroupMatchingResult, error) {
	body := struct {
		Use         string   `json:"use"`
		PrintJobIDs []string `json:"printJobIds"`
	}{
		Use:         "direct",
		PrintJobIDs: printJobIDs,
	}
	var result GroupMatchingResult
	if err := c.do(ctx, http.MethodPost, "/v1/print-jobs/queue/group-matching", body, &result); err != nil {
		return GroupMatchingResult{}, err
	}
	return result, nil
}

// RunQueue posts a manual queue run.
func (c *Client) RunQueue(ctx context.Context) error {
	body := struct {
		Manual bool `json:"manual"`
	}{Manual: true}
	return c.do(ctx, http.MethodPost, "/v1/print-jobs/queue/run", body, nil)
}

// MatchingTroubleshoot returns matching checks for one print job.
func (c *Client) MatchingTroubleshoot(ctx context.Context, printJobID string) (MatchingTroubleshootResult, error) {
	var result MatchingTroubleshootResult
	path := "/v1/print-jobs/" + printJobID + "/matching-troubleshoot"
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return MatchingTroubleshootResult{}, err
	}
	return result, nil
}
