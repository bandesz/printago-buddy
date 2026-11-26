package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

type queueEntry struct {
	Job          printago.PrintJob
	JobMaterials []JobMaterial
	Printers     []PrinterCandidate
	Assignment   string
}

type queuePageData struct {
	Page          string
	NormalEntries []queueEntry
	LowEntries    []queueEntry
	Err           []string
	SmartQueueOn  bool
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
	flash := r.URL.Query().Get("error")
	plan := s.currentPlan()
	data.SmartQueueOn = plan != nil && plan.Enabled

	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		slog.Error("web: failed to fetch print jobs", "error", err)
		data.Err = []string{"Failed to load print jobs: " + err.Error()}
		s.render(w, s.queueTmpl, data)
		return
	}

	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		slog.Error("web: failed to fetch printers", "error", err)
		data.Err = []string{"Failed to load printers: " + err.Error()}
		s.render(w, s.queueTmpl, data)
		return
	}

	slots, err := s.client.GetPrinterSlots(ctx)
	if err != nil {
		slog.Error("web: failed to fetch printer slots", "error", err)
		data.Err = []string{"Failed to load printer slots: " + err.Error()}
		s.render(w, s.queueTmpl, data)
		return
	}
	slotsByPrinter := groupSlotsByPrinter(slots)

	materials, err := s.client.GetMaterials(ctx)
	if err != nil {
		slog.Error("web: failed to fetch materials", "error", err)
		data.Err = []string{"Failed to load materials: " + err.Error()}
		s.render(w, s.queueTmpl, data)
		return
	}
	materialsByID := make(map[string]printago.Material, len(materials))
	for _, m := range materials {
		materialsByID[m.ID] = m
	}

	variants, err := s.client.GetMaterialVariants(ctx)
	if err != nil {
		slog.Error("web: failed to fetch material variants", "error", err)
		data.Err = []string{"Failed to load material variants: " + err.Error()}
		s.render(w, s.queueTmpl, data)
		return
	}
	variantsByID := make(map[string]printago.MaterialVariant, len(variants))
	for _, v := range variants {
		variantsByID[v.ID] = v
	}

	// Fetch part material assignments for all jobs concurrently.
	assignmentsMap := fetchAllPartAssignments(ctx, s.client, jobs)
	plan = s.currentPlan()
	data.SmartQueueOn = plan != nil && plan.Enabled
	data.Err = queueBanner(flash, plan)

	normalEntries := make([]queueEntry, 0, len(jobs))
	lowEntries := make([]queueEntry, 0)
	for _, job := range jobs {
		jobAssignments := assignmentsMap[job.ID]
		entry := queueEntry{
			Job:          job,
			JobMaterials: JobFilaments(job, jobAssignments, variantsByID, materialsByID),
			Printers:     RankJobPrinters(job, jobAssignments, printers, slotsByPrinter, variantsByID, materialsByID),
			Assignment:   assignmentLabel(plan, job.ID),
		}
		if job.Priority == 1000 {
			lowEntries = append(lowEntries, entry)
		} else {
			normalEntries = append(normalEntries, entry)
		}
	}
	data.NormalEntries = normalEntries
	data.LowEntries = lowEntries
	s.render(w, s.queueTmpl, data)
}

func queueBanner(query string, plan *smartqueue.Plan) []string {
	var planErr string
	if plan != nil {
		planErr = plan.Err
	}
	var msgs []string
	if query != "" {
		msgs = append(msgs, query)
	}
	if planErr != "" && planErr != query {
		msgs = append(msgs, planErr)
	}
	return msgs
}

// currentPlan reads the cached plan. A nil scheduler is the same as a plan that has not been published yet.
func (s *Server) currentPlan() *smartqueue.Plan {
	if s.scheduler == nil {
		return nil
	}
	return s.scheduler.Snapshot()
}

func (s *Server) handleSmartQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("enabled")
	if enabled != "on" && enabled != "off" {
		redirectQueue(w, r, "invalid smart queue value")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var err error
	if enabled == "on" {
		err = s.scheduler.Enable(ctx)
	} else {
		err = s.scheduler.Disable(ctx)
	}
	if err != nil {
		slog.Error("web: failed to update smart queue", "error", err)
		redirectQueue(w, r, "Smart queue update failed: "+err.Error())
		return
	}
	redirectQueue(w, r, "")
}

func redirectQueue(w http.ResponseWriter, r *http.Request, errorText string) {
	loc := "/queue"
	if errorText != "" {
		loc += "?error=" + strings.ReplaceAll(url.QueryEscape(errorText), "+", "%20")
	}
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

// assignmentLabel is the assignment cell for one visible pending job.
// A nil plan, or a job the plan does not list, is still being calculated.
func assignmentLabel(plan *smartqueue.Plan, jobID string) string {
	if plan == nil {
		return "Calculating…"
	}
	row, ok := plan.Rows[jobID]
	if !ok {
		return "Calculating…"
	}
	switch row.Status {
	case smartqueue.StatusAssigned:
		if row.PrinterName != "" {
			return row.PrinterName
		}
		return row.PrinterID
	case smartqueue.StatusWaiting:
		return "Waiting for a printer"
	case smartqueue.StatusIncompatible:
		return "No compatible printer"
	case smartqueue.StatusMerged:
		return "Part of a merged plate"
	case smartqueue.StatusUnreadable:
		return "Unreadable job age"
	default:
		return "Calculating…"
	}
}

// validJobID matches the Printago job ID format: 24 lowercase hex characters.
var validJobID = regexp.MustCompile(`^[a-z0-9]{24}$`)

func (s *Server) handlePrioritizeJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	jobID := r.FormValue("id")
	if !validJobID.MatchString(jobID) {
		http.Redirect(w, r, "/queue?error="+url.QueryEscape("invalid job id"), http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.scheduler.Prioritize(ctx, jobID); err != nil {
		slog.Error("web: failed to prioritize print job", "job", jobID, "error", err) //nolint:gosec // jobID is validated against ^[a-z0-9]{24}$ above
		redirectQueue(w, r, "Failed to prioritize job: "+err.Error())
		return
	}
	redirectQueue(w, r, "")
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	jobID := r.FormValue("id")
	if !validJobID.MatchString(jobID) {
		http.Redirect(w, r, "/queue?error="+url.QueryEscape("invalid job id"), http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.client.CancelPrintJob(ctx, jobID); err != nil {
		slog.Error("web: failed to cancel print job", "job", jobID, "error", err) //nolint:gosec // jobID is validated against ^[a-z0-9]{24}$ above
		http.Redirect(w, r, "/queue?error="+url.QueryEscape("Failed to cancel job: "+err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/queue", http.StatusSeeOther)
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
