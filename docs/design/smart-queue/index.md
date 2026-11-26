# Smart queue

Buddy chooses the next job for each eligible printer by maximum-weight matching, shows that plan on `/queue`, and, while smart queue is on, makes Printago start those pairs and no others.

`user-flows.md` is the behaviour contract. If another file disagrees with it, this index and `user-flows.md` win, then the other file is updated to match.

## Implementation status
**Status:** in progress — Batch 1 of 4, task 1.1 done

## Read this first

| File | What it decides |
|---|---|
| [user-flows.md](user-flows.md) | What the operator sees, in order |
| [public-surface.md](public-surface.md) | `GET /queue`, `POST /smart-queue`, `POST /prioritize-job` |
| [architecture.md](architecture.md) | Matching, reservations, poll, state file |
| [error-handling.md](error-handling.md) | What stays put when a Printago call fails |
| [testing-strategy.md](testing-strategy.md) | Which test covers which flow |
| [tasks.md](tasks.md) | Batches, files, acceptance, verify commands |

## Problem

Printago assigns the oldest matching job to a free, clear-and-ready printer, then applies routing preferences, then least-recently-used. That greedy pass can give the only suitable printer to a young job and leave an older job with nowhere to go.

Buddy instead weights each pending job by age, `2^(age / half-life)`, matches normal jobs before low-priority jobs, and chooses a maximum-weight pairing. Because every job ages on the same half-life, the pairing does not change merely because the clock moves.

## Decisions

1. **Off by default.** A missing state file means off. Startup does not pause Printago and does not rewrite jobs.
2. **The assignment column is always visible.** It reads an in-memory plan. Reloading `/queue` does not recompute it. The poll recomputes when the graph changes, whether or not the switch is on.
3. **While the switch is on, Buddy is the only router** for pending jobs that are not merged-plate children. The chosen job gets `requiredPrinterTags["printer.id"]`. Every other such job requires the tag `buddy-hold`, which no printer has. Existing nozzle, model, filament, and user tags stay.
4. **Pause, reserve, then run.** Turning on persists `enabled: true`, sets `automaticQueueMatching` to false, sets automatic plate merging off, writes reservations, then `POST /v1/print-jobs/queue/run` with `{"manual": true}`.
5. **Turning off** removes `buddy-hold` and `printer.id` from pending jobs Buddy manages, sets `automaticQueueMatching` to true, restores automatic plate merging to the value saved at turn-on, and persists `enabled: false` only after that cleanup succeeds.
6. **Stopping the process** leaves the switch, Printago matching, and reservations as they are.
7. **Lanes.** Printago priority `100` is normal. `1000` is low. Normal jobs are matched first. A low-priority job is paired only with a printer no normal job can use.
8. **Prioritize** stays on the page in both modes. It marks the job in Buddy's state. Inside its lane, that job outweighs every job that is not prioritized. Among prioritized jobs in that lane, the older one wins. A second click changes nothing. The mark is removed when the job leaves the pending list. With the switch off, the first click also moves the job to the front of the Printago queue.
9. **Best matching printers** stays on the page while the switch is off and is omitted while the switch is on.
10. **A printer that is not marked ready is still a candidate.** Marking it ready does not change the pairs. With the switch on, that change still runs one matcher pass against the cached plan.

## Assumptions

- The park tag text is `buddy-hold`. No printer is given that tag.
- `printer.id` is set to the printer's id.
- `user.tags` is either empty or an AND-list `&=tag1,tag2`. Any other expression fails that pass.
- Half-life defaults to `1h` (`QUEUE_WEIGHT_HALF_LIFE`). Poll defaults to `15s` (`QUEUE_POLL_INTERVAL`). State file defaults to `/data/smart-queue.json` (`SMART_QUEUE_STATE_FILE`).
- Invalid duration configuration fails process startup.
- Docker Compose mounts a volume at `/data`. The image creates `/data` owned by `app`.
- `ready`, `staggered-start-hold`, and `subscription-slots` do not remove an edge.
- `enabled`, `online`, `available`, and `in-maintenance` remove the printer.
- Merged-plate children (`parentJobId` set) are not pinned, parked, or matched. The column says `Part of a merged plate`.
- A job whose `createdAt` cannot be parsed is left out of the matching. The column says `Unreadable job age`.
- Turning off sets `automaticQueueMatching` to true even when it was already false before turn-on.
- Prioritize does not move a job between normal and low.
- There is no confirm step on Prioritize. There is a confirm step on both switch directions.
- The queue page keeps its 10 second reload. That reload only reads the cache.
- A priority other than `1000` is a normal job. `1000` is low.
- The queue page lists one row per visible pending job. Copies that share a part, label, and colors are separate rows, each with its own assignment. The xN collapse is not rendered.
- `Plan.Enabled` matches the state file as soon as that file is saved. Assignment rows publish only after a finished recompute, and, when the switch is on, only after every reservation PATCH for that plan has succeeded.
- Edge weight depends only on the job. All weights are positive.
- Weights inside one stage are divided by that stage's maximum before the solver. The winning set of edges stays the same.

## Non-goals

- Buddy does not build merged plates. While the switch is on, automatic plate merging is off.
- Buddy does not change a job's Printago priority between 100 and 1000.
- Turning off does not restore a `printer.id` that was on the job before Buddy replaced it.
- The plan is the next job per printer, not a sequence of later jobs.
- Partial filament matches are not edges. Printago's checks are the oracle.
- The process does not turn Printago matching back on when it exits.
- The most recently prioritized job does not outrank an older prioritized job in the same lane.
- The daemon does not gain login.

## Batches

1. **Matching core** — weight, two-stage assignment, tag edits, edges, change signature. No Printago calls.
2. **Printago client** — list every pending job, store settings, job tags, group matching, troubleshoot, manual queue run.
3. **Scheduler** — state file, on/off, poll, cache, prioritize, process wiring.
4. **Queue page** — assignment column, switch, prioritize, error banner.

Task acceptance and verify commands are in [tasks.md](tasks.md).
