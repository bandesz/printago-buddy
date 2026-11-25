// Package web serves the Printago Buddy web UI.
package web

import (
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/bandesz/printago-buddy/internal/printago"
)

// MatchLevel indicates how well a printer matches a job's filament requirements.
type MatchLevel int

const (
	MatchNone    MatchLevel = 0
	MatchPartial MatchLevel = 1
	MatchFull    MatchLevel = 2
)

// MatchedFilament holds display info for a required filament that is present on
// a printer.
type MatchedFilament struct {
	Brand    string
	Type     string
	Name     string       // variant name (color name), e.g. "Magenta"
	ColorCSS template.CSS // CSS background value derived from the variant's hex color
	Tooltip  string       // pre-computed tooltip, e.g. "Bambu PLA Basic — Magenta"
}

// PrinterCandidate holds a printer and its computed match quality for a job.
type PrinterCandidate struct {
	Printer          printago.Printer
	Level            MatchLevel
	Matched          int // number of required slots the printer satisfies
	Total            int // total required slots
	MatchedFilaments []MatchedFilament
}

// IsFull reports whether this candidate is a full match.
func (c PrinterCandidate) IsFull() bool { return c.Level == MatchFull }

// matSpec identifies a material by variantId and/or materialId.
type matSpec struct{ variantID, materialID string }

// reqSlot pairs the match alternatives for a required slot with the source
// assignment, so that filament display info can be looked up on a match.
type reqSlot struct {
	alts       []matSpec
	assignment printago.PartMaterialAssignment
}

// RankPrinters returns up to 3 best-matching printers for a job, ordered best
// first.
//
// Required filaments come from partAssignments (from /v1/part-material-assignments).
// Each assignment describes one required filament slot matched by variantId or
// materialId against the printer's loaded slots.
//
// variantsByID and materialsByID are optional lookup maps used to populate
// MatchedFilaments with display info (color, brand, type). Pass nil if not
// needed.
//
// If there are no requirements, all printers are full matches.
func RankPrinters(
	partAssignments []printago.PartMaterialAssignment,
	printers []printago.Printer,
	slotsByPrinter map[string][]printago.PrinterSlot,
	variantsByID map[string]printago.MaterialVariant,
	materialsByID map[string]printago.Material,
) []PrinterCandidate {
	return RankJobPrinters(printago.PrintJob{}, partAssignments, printers, slotsByPrinter, variantsByID, materialsByID)
}

// RankJobPrinters ranks printers using the job's resolved filament requirements.
// Assignments resolved from a SKU variant replace the part default on the same
// channel. Other channels keep the part default.
func RankJobPrinters(
	job printago.PrintJob,
	partAssignments []printago.PartMaterialAssignment,
	printers []printago.Printer,
	slotsByPrinter map[string][]printago.PrinterSlot,
	variantsByID map[string]printago.MaterialVariant,
	materialsByID map[string]printago.Material,
) []PrinterCandidate {
	var required []reqSlot
	for _, slot := range requirementSlots(job, partAssignments) {
		if len(slot.alts) == 0 {
			continue
		}
		required = append(required, slot)
	}

	totalRequired := len(required)

	// No requirements at all — all printers are full matches, pick first 3 by name.
	if totalRequired == 0 {
		sorted := make([]printago.Printer, len(printers))
		copy(sorted, printers)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
		out := make([]PrinterCandidate, 0, min(3, len(sorted)))
		for _, p := range sorted {
			if len(out) == 3 {
				break
			}
			out = append(out, PrinterCandidate{Printer: p, Level: MatchFull})
		}
		return out
	}

	var candidates []PrinterCandidate
	for _, p := range printers {
		matched := 0
		var matchedFilaments []MatchedFilament
		for _, req := range required {
			if printerSatisfiesSlot(slotsByPrinter[p.ID], req.alts) {
				matched++
				matchedFilaments = append(matchedFilaments,
					buildMatchedFilament(req.assignment, variantsByID, materialsByID))
			}
		}

		totalMatched := matched
		if totalMatched == 0 {
			continue
		}

		level := MatchPartial
		if totalMatched == totalRequired {
			level = MatchFull
		}
		candidates = append(candidates, PrinterCandidate{
			Printer:          p,
			Level:            level,
			Matched:          totalMatched,
			Total:            totalRequired,
			MatchedFilaments: matchedFilaments,
		})
	}

	// Sort: full matches first, then by matched count descending, then by name.
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Level != b.Level {
			return a.Level > b.Level
		}
		if a.Matched != b.Matched {
			return a.Matched > b.Matched
		}
		return a.Printer.Name < b.Printer.Name
	})

	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

// JobFilaments returns the filament swatches to show for a queued job.
// Assignments resolved from a SKU variant replace the part default on the
// same channel. Other channels keep the part default.
func JobFilaments(
	job printago.PrintJob,
	partAssignments []printago.PartMaterialAssignment,
	variantsByID map[string]printago.MaterialVariant,
	materialsByID map[string]printago.Material,
) []MatchedFilament {
	slots := requirementSlots(job, partAssignments)
	out := make([]MatchedFilament, 0, len(slots))
	for _, slot := range slots {
		a := slot.assignment
		if a.VariantID == "" && a.MaterialID == "" && a.MaterialType == "" {
			continue
		}
		out = append(out, buildMatchedFilament(a, variantsByID, materialsByID))
	}
	return out
}

// effectiveMaterialAssignments returns the colors selected for this job.
// The print job's own assignments win. Otherwise the part build carries the
// assignments resolved from a SKU variant.
func effectiveMaterialAssignments(job printago.PrintJob) printago.MaterialAssignments {
	if len(job.MaterialAssignments) > 0 {
		return job.MaterialAssignments
	}
	if job.PartBuild != nil && len(job.PartBuild.MaterialAssignments) > 0 {
		return job.PartBuild.MaterialAssignments
	}
	return nil
}

// requirementSlots builds one required slot per filament channel. A channel
// present in the job's material assignments replaces the part assignment with
// the same index.
func requirementSlots(job printago.PrintJob, partAssignments []printago.PartMaterialAssignment) []reqSlot {
	overrides := effectiveMaterialAssignments(job)
	overridden := make(map[int]struct{}, len(overrides))
	slots := make([]reqSlot, 0, len(partAssignments)+len(overrides))
	for _, key := range sortedChannelKeys(overrides) {
		idx, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		slot, ok := slotFromSpecs(overrides[key])
		if !ok {
			continue
		}
		overridden[idx] = struct{}{}
		slots = append(slots, slot)
	}
	for _, a := range partAssignments {
		if _, skip := overridden[a.Index]; skip {
			continue
		}
		if a.VariantID == "" && a.MaterialID == "" {
			continue
		}
		slots = append(slots, reqSlot{
			alts:       []matSpec{{variantID: a.VariantID, materialID: a.MaterialID}},
			assignment: a,
		})
	}
	return slots
}

func slotFromSpecs(specs []printago.MaterialSpec) (reqSlot, bool) {
	var alts []matSpec
	var preferred *printago.MaterialSpec
	for i := range specs {
		spec := specs[i]
		if spec.VariantID == "" && spec.MaterialID == "" && spec.MaterialType == "" {
			continue
		}
		if preferred == nil {
			copied := spec
			preferred = &copied
		}
		if spec.VariantID != "" || spec.MaterialID != "" {
			alts = append(alts, matSpec{variantID: spec.VariantID, materialID: spec.MaterialID})
		}
	}
	if preferred == nil {
		return reqSlot{}, false
	}
	return reqSlot{
		alts: alts,
		assignment: printago.PartMaterialAssignment{
			VariantID:    preferred.VariantID,
			MaterialID:   preferred.MaterialID,
			MaterialType: preferred.MaterialType,
		},
	}, true
}

func sortedChannelKeys(assignments printago.MaterialAssignments) []string {
	keys := make([]string, 0, len(assignments))
	for key := range assignments {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, aErr := strconv.Atoi(keys[i])
		bi, bErr := strconv.Atoi(keys[j])
		if aErr == nil && bErr == nil {
			return ai < bi
		}
		return keys[i] < keys[j]
	})
	return keys
}

// buildMatchedFilament constructs a MatchedFilament from a part material
// assignment, looking up display info in the provided lookup maps.
func buildMatchedFilament(
	a printago.PartMaterialAssignment,
	variantsByID map[string]printago.MaterialVariant,
	materialsByID map[string]printago.Material,
) MatchedFilament {
	var brand, matType, name string
	var colorCSS template.CSS

	if a.VariantID != "" {
		if v, ok := variantsByID[a.VariantID]; ok {
			name = v.Name
			if v.Color != nil && *v.Color != "" {
				colorCSS = variantColorToCSS(*v.Color)
			}
			if m, ok := materialsByID[v.MaterialID]; ok {
				brand = m.Brand
				matType = m.Name
			}
		}
	} else if a.MaterialID != "" {
		if m, ok := materialsByID[a.MaterialID]; ok {
			brand = m.Brand
			matType = m.Name
		}
	}

	// Fall back to the assignment's MaterialType if still unknown.
	if matType == "" {
		matType = a.MaterialType
	}

	// Default swatch color when the variant has no hex color.
	if colorCSS == "" {
		colorCSS = "#94a3b8" // slate-400 neutral
	}

	// Build tooltip: "Brand Type — Name" e.g. "Bambu PLA Basic — Magenta".
	var parts []string
	if brand != "" {
		parts = append(parts, brand)
	}
	if matType != "" {
		parts = append(parts, matType)
	}
	tooltip := strings.Join(parts, " ")
	if name != "" {
		if tooltip != "" {
			tooltip += " — "
		}
		tooltip += name
	}

	return MatchedFilament{
		Brand:    brand,
		Type:     matType,
		Name:     name,
		ColorCSS: colorCSS,
		Tooltip:  tooltip,
	}
}

// variantColorToCSS converts a Printago variant color string ("#RRGGBBAA" or
// multiple colors separated by ";") to a CSS background value. For
// multi-color filaments the first segment is used; CSS supports 8-digit hex
// (#RRGGBBAA) natively in modern browsers.
func variantColorToCSS(color string) template.CSS {
	if idx := strings.Index(color, ";"); idx >= 0 {
		color = color[:idx]
	}
	return template.CSS(color) //nolint:gosec // trusted data from Printago API
}

// printerSatisfiesSlot reports whether any of the printer's slots matches any
// of the acceptable material alternatives for a required job slot.
//
// If an alternative specifies a variantId the slot must carry that exact
// variant (brand + type + color). Only when no variantId is provided does
// materialId serve as a coarser fallback (brand + type, any color).
func printerSatisfiesSlot(printerSlots []printago.PrinterSlot, alts []matSpec) bool {
	for _, alt := range alts {
		for _, ps := range printerSlots {
			if alt.variantID != "" {
				if ps.VariantID != nil && *ps.VariantID == alt.variantID {
					return true
				}
			} else if alt.materialID != "" {
				if ps.MaterialID != nil && *ps.MaterialID == alt.materialID {
					return true
				}
			}
		}
	}
	return false
}
