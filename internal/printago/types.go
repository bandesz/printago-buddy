package printago

import "encoding/json"

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
	ID                  string              `json:"id"`
	ParentJobID         *string             `json:"parentJobId"`
	GroupID             *string             `json:"groupId"`
	PartID              *string             `json:"partId"`
	PartName            string              `json:"partName"`
	Label               string              `json:"label"`
	Status              string              `json:"status"`
	QueueOrder          int                 `json:"queueOrder"`
	ThumbnailURI        *string             `json:"thumbnailUri"`
	BuildThumbnailURI   *string             `json:"buildThumbnailUri"`
	RequiredPrinterTags map[string]string   `json:"requiredPrinterTags"`
	Priority            int                 `json:"priority"`
	CreatedAt           string              `json:"createdAt"`
	MaterialAssignments MaterialAssignments `json:"materialAssignments"`
	PartBuild           *PartBuild          `json:"partBuild"`
}

// PreviewURI returns the image to show for this job. buildThumbnailUri is the
// preview Printago renders after the SKU variant colors are applied.
// thumbnailUri stays the part's original image, and coloredThumbnailUri is
// empty until a printer is assigned.
func (j PrintJob) PreviewURI() string {
	if uri := uriString(j.BuildThumbnailURI); uri != "" {
		return uri
	}
	return uriString(j.ThumbnailURI)
}

func uriString(uri *string) string {
	if uri == nil {
		return ""
	}
	return *uri
}

// PartBuild is the build a print job was created from. SKU variant selections
// are resolved onto its material assignments.
type PartBuild struct {
	ID                  string              `json:"id"`
	MaterialAssignments MaterialAssignments `json:"materialAssignments"`
}

// MaterialSpec is one acceptable filament for a channel, in preference order.
// A SKU variant selection resolves to a spec with VariantID set.
type MaterialSpec struct {
	GroupID      string `json:"groupId,omitempty"`
	VariantID    string `json:"variantId,omitempty"`
	MaterialID   string `json:"materialId,omitempty"`
	MaterialType string `json:"materialType,omitempty"`
}

// MaterialAssignments maps a filament channel index ("0", "1", …) to the
// acceptable specs for that channel, most preferred first.
type MaterialAssignments map[string][]MaterialSpec

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

// StoreSettings is the subset of GET /v1/settings/store that Buddy reads.
// A missing plateMerging object or automaticMerging field is false.
type StoreSettings struct {
	AutomaticQueueMatching bool         `json:"automaticQueueMatching"`
	PlateMerging           PlateMerging `json:"plateMerging"`
}

// PlateMerging is the plateMerging object on store settings.
type PlateMerging struct {
	AutomaticMerging bool `json:"automaticMerging"`
}

// StoreSettingsUpdate is a partial body for PATCH /v1/settings/store.
// A nil field is omitted. A pointer to false is sent.
type StoreSettingsUpdate struct {
	AutomaticQueueMatching *bool               `json:"automaticQueueMatching,omitempty"`
	PlateMerging           *PlateMergingUpdate `json:"plateMerging,omitempty"`
}

// PlateMergingUpdate is the plateMerging object on a settings patch.
type PlateMergingUpdate struct {
	AutomaticMerging *bool `json:"automaticMerging,omitempty"`
}

// GroupMatchingResult is the POST /v1/print-jobs/queue/group-matching response.
type GroupMatchingResult struct {
	Truncated bool         `json:"truncated"`
	Groups    []MatchGroup `json:"groups"`
}

// MatchGroup is one queue group and the verdict for each printer.
type MatchGroup struct {
	MemberPrintJobIDs        []string              `json:"memberPrintJobIds"`
	RepresentativePrintJobID string                `json:"representativePrintJobId"`
	Printers                 []MatchPrinterVerdict `json:"printers"`
}

// MatchPrinterVerdict is one printer's group-matching verdict.
// FirstFailedCheck is empty when that printer did not fail a check.
type MatchPrinterVerdict struct {
	PrinterID        string `json:"printerId"`
	Matched          bool   `json:"matched"`
	Compatible       bool   `json:"compatible"`
	FirstFailedCheck string `json:"firstFailedCheck"`
}

// MatchingTroubleshootResult is the GET /v1/print-jobs/{id}/matching-troubleshoot response.
type MatchingTroubleshootResult struct {
	Printers []TroubleshootPrinter `json:"printers"`
}

// TroubleshootPrinter is one printer in a matching-troubleshoot response.
type TroubleshootPrinter struct {
	PrinterID string              `json:"printerId"`
	Checks    []TroubleshootCheck `json:"checks"`
}

// TroubleshootCheck is one matching criterion for a printer.
type TroubleshootCheck struct {
	Code        string                   `json:"code"`
	Passed      bool                     `json:"passed"`
	Comparisons []TroubleshootComparison `json:"comparisons"`
}

// TroubleshootComparison is one tag comparison on a check.
type TroubleshootComparison struct {
	Key         string `json:"key"`
	Requirement string `json:"requirement"`
	Matched     bool   `json:"matched"`
}

// UnmarshalJSON decodes a check. A missing or null comparisons list becomes empty.
func (c *TroubleshootCheck) UnmarshalJSON(data []byte) error {
	var raw struct {
		Code        string                   `json:"code"`
		Passed      bool                     `json:"passed"`
		Comparisons []TroubleshootComparison `json:"comparisons"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Comparisons == nil {
		raw.Comparisons = []TroubleshootComparison{}
	}
	*c = TroubleshootCheck{
		Code:        raw.Code,
		Passed:      raw.Passed,
		Comparisons: raw.Comparisons,
	}
	return nil
}
