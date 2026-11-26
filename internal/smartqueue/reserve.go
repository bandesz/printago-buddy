package smartqueue

import "maps"

// TagPatch writes one job's requiredPrinterTags.
type TagPatch func(jobID string, tags map[string]string) error

// ReserveJob is one job in diff order and its current requiredPrinterTags.
type ReserveJob struct {
	ID           string
	RequiredTags map[string]string
}

// Reserve applies the pin and park diff for one plan.
// Assigned rows are pinned. Waiting and incompatible rows are parked.
// Merged and unreadable rows are skipped.
// It does not run the queue. The first tag or patch error stops the walk.
func Reserve(plan Plan, jobs []ReserveJob, patch TagPatch) error {
	for _, job := range jobs {
		row, ok := plan.Rows[job.ID]
		if !ok {
			continue
		}
		next, err := reservationTags(row, job.RequiredTags)
		if err != nil {
			return err
		}
		if maps.Equal(job.RequiredTags, next) {
			continue
		}
		if err := patch(job.ID, next); err != nil {
			return err
		}
	}
	return nil
}

// reservationTags is the pin or park result for a managed row.
// Merged and unreadable rows keep their current tags.
func reservationTags(row Row, tags map[string]string) (map[string]string, error) {
	switch row.Status {
	case StatusAssigned:
		return Pin(tags, row.PrinterID)
	case StatusWaiting, StatusIncompatible:
		return Park(tags)
	case StatusMerged, StatusUnreadable:
		return tags, nil
	default:
		return tags, nil
	}
}
