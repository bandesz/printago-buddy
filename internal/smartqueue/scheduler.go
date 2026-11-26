package smartqueue

import (
	"context"
	"errors"
	"maps"
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
	client          printago.ClientInterface
	cfg             Config
	cache           PlanCache
	mu              sync.Mutex
	settingsPending bool
	applyPending    bool
	dispatchPending bool
	hasSig          bool
	assignmentSig   string
	dispatchSig     string
	pendingPlan     *Plan
}

// errMatchingTruncated is returned when a group-matching request that names
// job ids is still truncated.
var errMatchingTruncated = errors.New("group matching truncated")

// NewScheduler returns a scheduler that reads and writes cfg.StateFile.
// settingsPending starts set so the first enabled poll pauses Printago matching.
func NewScheduler(client printago.ClientInterface, cfg Config) *Scheduler {
	return &Scheduler{client: client, cfg: cfg, settingsPending: true}
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
	s.settingsPending = true
	s.publishEnabled()

	if err = s.pauseMatching(ctx); err != nil {
		return err
	}
	s.settingsPending = false

	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		s.markApplyPending(nil)
		return err
	}
	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		s.markApplyPending(nil)
		return err
	}
	matchingResult, err := s.client.GroupMatching(ctx, jobIDs(jobs))
	if err != nil {
		s.markApplyPending(nil)
		return err
	}
	groups, err := s.planGroups(ctx, matchingResult)
	if err != nil {
		s.markApplyPending(nil)
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
	if err = s.reserve(ctx, plan, jobs); err != nil {
		copied := plan
		s.markApplyPending(&copied)
		return s.failPublish(err)
	}
	s.applyPending = false
	s.pendingPlan = nil
	s.cache.Publish(plan)
	if err = s.runIfReady(ctx, plan, groups); err != nil {
		plan.Err = err.Error()
		s.cache.Publish(plan)
		return err
	}
	return nil
}

// Disable turns smart queue off.
// It clears reservation tags, restores Printago matching and the saved merging
// value, then saves enabled false. A tag or settings error leaves the switch on.
func (s *Scheduler) Disable(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := LoadState(s.cfg.StateFile)
	if err != nil {
		return err
	}
	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.ParentJobID != nil && *job.ParentJobID != "" {
			continue
		}
		next, clearErr := ClearTags(job.RequiredPrinterTags)
		if clearErr != nil {
			return clearErr
		}
		if maps.Equal(job.RequiredPrinterTags, next) {
			continue
		}
		if patchErr := s.client.UpdatePrintJobTags(ctx, job.ID, next); patchErr != nil {
			return patchErr
		}
	}

	matching := true
	merging := false
	if state.HasMergingSnapshot {
		merging = state.AutomaticMerging
	}
	if err = s.client.UpdateStoreSettings(ctx, printago.StoreSettingsUpdate{
		AutomaticQueueMatching: &matching,
		PlateMerging:           &printago.PlateMergingUpdate{AutomaticMerging: &merging},
	}); err != nil {
		return err
	}

	state.Enabled = false
	state.HasMergingSnapshot = false
	if err = SaveState(s.cfg.StateFile, state); err != nil {
		return err
	}
	s.publishSwitch(false)
	return nil
}

// Prioritize marks jobID so it outweighs other jobs in its lane.
// The id is saved before any Printago write. A second click does nothing.
// With the switch off, the first click moves the job to the front of the Printago queue.
// With the switch on, reservations are rewritten and move-to-front is not called.
func (s *Scheduler) Prioritize(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := LoadState(s.cfg.StateFile)
	if err != nil {
		return err
	}
	if slices.Contains(state.PrioritizedJobIDs, jobID) {
		return nil
	}
	state.PrioritizedJobIDs = append(state.PrioritizedJobIDs, jobID)
	if err = SaveState(s.cfg.StateFile, state); err != nil {
		return err
	}
	if !state.Enabled {
		if err = s.client.PrioritizePrintJob(ctx, jobID); err != nil {
			state.PrioritizedJobIDs = removeID(state.PrioritizedJobIDs, jobID)
			if saveErr := SaveState(s.cfg.StateFile, state); saveErr != nil {
				return saveErr
			}
			return err
		}
	}
	return s.recompute(ctx, state)
}

// recompute publishes a fresh plan from the saved state without waiting for the next poll.
func (s *Scheduler) recompute(ctx context.Context, state State) error {
	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		return err
	}
	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		return err
	}
	slots, err := s.client.GetPrinterSlots(ctx)
	if err != nil {
		return err
	}
	groups, err := s.loadGroups(ctx, jobs)
	if err != nil {
		return err
	}
	input := signatureInput(jobs, printers, slots, groups, state.PrioritizedJobIDs)
	return s.publishRecomputed(ctx, state, jobs, printers, groups, AssignmentSignature(input), DispatchSignature(input))
}

// Poll reads the pending queue and publishes the assignment plan.
// It patches Printago when the switch is on and a signature changed, and when a
// turn-on settings patch, reservation diff, or queue run is still unfinished.
// The poll that finishes a pending settings patch continues into reserve and run
// unless a reservation plan is already waiting.
func (s *Scheduler) Poll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := LoadState(s.cfg.StateFile)
	if err != nil {
		return s.failPublish(err)
	}
	jobs, err := s.client.GetPrintJobs(ctx)
	if err != nil {
		return s.failPublish(err)
	}
	state, err = s.dropStalePriorities(state, jobs)
	if err != nil {
		return s.failPublish(err)
	}
	printers, err := s.client.GetPrinters(ctx)
	if err != nil {
		return s.failPublish(err)
	}
	slots, err := s.client.GetPrinterSlots(ctx)
	if err != nil {
		return s.failPublish(err)
	}
	groups, err := s.loadGroups(ctx, jobs)
	if err != nil {
		return s.failPublish(err)
	}

	input := signatureInput(jobs, printers, slots, groups, state.PrioritizedJobIDs)
	assignSig := AssignmentSignature(input)
	dispatchSig := DispatchSignature(input)
	assignmentChanged := !s.hasSig || assignSig != s.assignmentSig
	dispatchChanged := !s.hasSig || dispatchSig != s.dispatchSig

	resumedTurnOn := false
	if state.Enabled && s.settingsPending {
		if err = s.pauseMatching(ctx); err != nil {
			return s.failPublish(err)
		}
		s.settingsPending = false
		resumedTurnOn = true
	}

	waitingPlan := state.Enabled && s.applyPending && s.pendingPlan != nil
	if assignmentChanged || (state.Enabled && s.applyPending && s.pendingPlan == nil) || (resumedTurnOn && !waitingPlan) {
		return s.publishRecomputed(ctx, state, jobs, printers, groups, assignSig, dispatchSig)
	}
	if waitingPlan {
		return s.retryApply(ctx, state, jobs, groups, assignSig, dispatchSig)
	}
	if state.Enabled && (dispatchChanged || s.dispatchPending) {
		return s.retryDispatch(ctx, groups, assignSig, dispatchSig)
	}
	s.storeSigs(assignSig, dispatchSig)
	if !state.Enabled {
		s.dispatchPending = false
	}
	s.clearErr()
	return nil
}

func (s *Scheduler) publishRecomputed(ctx context.Context, state State, jobs []printago.PrintJob, printers []printago.Printer, groups []PlanGroup, assignSig, dispatchSig string) error {
	plan := BuildPlan(PlanInput{
		Jobs:         planJobs(jobs),
		PrinterNames: printerNames(printers),
		Groups:       groups,
		HalfLife:     s.cfg.HalfLife,
		Now:          time.Now(),
		Prioritized:  idSet(state.PrioritizedJobIDs),
	})
	plan.Enabled = state.Enabled
	if !state.Enabled {
		s.applyPending = false
		s.pendingPlan = nil
		s.dispatchPending = false
		s.storeSigs(assignSig, dispatchSig)
		s.cache.Publish(plan)
		return nil
	}
	if err := s.reserve(ctx, plan, jobs); err != nil {
		s.applyPending = true
		copied := plan
		s.pendingPlan = &copied
		s.storeSigs(assignSig, dispatchSig)
		return s.failPublish(err)
	}
	s.applyPending = false
	s.pendingPlan = nil
	s.storeSigs(assignSig, dispatchSig)
	if err := s.runIfReady(ctx, plan, groups); err != nil {
		plan.Err = err.Error()
		s.cache.Publish(plan)
		return err
	}
	s.cache.Publish(plan)
	return nil
}

func (s *Scheduler) retryApply(ctx context.Context, state State, jobs []printago.PrintJob, groups []PlanGroup, assignSig, dispatchSig string) error {
	plan := *s.pendingPlan
	plan.Enabled = state.Enabled
	if err := s.reserve(ctx, plan, jobs); err != nil {
		return s.failPublish(err)
	}
	s.applyPending = false
	s.pendingPlan = nil
	s.storeSigs(assignSig, dispatchSig)
	if err := s.runIfReady(ctx, plan, groups); err != nil {
		plan.Err = err.Error()
		s.cache.Publish(plan)
		return err
	}
	s.cache.Publish(plan)
	return nil
}

func (s *Scheduler) retryDispatch(ctx context.Context, groups []PlanGroup, assignSig, dispatchSig string) error {
	s.storeSigs(assignSig, dispatchSig)
	current := s.cache.Snapshot()
	if current == nil || !assignedPrinterReady(*current, groups) {
		s.dispatchPending = false
		s.clearErr()
		return nil
	}
	if err := s.client.RunQueue(ctx); err != nil {
		s.dispatchPending = true
		return s.failPublish(err)
	}
	s.dispatchPending = false
	s.clearErr()
	return nil
}

func (s *Scheduler) markApplyPending(plan *Plan) {
	s.applyPending = true
	s.pendingPlan = plan
}

func (s *Scheduler) reserve(ctx context.Context, plan Plan, jobs []printago.PrintJob) error {
	return Reserve(plan, reserveJobs(jobs), func(id string, tags map[string]string) error {
		return s.client.UpdatePrintJobTags(ctx, id, tags)
	})
}

func (s *Scheduler) runIfReady(ctx context.Context, plan Plan, groups []PlanGroup) error {
	if !assignedPrinterReady(plan, groups) {
		s.dispatchPending = false
		return nil
	}
	if err := s.client.RunQueue(ctx); err != nil {
		s.dispatchPending = true
		return err
	}
	s.dispatchPending = false
	return nil
}

func (s *Scheduler) pauseMatching(ctx context.Context) error {
	matching := false
	merging := false
	return s.client.UpdateStoreSettings(ctx, printago.StoreSettingsUpdate{
		AutomaticQueueMatching: &matching,
		PlateMerging:           &printago.PlateMergingUpdate{AutomaticMerging: &merging},
	})
}

func (s *Scheduler) dropStalePriorities(state State, jobs []printago.PrintJob) (State, error) {
	if len(state.PrioritizedJobIDs) == 0 {
		return state, nil
	}
	pending := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		pending[job.ID] = struct{}{}
	}
	kept := make([]string, 0, len(state.PrioritizedJobIDs))
	changed := false
	for _, id := range state.PrioritizedJobIDs {
		if _, ok := pending[id]; !ok {
			changed = true
			continue
		}
		kept = append(kept, id)
	}
	if !changed {
		return state, nil
	}
	state.PrioritizedJobIDs = kept
	if err := SaveState(s.cfg.StateFile, state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Scheduler) loadGroups(ctx context.Context, jobs []printago.PrintJob) ([]PlanGroup, error) {
	ids := jobIDs(jobs)
	result, err := s.client.GroupMatching(ctx, ids)
	if err != nil {
		return nil, err
	}
	if result.Truncated {
		missing := missingMemberIDs(ids, result.Groups)
		retry, retryErr := s.client.GroupMatching(ctx, missing)
		if retryErr != nil {
			return nil, retryErr
		}
		if retry.Truncated {
			return nil, errMatchingTruncated
		}
		result.Groups = append(result.Groups, retry.Groups...)
	}
	return s.planGroups(ctx, result)
}

func (s *Scheduler) failPublish(err error) error {
	if current := s.cache.Snapshot(); current != nil {
		current.Err = err.Error()
		s.cache.Publish(*current)
	}
	return err
}

func (s *Scheduler) clearErr() {
	current := s.cache.Snapshot()
	if current == nil || current.Err == "" {
		return
	}
	current.Err = ""
	s.cache.Publish(*current)
}

func (s *Scheduler) storeSigs(assignment, dispatch string) {
	s.hasSig = true
	s.assignmentSig = assignment
	s.dispatchSig = dispatch
}

func signatureInput(jobs []printago.PrintJob, printers []printago.Printer, slots []printago.PrinterSlot, groups []PlanGroup, prioritized []string) SignatureInput {
	sigJobs := make([]SignatureJob, len(jobs))
	for i, job := range jobs {
		sigJobs[i] = SignatureJob{
			ID:           job.ID,
			Priority:     job.Priority,
			CreatedAt:    job.CreatedAt,
			RequiredTags: job.RequiredPrinterTags,
		}
	}
	slotsByPrinter := make(map[string][]SignatureSlot)
	for _, slot := range slots {
		slotsByPrinter[slot.PrinterID] = append(slotsByPrinter[slot.PrinterID], SignatureSlot{
			MaterialID: derefString(slot.MaterialID),
			VariantID:  derefString(slot.VariantID),
		})
	}
	sigPrinters := make([]SignaturePrinter, len(printers))
	for i, printer := range printers {
		sigPrinters[i] = SignaturePrinter{
			ID:     printer.ID,
			Tags:   printer.Tags,
			Slots:  slotsByPrinter[printer.ID],
			Checks: printerChecks(printer.ID, groups),
		}
	}
	return SignatureInput{
		Jobs:              sigJobs,
		Printers:          sigPrinters,
		PrioritizedJobIDs: prioritized,
	}
}

func printerChecks(printerID string, groups []PlanGroup) []Check {
	var checks []Check
	for _, group := range groups {
		verdict, ok := findVerdict(group.Verdicts, printerID)
		if !ok {
			continue
		}
		if existing, found := group.Checks[printerID]; found {
			checks = append(checks, existing...)
			continue
		}
		checks = append(checks, synthesizedChecks(verdict)...)
	}
	return checks
}

func synthesizedChecks(verdict Verdict) []Check {
	var checks []Check
	if verdict.FirstFailedCheck != "" {
		checks = append(checks, Check{Code: verdict.FirstFailedCheck, Passed: false})
	}
	if printerCanStart(verdict) {
		checks = append(checks, Check{Code: "ready", Passed: true})
	}
	return checks
}

func printerCanStart(verdict Verdict) bool {
	if !verdictEligible(verdict) {
		return false
	}
	if verdict.FirstFailedCheck == "" {
		return true
	}
	return nonBlockingFailure(verdict.FirstFailedCheck) && verdict.FirstFailedCheck != "ready"
}

func missingMemberIDs(ids []string, groups []printago.MatchGroup) []string {
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, id := range group.MemberPrintJobIDs {
			seen[id] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		missing = append(missing, id)
	}
	return missing
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// publishEnabled reports the switch on without replacing the published rows.
func (s *Scheduler) publishEnabled() {
	s.publishSwitch(true)
}

// publishSwitch reports the switch without replacing the published rows.
func (s *Scheduler) publishSwitch(enabled bool) {
	next := Plan{Enabled: enabled}
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

func removeID(ids []string, drop string) []string {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			kept = append(kept, id)
		}
	}
	return kept
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
