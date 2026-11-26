package smartqueue

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
)

// Config is the scheduler's half-life and state file.
type Config struct {
	HalfLife  time.Duration
	StateFile string
}

// Scheduler turns smart queue on and publishes the plan cache.
type Scheduler struct {
	client printago.ClientInterface
	cfg    Config
	cache  PlanCache
	mu     sync.Mutex
}

// NewScheduler returns a scheduler that reads and writes cfg.StateFile.
func NewScheduler(client printago.ClientInterface, cfg Config) *Scheduler {
	return &Scheduler{client: client, cfg: cfg}
}

// Snapshot returns the published plan. It is nil before the first publish.
func (s *Scheduler) Snapshot() *Plan {
	return s.cache.Snapshot()
}

// Enable turns smart queue on.
// It saves enabled and the merging snapshot, pauses Printago matching and merging,
// reserves the new plan, and runs the queue when an assigned printer is ready.
func (s *Scheduler) Enable(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.client.GetStoreSettings(ctx)
	if err != nil {
		return err
	}
	state, err := LoadState(s.cfg.StateFile)
	if err != nil {
		return err
	}
	state.Enabled = true
	state.HasMergingSnapshot = true
	state.AutomaticMerging = settings.PlateMerging.AutomaticMerging
	if err = SaveState(s.cfg.StateFile, state); err != nil {
		return err
	}
	s.publishEnabled()

	matching := false
	merging := false
	if err = s.client.UpdateStoreSettings(ctx, printago.StoreSettingsUpdate{
		AutomaticQueueMatching: &matching,
		PlateMerging:           &printago.PlateMergingUpdate{AutomaticMerging: &merging},
	}); err != nil {
		return err
	}

	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		return err
	}
	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		return err
	}
	matchingResult, err := s.client.GroupMatching(ctx, jobIDs(jobs))
	if err != nil {
		return err
	}
	groups, err := s.planGroups(ctx, matchingResult)
	if err != nil {
		return err
	}

	plan := BuildPlan(PlanInput{
		Jobs:         planJobs(jobs),
		PrinterNames: printerNames(printers),
		Groups:       groups,
		HalfLife:     s.cfg.HalfLife,
		Now:          time.Now(),
		Prioritized:  idSet(state.PrioritizedJobIDs),
	})
	plan.Enabled = true
	if err = Reserve(plan, reserveJobs(jobs), func(id string, tags map[string]string) error {
		return s.client.UpdatePrintJobTags(ctx, id, tags)
	}); err != nil {
		return err
	}
	s.cache.Publish(plan)
	if !assignedPrinterReady(plan, groups) {
		return nil
	}
	if err = s.client.RunQueue(ctx); err != nil {
		plan.Err = err.Error()
		s.cache.Publish(plan)
		return err
	}
	return nil
}

// publishEnabled reports the switch on without replacing the published rows.
func (s *Scheduler) publishEnabled() {
	next := Plan{Enabled: true}
	if current := s.cache.Snapshot(); current != nil {
		next.Rows = current.Rows
		next.Err = current.Err
	}
	s.cache.Publish(next)
}

func (s *Scheduler) planGroups(ctx context.Context, result printago.GroupMatchingResult) ([]PlanGroup, error) {
	groups := make([]PlanGroup, 0, len(result.Groups))
	for _, group := range result.Groups {
		var checks map[string][]Check
		if tagsMatchFailure(group.Printers) {
			trouble, err := s.client.MatchingTroubleshoot(ctx, group.RepresentativePrintJobID)
			if err != nil {
				return nil, err
			}
			checks = troubleChecks(trouble)
		}
		groups = append(groups, PlanGroup{
			MemberIDs:        group.MemberPrintJobIDs,
			RepresentativeID: group.RepresentativePrintJobID,
			Verdicts:         verdictsOf(group.Printers),
			Checks:           checks,
		})
	}
	return groups, nil
}

func tagsMatchFailure(printers []printago.MatchPrinterVerdict) bool {
	for _, printer := range printers {
		if printer.FirstFailedCheck == "tags-match" {
			return true
		}
	}
	return false
}

func troubleChecks(result printago.MatchingTroubleshootResult) map[string][]Check {
	checks := make(map[string][]Check, len(result.Printers))
	for _, printer := range result.Printers {
		converted := make([]Check, len(printer.Checks))
		for i, check := range printer.Checks {
			comparisons := make([]Comparison, len(check.Comparisons))
			for j, comparison := range check.Comparisons {
				comparisons[j] = Comparison{
					Key:         comparison.Key,
					Requirement: comparison.Requirement,
					Matched:     comparison.Matched,
				}
			}
			converted[i] = Check{
				Code:        check.Code,
				Passed:      check.Passed,
				Comparisons: comparisons,
			}
		}
		checks[printer.PrinterID] = converted
	}
	return checks
}

func verdictsOf(printers []printago.MatchPrinterVerdict) []Verdict {
	out := make([]Verdict, len(printers))
	for i, printer := range printers {
		out[i] = Verdict{
			PrinterID:        printer.PrinterID,
			Matched:          printer.Matched,
			Compatible:       printer.Compatible,
			FirstFailedCheck: printer.FirstFailedCheck,
		}
	}
	return out
}

func jobIDs(jobs []printago.PrintJob) []string {
	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}

func planJobs(jobs []printago.PrintJob) []PlanJob {
	out := make([]PlanJob, len(jobs))
	for i, job := range jobs {
		parent := ""
		if job.ParentJobID != nil {
			parent = *job.ParentJobID
		}
		out[i] = PlanJob{
			ID:          job.ID,
			ParentJobID: parent,
			Priority:    job.Priority,
			CreatedAt:   job.CreatedAt,
		}
	}
	return out
}

func printerNames(printers []printago.Printer) map[string]string {
	names := make(map[string]string, len(printers))
	for _, printer := range printers {
		names[printer.ID] = printer.Name
	}
	return names
}

func reserveJobs(jobs []printago.PrintJob) []ReserveJob {
	out := make([]ReserveJob, len(jobs))
	for i, job := range jobs {
		out[i] = ReserveJob{ID: job.ID, RequiredTags: job.RequiredPrinterTags}
	}
	return out
}

func idSet(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// assignedPrinterReady reports whether a plan assigns a job to an eligible printer
// that can start. An empty first failed check is ready, and that signal does not
// need a troubleshoot list. When the printer's checks include ready, it must have passed.
func assignedPrinterReady(plan Plan, groups []PlanGroup) bool {
	for jobID, row := range plan.Rows {
		if row.Status != StatusAssigned {
			continue
		}
		verdict, checks, ok := memberVerdict(groups, jobID, row.PrinterID)
		if ok && Eligible([]Verdict{verdict}, verdict.PrinterID) && printerReady(verdict, checks) {
			return true
		}
	}
	return false
}

func printerReady(verdict Verdict, checks []Check) bool {
	sawReady := false
	for _, check := range checks {
		if check.Code != "ready" {
			continue
		}
		sawReady = true
		if !check.Passed {
			return false
		}
	}
	if sawReady {
		return true
	}
	return verdict.FirstFailedCheck != "ready"
}

func memberVerdict(groups []PlanGroup, jobID, printerID string) (Verdict, []Check, bool) {
	for _, group := range groups {
		if !slices.Contains(group.MemberIDs, jobID) {
			continue
		}
		for _, verdict := range group.Verdicts {
			if verdict.PrinterID == printerID {
				return verdict, group.Checks[printerID], true
			}
		}
	}
	return Verdict{}, nil, false
}
