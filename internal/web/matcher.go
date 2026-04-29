// Package web serves the Printago Buddy web UI.
package web

import (
	"html/template"
	"sort"
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
	// Build required slot specs from part material assignments.
	// Each assignment is one required slot; variantId and materialId are both
	// acceptable identifiers for the same slot.
	var required []reqSlot
	for _, a := range partAssignments {
		if a.VariantID != "" || a.MaterialID != "" {
			required = append(required, reqSlot{
				alts:       []matSpec{{a.VariantID, a.MaterialID}},
				assignment: a,
			})
		}
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
