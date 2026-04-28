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
