package jobs

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
)

const filamentTagPrefix = "filament_"

// nonAlphanumRe matches one or more characters that are not lowercase letters
// or digits, used to normalise filament names into tag-safe strings.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// FilamentTaggerJob queries all printers and their loaded filaments, then
// updates each printer's tags so that every AMS slot and external spool is
// represented by a tag of the form "filament_<normalised_name>".
type FilamentTaggerJob struct {
	client printago.ClientInterface
}

// NewFilamentTaggerJob creates a new FilamentTaggerJob backed by the given API client.
func NewFilamentTaggerJob(client printago.ClientInterface) *FilamentTaggerJob {
	return &FilamentTaggerJob{client: client}
}

// Run executes the filament tagging job. It is safe to call concurrently; each
// invocation uses its own context with a generous timeout.
func (j *FilamentTaggerJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	slog.Info("filament tagger: starting run")

	if err := j.run(ctx); err != nil {
		slog.Error("filament tagger: run failed", "error", err)
		return
	}

	slog.Info("filament tagger: run completed")
}

func (j *FilamentTaggerJob) run(ctx context.Context) error {
	printers, err := j.client.GetPrinters(ctx)
	if err != nil {
		return err
	}

	slots, err := j.client.GetPrinterSlots(ctx)
	if err != nil {
		return err
	}

	materials, err := j.client.GetMaterials(ctx)
	if err != nil {
		return err
	}

	variants, err := j.client.GetMaterialVariants(ctx)
	if err != nil {
		return err
	}

	// Build lookup maps.
	materialByID := make(map[string]printago.Material, len(materials))
	for _, m := range materials {
		materialByID[m.ID] = m
	}

	variantByID := make(map[string]printago.MaterialVariant, len(variants))
	for _, v := range variants {
		variantByID[v.ID] = v
	}

	// Group slots by printer ID.
	slotsByPrinter := make(map[string][]printago.PrinterSlot, len(printers))
	for _, s := range slots {
		slotsByPrinter[s.PrinterID] = append(slotsByPrinter[s.PrinterID], s)
	}

	for _, printer := range printers {
		newFilamentTags := buildFilamentTags(slotsByPrinter[printer.ID], materialByID, variantByID)
		updatedTags := mergeTags(printer.Tags, newFilamentTags)

		if tagsEqual(printer.Tags, updatedTags) {
			slog.Debug("filament tagger: no tag changes", "printer", printer.Name)
			continue
		}

		slog.Info("filament tagger: updating tags",
			"printer", printer.Name,
			"tags", updatedTags,
		)

		if err := j.client.UpdatePrinterTags(ctx, printer.ID, updatedTags); err != nil {
			slog.Error("filament tagger: failed to update printer tags",
				"printer", printer.Name,
				"error", err,
			)
			// Continue with remaining printers rather than aborting the whole run.
		}
	}

	return nil
}

// buildFilamentTags derives the set of filament tags for a printer's slots.
func buildFilamentTags(
	slots []printago.PrinterSlot,
	materialByID map[string]printago.Material,
	variantByID map[string]printago.MaterialVariant,
) []string {
	seen := make(map[string]struct{})
	var tags []string

	for _, slot := range slots {
		tag := slotToTag(slot, materialByID, variantByID)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}

	sort.Strings(tags)
	return tags
}

// slotToTag converts a single printer slot to a filament tag, or returns ""
// if the slot has no material information.
func slotToTag(
	slot printago.PrinterSlot,
	materialByID map[string]printago.Material,
	variantByID map[string]printago.MaterialVariant,
) string {
	// Prefer variant → its name already identifies the specific filament.
	if slot.VariantID != nil {
		if v, ok := variantByID[*slot.VariantID]; ok {
			// Combine with the parent material name for a more descriptive tag.
			if m, ok := materialByID[v.MaterialID]; ok {
				return normaliseTag(m.Name + " " + v.Name)
			}
			return normaliseTag(v.Name)
		}
	}

	if slot.MaterialID != nil {
		if m, ok := materialByID[*slot.MaterialID]; ok {
			return normaliseTag(m.Name)
		}
	}

	return ""
}

// normaliseTag converts an arbitrary string into a tag of the form
// "filament_<snake_case_name>", e.g. "PLA Basic - Magenta" →
// "filament_pla_basic_magenta".
func normaliseTag(name string) string {
	lower := strings.ToLower(name)
	snake := nonAlphanumRe.ReplaceAllString(lower, "_")
	snake = strings.Trim(snake, "_")
	if snake == "" {
		return ""
	}
	return filamentTagPrefix + snake
}

// mergeTags returns a new tag slice that keeps all non-filament tags from
// current and replaces any filament_ tags with the provided new set.
func mergeTags(current []string, newFilament []string) []string {
	result := make([]string, 0, len(current))
	for _, t := range current {
		if !strings.HasPrefix(t, filamentTagPrefix) {
			result = append(result, t)
		}
	}
	result = append(result, newFilament...)
	sort.Strings(result)
	return result
}

// tagsEqual reports whether two tag slices contain the same elements
// regardless of order.
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sorted := func(s []string) []string {
		c := slices.Clone(s)
		sort.Strings(c)
		return c
	}
	sa, sb := sorted(a), sorted(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
