package web

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
)

type queueEntry struct {
	Index    int
	Job      printago.PrintJob
	Printers []PrinterCandidate
}

type queuePageData struct {
	Page    string
	Entries []queueEntry
	Err     string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/queue", http.StatusFound)
}

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	data := queuePageData{Page: "queue"}

	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		slog.Error("web: failed to fetch print jobs", "error", err)
		data.Err = "Failed to load print jobs: " + err.Error()
		s.render(w, s.queueTmpl, data)
		return
	}

	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		slog.Error("web: failed to fetch printers", "error", err)
		data.Err = "Failed to load printers: " + err.Error()
		s.render(w, s.queueTmpl, data)
		return
	}

	slots, err := s.client.GetPrinterSlots(ctx)
	if err != nil {
		slog.Error("web: failed to fetch printer slots", "error", err)
		data.Err = "Failed to load printer slots: " + err.Error()
		s.render(w, s.queueTmpl, data)
		return
	}
	slotsByPrinter := groupSlotsByPrinter(slots)

	// Fetch part material assignments for all jobs concurrently.
	assignmentsMap := fetchAllPartAssignments(ctx, s.client, jobs)

	entries := make([]queueEntry, 0, len(jobs))
	for i, job := range jobs {
		entries = append(entries, queueEntry{
			Index:    i + 1,
			Job:      job,
			Printers: RankPrinters(assignmentsMap[job.ID], printers, slotsByPrinter),
		})
	}
	data.Entries = entries
	s.render(w, s.queueTmpl, data)
}

// groupSlotsByPrinter indexes printer slots by printer ID.
func groupSlotsByPrinter(slots []printago.PrinterSlot) map[string][]printago.PrinterSlot {
	m := make(map[string][]printago.PrinterSlot, len(slots))
	for _, s := range slots {
		m[s.PrinterID] = append(m[s.PrinterID], s)
	}
	return m
}

// fetchAllPartAssignments fetches part material assignments for each job concurrently.
func fetchAllPartAssignments(
	ctx context.Context,
	client printago.ClientInterface,
	jobs []printago.PrintJob,
) map[string][]printago.PartMaterialAssignment {
	type result struct {
		jobID       string
		assignments []printago.PartMaterialAssignment
	}
	ch := make(chan result, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		if job.PartID == nil {
			continue
		}
		wg.Add(1)
		go func(j printago.PrintJob) {
			defer wg.Done()
			a, err := client.GetPartMaterialAssignments(ctx, *j.PartID)
			if err != nil {
				slog.Warn("web: failed to fetch part material assignments", "job", j.ID, "error", err)
			}
			ch <- result{j.ID, a}
		}(job)
	}
	wg.Wait()
	close(ch)

	m := make(map[string][]printago.PartMaterialAssignment, len(jobs))
	for r := range ch {
		m[r.jobID] = r.assignments
	}
	return m
}
