# Architecture

## Components

| Piece | Role |
|---|---|
| `internal/smartqueue` | Weight, matching, tag edits, edges, signature, state file, scheduler |
| `internal/printago` | HTTP for settings, job tags, group matching, troubleshoot, queue run, paginated pending jobs |
| `internal/web` | Renders the cache. Posts the switch and Prioritize into the scheduler |
| `internal/config` | Half-life, poll interval, state file path |
| `cmd/printago-buddy` | Starts the poll. Shutdown does not turn smart queue off |

The web handler depends on this interface, not on the poll loop:

```text
Snapshot() Plan
Enable(ctx) error
Disable(ctx) error
Prioritize(ctx, jobID) error
```

Enable, disable, prioritize, and poll hold the same mutex.

## Plan cache

```text
Plan
  Enabled bool
  Err string
  Rows by job id
    PrinterID
    PrinterName
    Status: assigned | waiting | incompatible | merged | unreadable
```

`Err` empty means the last poll succeeded. The column maps status to the cell text in `user-flows.md`. A nil plan means `Calculating…`.

`Plan.Enabled` follows the state file as soon as that file is saved. After enable saves `enabled: true`, `Snapshot` reports enabled even when a later settings PATCH, reservation write, or queue run fails. After disable saves `enabled: false`, `Snapshot` reports disabled. A failed disable does not save that flag, so `Snapshot` stays enabled.

The published rows change only when a recompute finishes and, if the switch is on, every reservation PATCH for that plan has succeeded. A failed queue run still publishes the rows, because the tags already match them, and records `Err`.

## Weight

`age` is `now - createdAt`, floored at zero.

`weight = 2^(age / halfLife)`.

Lanes, in order:

1. Normal prioritized jobs.
2. Normal jobs that are not prioritized.
3. Low prioritized jobs.
4. Low jobs that are not prioritized.

Each stage is a maximum-weight bipartite matching on the printers not chosen by an earlier stage. Edge weight is the job weight. Vertices may stay unmatched. Greedy heaviest-first is not used.

Counterexample the tests lock: job H can use printers A and B, job L can use only A, and H weighs more than L. The result gives A to L and B to H.

A common positive scale factor may be applied inside a stage. It must be the same for every edge in that stage.

## Who is in the graph

Pending jobs with an empty `parentJobId` and a parseable `createdAt` are job vertices.

A printer is eligible when group matching reports it and its first failed check is not `enabled`, `online`, `available`, or `in-maintenance`. A printer missing from the verdict is not eligible.

An edge exists when the printer is eligible and any of these is true:

- `matched` is true
- `firstFailedCheck` is empty, `ready`, `staggered-start-hold`, or `subscription-slots`
- `firstFailedCheck` is `tags-match`, and troubleshoot shows every other check passing except `ready`, `staggered-start-hold`, and `subscription-slots`, and every failing tag comparison is a reservation comparison

`compatible` true does not create an edge by itself. `profiles-configured`, `slots-loaded`, `routing-rules`, `material-match`, `filament-profiles`, and `bed-fits` make the printer eligible and create no edge, even when `compatible` is true.

A troubleshoot check has `code`, `passed`, and `comparisons`. Each comparison has `key`, `requirement`, and `matched`. A failing comparison is a reservation comparison when `key` is `printer.id` or `buddy-hold`. Any other failing comparison blocks the edge.

Group matching is called with `use: "direct"` so routing preferences are not edges. Routing requirements still appear as `routing-rules`.

The decoded group-matching value has `truncated` and a list of groups. Each group has `memberPrintJobIds`, `representativePrintJobId`, and one verdict per printer with `printerId`, `matched`, `compatible`, and `firstFailedCheck`. `firstFailedCheck` is empty when that printer did not fail a check.

One troubleshoot call is made per group whose representative has any `tags-match` failure. Every member of the group gets those edges. Each member keeps its own weight, lane, and prioritize mark.

If `truncated` is true, the scheduler requests again with `printJobIds` for the job ids that did not appear. A truncated response to a request that already names specific ids fails the pass.

## Reservations

Managed pending jobs, while the switch is on:

- Assigned: `requiredPrinterTags["printer.id"]` is that printer id, and `user.tags` does not contain `buddy-hold`.
- Not assigned: `printer.id` is absent, and `user.tags` requires `buddy-hold`.

`PATCH` sends the whole `requiredPrinterTags` object. Other keys are copied. `printer.id` is omitted to clear it. `user.tags` is omitted when the AND-list would be empty.

`&=a,buddy-hold` removes to `&=a`. `&=buddy-hold` removes `user.tags`. `&=a,b` gains `&=a,b,buddy-hold`. Empty gains `&=buddy-hold`. Repeating an edit that is already done does not PATCH.

A `user.tags` value that is not empty and does not start with `&=` fails the pass. The job is not given a rewritten expression.

Merged-plate children are never patched.

While the switch is off, the poll does not PATCH jobs or settings.

## When the plan is recomputed

The poll waits `QUEUE_POLL_INTERVAL`, default `15s`.

The assignment signature covers managed and child job ids, priority, `createdAt`, required tags with `printer.id` and `buddy-hold` removed, the prioritize set, printer ids, printer tags, slot material and variant ids, and the eligibility checks other than `ready`. It ignores Buddy's own `printer.id` and `buddy-hold`.

The dispatch signature is the assignment signature plus each eligible printer's ready bit.

- Assignment signature unchanged and dispatch signature unchanged: do no Printago work, except a retry flagged below.
- Only the ready bits changed, switch on: do not recompute pairs. Call queue run when the cached plan assigns a job to a printer that is now ready.
- Assignment signature changed: recompute. Publish immediately when the switch is off. When it is on, PATCH the diff, then publish, then queue run if any assigned printer is ready.
- Prioritize and the switch run the same recompute immediately. They do not wait for the next tick.

Prioritize ids that are not in the latest visible pending list are deleted from the state file before matching.

## Switch sequences

Turn on:

1. Read store settings and remember `plateMerging.automaticMerging`. Absent means false.
2. Persist `enabled: true` and that merging value.
3. `PATCH` `automaticQueueMatching` false and `plateMerging.automaticMerging` false.
4. Recompute, write reservations, publish, queue run when a ready printer has a job.

Turn off:

1. Remove `buddy-hold` and `printer.id` from managed pending jobs.
2. `PATCH` `automaticQueueMatching` true and restore the saved merging value.
3. Persist `enabled: false` and drop the merging snapshot. Prioritize ids stay.

Restart with `enabled: true` repeats the turn-on settings patch, reservation diff, and queue run. Restart with a missing file, or `enabled: false`, does not PATCH settings or jobs.

SIGINT and SIGTERM stop the poll and do not run turn-off.

## State file

Path `SMART_QUEUE_STATE_FILE`, default `/data/smart-queue.json`.

```json
{
  "enabled": false,
  "hasMergingSnapshot": false,
  "automaticMerging": false,
  "prioritizedJobIds": []
}
```

`hasMergingSnapshot` is true only while the switch is on. `automaticMerging` is the value read at turn-on.

## Queue run

`POST /v1/print-jobs/queue/run` with body `{"manual": true}`.

Skip the call when the switch is off, when no assigned printer is ready, and when no retry is pending.

## Wiring

`GetPrintJobs` keeps `status=pending`, `hidden=false`, `sort=queueOrder`, and `include=partBuild`. It follows `offset` with `limit=200` until a page is shorter than 200.

`PrintJob` gains `ParentJobID` and `GroupID`.

New client methods are on `ClientInterface`, forwarded uncached by `CachingClient`, and added to the existing test doubles in the same change that adds them.

`main` constructs the scheduler, starts its loop, and passes it to the web server. The filament tagger cron stays as it is.
