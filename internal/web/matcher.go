// Package web serves the Printago Buddy web UI.
package web

import (
	"sort"

	"github.com/bandesz/printago-buddy/internal/printago"
)

// MatchLevel indicates how well a printer matches a job's filament requirements.
type MatchLevel int

const (
	MatchNone    MatchLevel = 0
	MatchPartial MatchLevel = 1
	MatchFull    MatchLevel = 2
)

// PrinterCandidate holds a printer and its computed match quality for a job.
type PrinterCandidate struct {
	Printer printago.Printer
	Level   MatchLevel
	Matched int // number of required slots/tags the printer satisfies
	Total   int // total required slots/tags
}

// IsFull reports whether this candidate is a full match.
func (c PrinterCandidate) IsFull() bool { return c.Level == MatchFull }

// matSpec identifies a material by variantId and/or materialId.
type matSpec struct{ variantID, materialID string }

// RankPrinters returns up to 3 best-matching printers for a job, ordered best
// first.
//
// Required filaments come from partAssignments (from /v1/part-material-assignments).
// Each assignment describes one required filament slot matched by variantId or
// materialId against the printer's loaded slots. Tags prefixed with
// "build_plate_" are ignored entirely.
//
// If there are no requirements, all printers are full matches.
func RankPrinters(
	partAssignments []printago.PartMaterialAssignment,
	printers []printago.Printer,
	slotsByPrinter map[string][]printago.PrinterSlot,
) []PrinterCandidate {
	// Build required slot specs from part material assignments.
	// Each assignment is one required slot; variantId and materialId are both
	// acceptable identifiers for the same slot.
	var requiredSlots [][]matSpec
	for _, a := range partAssignments {
		if a.VariantID != "" || a.MaterialID != "" {
			requiredSlots = append(requiredSlots, []matSpec{{a.VariantID, a.MaterialID}})
		}
	}

	totalRequired := len(requiredSlots)

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
		for _, alts := range requiredSlots {
			if printerSatisfiesSlot(slotsByPrinter[p.ID], alts) {
				matched++
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
			Printer: p,
			Level:   level,
			Matched: totalMatched,
			Total:   totalRequired,
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

// printerSatisfiesSlot reports whether any of the printer's slots matches any
// of the acceptable material alternatives for a required job slot.
//
// If an alternative specifies a variantId the slot must carry that exact
// variant (brand + type + colour). Only when no variantId is provided does
// materialId serve as a coarser fallback (brand + type, any colour).
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

