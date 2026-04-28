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
	ID   string `json:"id"`
	Name string `json:"name"`
	Brand string `json:"brand"`
	Type string `json:"type"`
}

// MaterialVariant represents a specific variant of a material (e.g. colour "Magenta").
type MaterialVariant struct {
	ID         string `json:"id"`
	MaterialID string `json:"materialId"`
	Name       string `json:"name"`
}
