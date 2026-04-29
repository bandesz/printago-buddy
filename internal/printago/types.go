package printago

// Printer represents a printer entity from the Printago API.
type Printer struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// PrinterSlot represents a filament slot on a printer (AMS slot or external spool).
type PrinterSlot struct {
	ID         string  `json:"id"`
	PrinterID  string  `json:"printerId"`
	MaterialID *string `json:"materialId"`
	VariantID  *string `json:"variantId"`
	InstanceID *string `json:"instanceId"`
	// AmsIndex is -1 for external spool, 0+ for AMS unit index.
	AmsIndex  int `json:"amsIndex"`
	SlotIndex int `json:"slotIndex"`
}

// Material represents a filament material (e.g. "PLA Basic" by Bambu).
type Material struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Brand string `json:"brand"`
	Type  string `json:"type"`
}

// MaterialVariant represents a specific variant of a material (e.g. color "Magenta").
type MaterialVariant struct {
	ID         string `json:"id"`
	MaterialID string `json:"materialId"`
	Name       string `json:"name"`
	// Color is a hex RGBA color string ("#RRGGBBAA"), optionally multiple colors
	// separated by semicolons for multi-color filaments, e.g. "#FF0000FF;#0000FFFF".
	Color *string `json:"color"`
}

// PrintJob represents a print job in the Printago queue.
type PrintJob struct {
	ID                  string       `json:"id"`
	PartID              *string      `json:"partId"`
	PartName            string       `json:"partName"`
	Label               string       `json:"label"`
	Status              string       `json:"status"`
	QueueOrder          int          `json:"queueOrder"`
	ThumbnailURI        *string      `json:"thumbnailUri"`
	RequiredPrinterTags PrintJobTags `json:"requiredPrinterTags"`
	Priority            int          `json:"priority"`
	CreatedAt           string       `json:"createdAt"`
}

// PartMaterialAssignment represents a required filament slot for a part.
type PartMaterialAssignment struct {
	ID           string `json:"id"`
	PartID       string `json:"partId"`
	MaterialID   string `json:"materialId"`
	VariantID    string `json:"variantId"`
	MaterialType string `json:"materialType"`
	Index        int    `json:"index"`
	Priority     int    `json:"priority"`
}

// PrintJobTags holds the tag filter expressions for a print job.
// UserTags uses Printago's filter syntax, e.g. "&=tag1,tag2".
type PrintJobTags struct {
	UserTags string `json:"user.tags"`
}

// PrintJobMatchingDetails holds per-printer matching information for a job as
// returned by /v1/print-jobs/{id}/matching-details.
type PrintJobMatchingDetails struct {
	Details map[string]PrinterMatchInfo `json:"details"`
}

// PrinterMatchInfo is one printer's entry in matching details.
type PrinterMatchInfo struct {
	Data       PrinterMatchData `json:"data"`
	Matched    bool             `json:"matched"`
	Reason     string           `json:"reason"`
	ReasonCode string           `json:"reasonCode"`
}

// PrinterMatchData holds the slot and assignment data for one printer in the
// matching details response.
type PrinterMatchData struct {
	// ComputedAssignments maps slot index ("0", "1", …) to acceptable material specs.
	ComputedAssignments map[string][]MaterialAssignment `json:"computedAssignments"`
}

// MaterialAssignment describes one acceptable material for a job slot.
type MaterialAssignment struct {
	VariantID    string `json:"variantId"`
	MaterialID   string `json:"materialId"`
	MaterialType string `json:"materialType"`
}
