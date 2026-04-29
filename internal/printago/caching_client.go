package printago

import (
	"context"
	"sync"
	"time"
)

// defaultCacheTTL is the cache lifetime for slowly-changing API resources
// (materials and material variants).
const defaultCacheTTL = 5 * time.Minute

// defaultShortCacheTTL is the cache lifetime for data that changes more
// frequently: printers, printer slots, and part material assignments.
const defaultShortCacheTTL = time.Minute

// cacheEntry holds a cached value together with its expiry time.
type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func (e *cacheEntry[T]) valid() bool {
	return time.Now().Before(e.expiresAt)
}

// CachingClient wraps a ClientInterface and caches results for endpoints whose
// data changes infrequently: GetMaterials, GetMaterialVariants, and
// GetPartMaterialAssignments. All other methods are forwarded to the inner
// client unchanged.
type CachingClient struct {
	inner    ClientInterface
	ttl      time.Duration
	shortTTL time.Duration

	mu               sync.Mutex
	printers         *cacheEntry[[]Printer]
	printerSlots     *cacheEntry[[]PrinterSlot]
	materials        *cacheEntry[[]Material]
	materialVariants *cacheEntry[[]MaterialVariant]
	partAssignments  map[string]*cacheEntry[[]PartMaterialAssignment]
}

// NewCachingClient wraps inner with a 5-minute cache for material and variant
// queries, and a 1-minute cache for printer, printer-slot, and part-assignment
// queries.
func NewCachingClient(inner ClientInterface) *CachingClient {
	return &CachingClient{
		inner:           inner,
		ttl:             defaultCacheTTL,
		shortTTL:        defaultShortCacheTTL,
		partAssignments: make(map[string]*cacheEntry[[]PartMaterialAssignment]),
	}
}

// NewCachingClientWithTTL creates a CachingClient with a single TTL applied to
// all cached endpoints. Prefer NewCachingClient in production; this constructor
// exists for tests that need a uniform short TTL to exercise cache expiry.
func NewCachingClientWithTTL(inner ClientInterface, ttl time.Duration) *CachingClient {
	return &CachingClient{
		inner:           inner,
		ttl:             ttl,
		shortTTL:        ttl,
		partAssignments: make(map[string]*cacheEntry[[]PartMaterialAssignment]),
	}
}

// GetPrinters returns all printers, fetching from the API only when the cached
// result has expired.
func (c *CachingClient) GetPrinters(ctx context.Context) ([]Printer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.printers != nil && c.printers.valid() {
		return c.printers.value, nil
	}

	result, err := c.inner.GetPrinters(ctx)
	if err != nil {
		return nil, err
	}
	c.printers = &cacheEntry[[]Printer]{value: result, expiresAt: time.Now().Add(c.shortTTL)}
	return result, nil
}

// GetPrinterSlots returns all printer filament slots, fetching from the API
// only when the cached result has expired.
func (c *CachingClient) GetPrinterSlots(ctx context.Context) ([]PrinterSlot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.printerSlots != nil && c.printerSlots.valid() {
		return c.printerSlots.value, nil
	}

	result, err := c.inner.GetPrinterSlots(ctx)
	if err != nil {
		return nil, err
	}
	c.printerSlots = &cacheEntry[[]PrinterSlot]{value: result, expiresAt: time.Now().Add(c.shortTTL)}
	return result, nil
}

// UpdatePrinterTags delegates directly to the inner client (not cached).
func (c *CachingClient) UpdatePrinterTags(ctx context.Context, printerID string, tags []string) error {
	return c.inner.UpdatePrinterTags(ctx, printerID, tags)
}

// GetPrintJobs delegates directly to the inner client (not cached).
func (c *CachingClient) GetPrintJobs(ctx context.Context) ([]PrintJob, error) {
	return c.inner.GetPrintJobs(ctx)
}

// CancelPrintJob delegates directly to the inner client (not cached).
func (c *CachingClient) CancelPrintJob(ctx context.Context, jobID string) error {
	return c.inner.CancelPrintJob(ctx, jobID)
}

// PrioritizePrintJob delegates directly to the inner client (not cached).
func (c *CachingClient) PrioritizePrintJob(ctx context.Context, jobID string) error {
	return c.inner.PrioritizePrintJob(ctx, jobID)
}

// GetMaterials returns all materials, fetching from the API only when the
// cached result has expired.
func (c *CachingClient) GetMaterials(ctx context.Context) ([]Material, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.materials != nil && c.materials.valid() {
		return c.materials.value, nil
	}

	result, err := c.inner.GetMaterials(ctx)
	if err != nil {
		return nil, err
	}
	c.materials = &cacheEntry[[]Material]{value: result, expiresAt: time.Now().Add(c.ttl)}
	return result, nil
}

// GetMaterialVariants returns all material variants, fetching from the API
// only when the cached result has expired.
func (c *CachingClient) GetMaterialVariants(ctx context.Context) ([]MaterialVariant, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.materialVariants != nil && c.materialVariants.valid() {
		return c.materialVariants.value, nil
	}

	result, err := c.inner.GetMaterialVariants(ctx)
	if err != nil {
		return nil, err
	}
	c.materialVariants = &cacheEntry[[]MaterialVariant]{value: result, expiresAt: time.Now().Add(c.ttl)}
	return result, nil
}

// GetPartMaterialAssignments returns the material assignments for a part,
// fetching from the API only when the cached entry for that part has expired.
// Each part ID is cached independently.
func (c *CachingClient) GetPartMaterialAssignments(ctx context.Context, partID string) ([]PartMaterialAssignment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.partAssignments[partID]; ok && entry.valid() {
		return entry.value, nil
	}

	result, err := c.inner.GetPartMaterialAssignments(ctx, partID)
	if err != nil {
		return nil, err
	}
	c.partAssignments[partID] = &cacheEntry[[]PartMaterialAssignment]{value: result, expiresAt: time.Now().Add(c.shortTTL)}
	return result, nil
}
