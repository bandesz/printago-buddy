# Tasks

Batches run in order. Tasks inside a batch run in order. `user-flows.md` wins if a task and a flow disagree; fix the task before implementing.

Domain skills: none.

## Batch 1: Matching core
**Status:** in progress


No Printago calls. No web UI.

### Task 1.1: Age weight
**Status:** in progress
- Files: Create `internal/smartqueue/weight.go`, `internal/smartqueue/weight_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestWeight|TestScaleWeights'`
- Acceptance:
  - Age zero yields weight 1
  - One half-life yields weight 2
  - Two half-lives yields weight 4
  - A negative age yields weight 1
  - Scaling an empty slice returns an empty slice
  - Scaling divides every weight by the maximum, so the largest becomes 1 and a half-weight stays half
- Out of scope: Matching, lanes, prioritize

### Task 1.2: Maximum-weight assignment
**Status:** pending
- Files: Create `internal/smartqueue/matching.go`, `internal/smartqueue/matching_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestHeavyJob|TestLane|TestPrioritize'`
- Acceptance:
  - `TestHeavyJobDoesNotTakeTheOnlyPrinterOfALighterJob`: H can use A and B, L can use only A, H weighs more than L. A is assigned to L and B to H
  - A printer with an edge is used when the job weight is positive and the printer is free
  - A job with no edge is unmatched
  - Normal jobs are assigned before low jobs. A low job does not receive a printer a normal job can use
  - Inside the normal lane, prioritized jobs are assigned before jobs that are not prioritized
  - Two prioritized normal jobs: the heavier age weight gets the contested printer
  - A prioritized low job does not receive a printer a normal job can use
- Out of scope: Tag strings, Printago verdicts

### Task 1.3: Reservation tag edits
**Status:** pending
- Files: Create `internal/smartqueue/tags.go`, `internal/smartqueue/tags_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestPark|TestPin|TestClearTags'`
- Acceptance:
  - Empty `user.tags` plus park becomes `&=buddy-hold`. Nozzle, model, and filament keys are copied
  - `&=a,b` plus park becomes `&=a,b,buddy-hold`
  - Parking twice stays `&=a,b,buddy-hold`
  - Removing park from `&=a,buddy-hold` leaves `&=a`
  - Removing park from `&=buddy-hold` omits `user.tags`
  - Pinning sets `printer.id` to the given id and removes `buddy-hold`
  - Clearing removes `printer.id` and `buddy-hold` and leaves the other keys
  - `user.tags` that is not empty and does not start with `&=` returns an error and the input tags are unchanged
- Out of scope: HTTP PATCH

### Task 1.4: Edges from matcher verdicts
**Status:** pending
- Files: Create `internal/smartqueue/edges.go`, `internal/smartqueue/edges_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestEdge|TestEligible'`
- Acceptance:
  - `firstFailedCheck` `ready` yields an eligible printer and an edge
  - `enabled`, `online`, `available`, or `in-maintenance` yields an ineligible printer and no edge
  - `material-match` yields an eligible printer and no edge
  - The same no-edge result holds for `profiles-configured`, `slots-loaded`, `routing-rules`, `filament-profiles`, and `bed-fits`
  - `staggered-start-hold` and `subscription-slots` yield an edge
  - `matched` true yields an edge
  - `tags-match` yields an edge when the only failing tag comparisons are `printer.id` or `buddy-hold` and every other check passes or is one of `ready`, `staggered-start-hold`, `subscription-slots`
  - `tags-match` yields no edge when another required tag fails
  - A printer absent from the verdicts is ineligible
- Out of scope: HTTP, group truncation

### Task 1.5: Change signature
**Status:** pending
- Files: Create `internal/smartqueue/signature.go`, `internal/smartqueue/signature_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestAssignmentSignature|TestDispatchSignature'`
- Acceptance:
  - Equal inputs produce equal assignment signatures
  - Adding or removing `printer.id` or `buddy-hold` does not change the assignment signature
  - A new job, a missing job, a priority change, a slot material change, a printer tag change, or an online/maintenance change changes the assignment signature
  - A ready-only change keeps the assignment signature and changes the dispatch signature
  - Adding a job id to the prioritize set changes the assignment signature
- Out of scope: The poll loop

## Batch 2: Printago client
**Status:** pending


### Task 2.1: List every visible pending job
**Status:** pending
- Files: Modify `internal/printago/client.go`, `internal/printago/client_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ -count=1 -run TestGetPrintJobsPages`
- Acceptance:
  - Requests keep `status=pending`, `hidden=false`, `sort=queueOrder`, `include=partBuild`, and `limit=200`
  - A 200-row page is followed by `offset=200`. A shorter page stops the loop
  - The returned slice is page one then page two
  - Callers of `GetPrintJobs` are `internal/web/handlers.go` `handleQueue` and, after batch 3, the scheduler. Both receive the concatenated list
- Out of scope: New endpoints

### Task 2.2: Store settings get and patch
**Status:** pending
- Files: Modify `internal/printago/types.go`, `internal/printago/client.go`, `internal/printago/client_test.go`, `internal/printago/caching_client.go`, `internal/printago/caching_client_test.go`, `internal/jobs/filament_tagger_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ ./internal/jobs/ -count=1 -run 'TestStoreSettings|TestFilament'`
- Acceptance:
  - `GetStoreSettings` `GET /v1/settings/store` returns `automaticQueueMatching` and `plateMerging.automaticMerging`
  - `UpdateStoreSettings` `PATCH /v1/settings/store` sends only the fields it is given
  - `CachingClient` calls the inner client on every get and update
  - `ClientInterface` includes the new methods. `stubClient` and `mockClient` compile
- Out of scope: The enable sequence

### Task 2.3: Patch job tags
**Status:** pending
- Files: Modify `internal/printago/client.go`, `internal/printago/client_test.go`, `internal/printago/caching_client.go`, `internal/printago/types.go`, and the two test doubles
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ -count=1 -run TestUpdatePrintJobTags`
- Acceptance:
  - `UpdatePrintJobTags` `PATCH /v1/print-jobs/{id}` sends JSON `{"requiredPrinterTags": ...}` and no other job fields
  - `PrintJob` decodes `parentJobId` and `groupId`
  - `RequiredPrinterTags` decodes every string key, including `printer.id`, `user.tags`, `printer.nozzleDiameter`, `printer.modelName`, and `filament.type`. Absent keys stay absent
  - `CachingClient` forwards every call
- Out of scope: Building the tag object

### Task 2.4: Group matching
**Status:** pending
- Files: Modify `internal/printago/client.go`, `internal/printago/types.go`, `internal/printago/client_test.go`, `internal/printago/caching_client.go`, and the two test doubles
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ -count=1 -run TestGroupMatching`
- Acceptance:
  - `POST /v1/print-jobs/queue/group-matching` sends `use` `direct` and the given `printJobIds`
  - The decoded value keeps `truncated`, `memberPrintJobIds`, `representativePrintJobId`, `printerId`, `matched`, `compatible`, and `firstFailedCheck`
  - `CachingClient` forwards every call
- Out of scope: The truncation retry loop

### Task 2.5: Matching troubleshoot
**Status:** pending
- Files: Modify `internal/printago/client.go`, `internal/printago/types.go`, `internal/printago/client_test.go`, `internal/printago/caching_client.go`, and the two test doubles
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ -count=1 -run TestMatchingTroubleshoot`
- Acceptance:
  - `GET /v1/print-jobs/{id}/matching-troubleshoot` decodes each printer's checks, including `code`, `passed`, and tag comparisons `key`, `requirement`, and `matched`
  - `CachingClient` forwards every call
- Out of scope: Deciding edges

### Task 2.6: Manual queue run
**Status:** pending
- Files: Modify `internal/printago/client.go`, `internal/printago/client_test.go`, `internal/printago/caching_client.go`, and the two test doubles
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/printago/ -count=1 -run TestRunQueue`
- Acceptance:
  - `RunQueue` `POST /v1/print-jobs/queue/run` sends `{"manual": true}`
  - `CachingClient` forwards every call
- Out of scope: Deciding when to call it

## Batch 3: Scheduler
**Status:** pending


### Task 3.1: Configuration
**Status:** pending
- Files: Modify `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/config/ -count=1 -run 'TestLoad|TestHalfLife|TestPoll|TestStateFile'`
- Acceptance:
  - Missing optional vars yield half-life `1h`, poll `15s`, and state file `/data/smart-queue.json`
  - `QUEUE_WEIGHT_HALF_LIFE=30m` and `QUEUE_POLL_INTERVAL=5s` are kept
  - `QUEUE_WEIGHT_HALF_LIFE=0`, a negative duration, or `abc` makes `Load` return an error
  - The same rejection applies to `QUEUE_POLL_INTERVAL`
  - `.env.example` lists the three variables with those defaults
- Out of scope: Creating the directory

### Task 3.2: State file
**Status:** pending
- Files: Create `internal/smartqueue/state.go`, `internal/smartqueue/state_test.go`
- TDD: yes
- UI flow: N/A
- Verify: `go test ./internal/smartqueue/ -count=1 -run TestState`
- Acceptance:
  - A missing file loads `enabled` false, `hasMergingSnapshot` false, and no prioritize ids
  - Save then load returns `enabled`, `hasMergingSnapshot`, `automaticMerging`, and the prioritize ids
  - A directory that cannot be created returns an error and leaves no partial file that loads as enabled
- Out of scope: Printago

### Task 3.3: Build and publish a plan
**Status:** pending
- Files: Create `internal/smartqueue/plan.go`, `internal/smartqueue/plan_test.go`
- TDD: yes
- UI flow: Flow 6, Flow 12
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestBuildPlan|TestSnapshot'`
- Acceptance:
  - `Snapshot` returns a copy. Mutating the copy does not change the published plan
  - A matched job has status `assigned` and the printer name from the printer list. A missing name uses the printer id
  - An eligible job that loses the match has status `waiting`
  - A job with no edge has status `incompatible`
  - A job with `parentJobId` set has status `merged` and is not a vertex
  - A job with an unparseable `createdAt` has status `unreadable` and is not a vertex
  - Group members share edges and keep their own weights
- Out of scope: HTTP and tag PATCH

### Task 3.4: Reservation diff
**Status:** pending
- Files: Create `internal/smartqueue/reserve.go`, `internal/smartqueue/reserve_test.go`
- TDD: yes
- UI flow: Flow 6, Flow 7
- Verify: `go test ./internal/smartqueue/ -count=1 -run TestReserve`
- Acceptance:
  - An assigned job is patched only when `printer.id` or `buddy-hold` differs from the pin rule
  - An unassigned managed job is patched only when it still has `printer.id` or lacks `buddy-hold`
  - A merged child and an unreadable job are not patched
  - A tag expression that is not an `&=` list returns an error, does not patch that job, and does not call queue run
  - A PATCH error returns after the failed job. Later jobs in the diff are not patched
  - Writers of `printer.id` and `buddy-hold`: this diff, turn-off cleanup, and turn-on because it calls this diff. Readers: the edge builder, which ignores those two values; the assignment signature, which strips them; Printago's matcher, which honors them. The queue page does not read them
- Out of scope: Persisting the plan

### Task 3.5: Turn on
**Status:** pending
- Files: Create `internal/smartqueue/scheduler.go`, `internal/smartqueue/scheduler_test.go`
- TDD: yes
- UI flow: Flow 3
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestEnable'`
- Acceptance:
  - Enable reads settings, saves `enabled` true and the merging snapshot, then patches `automaticQueueMatching` false and `automaticMerging` false
  - It writes reservations, publishes the plan, and calls `RunQueue` when an assigned printer is ready
  - It does not call `RunQueue` when no assigned printer is ready
  - A `RunQueue` error after a successful diff still publishes the plan, sets `Err`, and returns that error
  - A state-file error before the flag is saved returns the error, leaves `enabled` false, and does not PATCH settings
  - A settings PATCH error after the flag is saved leaves `enabled` true and returns the error
  - After the state file is saved, `Snapshot` reports `Enabled` true even when a later settings PATCH, reservation write, or `RunQueue` fails
- Out of scope: The HTTP form

### Task 3.6: Turn off
**Status:** pending
- Files: Modify `internal/smartqueue/scheduler.go`, `internal/smartqueue/scheduler_test.go`
- TDD: yes
- UI flow: Flow 9
- Verify: `go test ./internal/smartqueue/ -count=1 -run TestDisable`
- Acceptance:
  - Disable removes `buddy-hold` and `printer.id` from managed pending jobs, patches `automaticQueueMatching` true, restores the saved `automaticMerging`, then saves `enabled` false and `hasMergingSnapshot` false
  - Prioritize ids remain
  - A tag or settings error leaves `enabled` true and does not write `enabled` false. `Snapshot` reports `Enabled` true
  - On success, `Snapshot` reports `Enabled` false
  - Merged children are not patched
- Out of scope: The HTTP form

### Task 3.7: Poll
**Status:** pending
- Files: Modify `internal/smartqueue/scheduler.go`, `internal/smartqueue/scheduler_test.go`
- TDD: yes
- UI flow: Flow 1, Flow 4, Flow 5, Flow 7, Flow 8
- Verify: `go test ./internal/smartqueue/ -count=1 -run 'TestPoll|TestStartup|TestReady|TestIncompatible|TestApplyFailure|TestNewJob'`
- Acceptance:
  - The poll loads pending jobs with `GetPrintJobs`
  - `TestStartupMissingStateFileDoesNotCallPrintago`: a missing file polls reads and group matching, publishes a plan, and does not PATCH settings or jobs
  - `TestIncompatibleJobIsParkedWhenEnabled`: a job with no edge is published as `incompatible` and, with the switch on, is patched to `buddy-hold` with no `printer.id`
  - An unchanged assignment and dispatch signature does no Printago work
  - `TestNewJobChangesAssignment`: a new job id recomputes the published plan. With the switch off, jobs are not patched
  - When `enabled` is true and the turn-on settings PATCH has not succeeded since the flag was saved, the poll PATCHes `automaticQueueMatching` false and `automaticMerging` false before reservations, even if both signatures match
  - `TestReadyChangeRunsQueueWithoutRecompute`: with the switch on, a ready-only change calls `RunQueue` and does not change pairs
  - A fetch, matching, or troubleshoot error keeps the published plan and sets `Err`
  - `TestApplyFailureKeepsPublishedPlan`: a failed reservation PATCH keeps the previous published rows, sets `Err`, and the next poll retries the diff when the signatures match
  - A failed `RunQueue` publishes the new rows, sets `Err`, and the next poll calls `RunQueue` again when the signatures match
  - An empty pending list publishes no rows
  - Prioritize ids missing from the pending list are removed from the state file before matching
  - Truncation retries with the missing job ids. Truncation of that named request sets `Err` and keeps the published plan
- Out of scope: Signal handling

### Task 3.8: Prioritize
**Status:** pending
- Files: Modify `internal/smartqueue/scheduler.go`, `internal/smartqueue/scheduler_test.go`
- TDD: yes
- UI flow: Flow 11
- Verify: `go test ./internal/smartqueue/ -count=1 -run TestPrioritize`
- Acceptance:
  - `TestPrioritizeBeatsOlderNormalJob`: a younger prioritized normal job takes the contested printer from an older normal job that is not prioritized
  - `TestPrioritizeLowDoesNotBeatNormal`: a prioritized low job does not take a printer a normal job can use
  - The mark is saved before reservations are written
  - When reservation writes fail after the mark is saved, the previous published rows stay, `Err` is set, and move-to-front is not called
  - With the switch on, move-to-front is not called
  - `TestPrioritizeWhileOffMovesToFrontOnce`: with the switch off, the first click calls move-to-front once and does not PATCH job tags. The second click does not call move-to-front and does not change the plan
  - A state-file error does not call move-to-front and does not add the id
  - Writers of the prioritize set: `Prioritize` and the poll's removal of ids that are no longer pending. Readers: lane matching, the assignment signature, and the state file
- Out of scope: The HTTP redirect

### Task 3.9: Process wiring
**Status:** pending
- Files: Modify `cmd/printago-buddy/main.go`, `Dockerfile`, `docker-compose.yml`, `AGENTS.md`, `internal/smartqueue/scheduler_test.go`, `internal/web/server.go`, `internal/web/queue_test.go`
- TDD: no
- UI flow: Flow 10
- Verify: `go test ./internal/smartqueue/ ./internal/web/ -count=1 -run 'TestShutdown|TestResume|TestGroupQueue|TestQueueTemplate'`
- Acceptance:
  - `TestShutdownDoesNotDisable`: canceling the poll context does not PATCH settings or jobs
  - `TestRestartResumesEnabled`: a state file with `enabled` true patches matching off and merging off, then applies reservations
  - `main` starts the scheduler and passes it to the web server. `NewServer` accepts that scheduler. Existing queue template tests still pass. The assignment column is not rendered yet
  - The image creates `/data` owned by `app` before the process drops to `app`
  - Compose mounts a named volume at `/data`
  - `AGENTS.md` lists `settings.edit`, `job.edit.all`, the three environment variables, and the scheduler
- Out of scope: Changing the filament tagger schedule

## Batch 4: Queue page
**Status:** pending


UI and the tests below ship in this batch.

### Task 4.1: Assignment column from the cache
**Status:** pending
- Files: Modify `internal/web/handlers.go`, `internal/web/server.go`, `internal/web/templates/queue.html`, `internal/web/queue_test.go`, create `internal/web/queue_page_test.go`
- TDD: no (UI flow test)
- UI flow: Flow 2, Flow 8, Flow 12
- Verify: `go test ./internal/web/ -count=1 -run 'TestQueuePage|TestEmptyQueuePage'`
- Acceptance:
  - `TestQueuePageDoesNotMatchOnReload`: rendering calls `Snapshot` and does not call group matching, troubleshoot, settings, tag update, or queue run
  - The column header is `Assignment`
  - Status `assigned` renders the printer name, `waiting` renders `Waiting for a printer`, `incompatible` renders `No compatible printer`, `merged` renders `Part of a merged plate`, `unreadable` renders `Unreadable job age`
  - A nil snapshot renders `Calculating…` on every job row
  - `TestEmptyQueuePage`: no pending jobs renders `No pending print jobs.`
  - Each visible pending job is its own table row, including copies that share a part, label, and colors. The xN badge is absent
  - While `Enabled` is false, the best-matching printers column is present
- Out of scope: The switch POST

### Task 4.2: Switch control and best-matching column
**Status:** pending
- Files: Modify `internal/web/templates/queue.html`, `internal/web/queue_page_test.go`
- TDD: no (UI flow test)
- UI flow: Flow 1, Flow 3, Flow 9
- Verify: `go test ./internal/web/ -count=1 -run 'TestSmartQueueToggle|TestBestMatching'`
- Acceptance:
  - `#smart-queue-toggle` reads `Smart queue: Off` when the snapshot is disabled and `Smart queue: On` when it is enabled
  - The form posts to `/smart-queue` with hidden `enabled=on` while off, and `enabled=off` while on
  - The confirm text is exactly the Flow 3 question while off, and exactly the Flow 9 question while on
  - `TestBestMatching`: while enabled, the best-matching printers column is absent and the Prioritize button is present
- Out of scope: Enable's Printago sequence, covered by task 3.5

### Task 4.3: Switch POST
**Status:** pending
- Files: Modify `internal/web/server.go`, `internal/web/handlers.go`, `internal/web/queue_page_test.go`
- TDD: no (UI flow test)
- UI flow: Flow 3, Flow 9
- Verify: `go test ./internal/web/ -count=1 -run TestSmartQueuePost`
- Acceptance:
  - `POST /smart-queue` with `enabled=on` calls `Enable` and returns `303` to `/queue`
  - `enabled=off` calls `Disable` and returns `303` to `/queue`
  - `enabled=maybe` returns `303` to `/queue?error=invalid%20smart%20queue%20value` and does not call `Enable` or `Disable`
  - An `Enable` or `Disable` error returns `303` to `/queue?error=` with `Smart queue update failed: ` plus the cause
- Out of scope: Banner rendering of a background poll error

### Task 4.4: Prioritize POST
**Status:** pending
- Files: Modify `internal/web/handlers.go`, `internal/web/queue_page_test.go`
- TDD: no (UI flow test)
- UI flow: Flow 11
- Verify: `go test ./internal/web/ -count=1 -run TestPrioritizePost`
- Acceptance:
  - A valid id calls `Prioritize` and returns `303` to `/queue`
  - The handler does not call `Client.PrioritizePrintJob`. Move-to-front stays inside the scheduler, and only while the switch is off
  - An error from `Prioritize` returns `303` to `/queue?error=` with `Failed to prioritize job: ` plus the cause
  - An invalid id keeps the existing redirect and does not call `Prioritize`
  - Writer of the HTTP action: `handlePrioritizeJob`. Readers: the scheduler prioritize set, and the queue page, which still renders the button while the switch is on and while it is off
- Out of scope: Lane weights, covered by task 3.8

### Task 4.5: Error banner
**Status:** pending
- Files: Modify `internal/web/handlers.go`, `internal/web/templates/queue.html`, `internal/web/queue_page_test.go`
- TDD: no (UI flow test)
- UI flow: Flow 1, Flow 7
- Verify: `go test ./internal/web/ -count=1 -run TestQueueBanner`
- Acceptance:
  - A non-empty plan `Err` is rendered in `.error`
  - A `?error=` query is rendered in `.error`
  - When both are set and the texts differ, both are rendered
  - When the texts are equal, the banner shows that text once
- Out of scope: New banner styles

### Task 4.6: Batch gate
**Status:** pending
- Files: none
- TDD: no
- UI flow: N/A
- Verify: `go test ./... -count=1`
- Acceptance: Every package test exits 0
- Out of scope: New behaviour
