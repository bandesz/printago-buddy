# User flows

This file is the behaviour contract for smart queue.

A pending job Buddy manages is a pending job with no `parentJobId`. Merged-plate children are shown and otherwise left alone.

Assignment cell text is one of:

- the printer's name
- `Waiting for a printer`
- `No compatible printer`
- `Part of a merged plate`
- `Unreadable job age`
- `Calculating…`

## Flow 1: Buddy starts with smart queue off
**Actor:** operator
**Precondition:** No state file. Printago automatic matching is on. Pending jobs exist.
**Steps:**
1. Start Buddy → Printago automatic matching stays on. No job tags are changed.
2. Open `/queue` → the header control reads `Smart queue: Off`. The best-matching printers column is present. The Prioritize button is present. The assignment column reads `Calculating…` until the first poll finishes.
3. After the first poll → each managed job shows a printer name, `Waiting for a printer`, or `No compatible printer`. Printago has not been patched.

**Error paths:**
- The first poll cannot read Printago → the assignment cells stay `Calculating…` and the page banner shows the poll error. Printago matching stays on.

## Flow 2: Assignment stays cached across reloads
**Actor:** operator
**Precondition:** A plan is cached. The switch may be on or off.
**Steps:**
1. Reload `/queue`, including the page's 10 second reload → the assignment column shows the cached plan.
2. The reload does not call group matching, troubleshoot, job tag updates, or queue run.

## Flow 3: Turn smart queue on
**Actor:** operator
**Precondition:** The switch is off. `/queue` shows best-matching printers and Prioritize.
**Steps:**
1. Click `Smart queue: Off` → the browser asks `Turn on smart queue? Printago will stop automatic matching, and Buddy will choose which job each printer runs.`
2. Confirm → Buddy saves the switch as on, sets `automaticQueueMatching` to false, and turns automatic plate merging off.
3. `/queue` reloads. The control reads `Smart queue: On`. Best-matching printers is gone. Prioritize remains. Each managed job shows its assignment. Each ready printer starts the job assigned to it. A printer that is on but not cleared does not start.

**Error paths:**
- The operator cancels the confirm → the switch stays off and Printago is unchanged.
- Enable fails → the control stays `Smart queue: Off` only when the state file was not saved. Once the state file says on, the control stays `Smart queue: On`, the banner shows `Smart queue update failed: ` plus the cause, and a later poll retries pause, reserve, and run.

## Flow 4: A ready printer starts the assigned job
**Actor:** operator
**Precondition:** The switch is on. A printer is eligible, not marked ready, and has a job assigned to it.
**Steps:**
1. The assignment column already shows that printer's name on that job.
2. Mark the printer Clear & Ready in Printago.
3. The next poll starts that job. The pairs are not recomputed when readiness is the only change.
4. `/queue` drops the job once Printago no longer lists it as a visible pending job. The printer's next managed job shows a name, or no managed job is assigned to it.

## Flow 5: A new job arrives
**Actor:** operator
**Precondition:** A plan is already cached.
**Steps:**
1. Queue a job in Printago.
2. After the next poll, `/queue` shows the new job's assignment.
3. When it takes a printer from an older job, that older row changes to `Waiting for a printer`.
4. With the switch on, reservations match the new plan before the matcher runs. A ready printer starts the new job only when that job is the one assigned to it.
5. With the switch off, Printago tags are not written. The column still updates.

## Flow 6: Nothing can print the job
**Actor:** operator
**Precondition:** A managed pending job has no edge to an eligible printer.
**Steps:**
1. `/queue` shows `No compatible printer` on that row.
2. With the switch on, the job requires `buddy-hold` and has no `printer.id`. No printer starts it.

## Flow 7: Buddy cannot finish an update
**Actor:** operator
**Precondition:** The switch is on and a tag write or the manual matcher run fails.
**Steps:**
1. `/queue` shows the error banner with the failure.
2. Automatic matching stays off.
3. The assignment column keeps the last plan whose reservations were fully written.
4. The next poll retries the unfinished tag writes or the unfinished matcher run.

## Flow 8: Empty queue
**Actor:** operator
**Precondition:** No visible pending jobs.
**Steps:**
1. `/queue` shows `No pending print jobs.`
2. With the switch on, automatic matching stays off and there is nothing to reserve.

## Flow 9: Turn smart queue off
**Actor:** operator
**Precondition:** The switch is on. Some managed jobs are pinned or parked.
**Steps:**
1. Click `Smart queue: On` → the browser asks `Turn off smart queue? This removes printer assignments and park tags and turns Printago automatic matching back on.`
2. Confirm → Buddy removes `buddy-hold` and `printer.id` from managed pending jobs, sets `automaticQueueMatching` to true, and restores automatic plate merging.
3. `/queue` reloads. The control reads `Smart queue: Off`. Best-matching printers and Prioritize are shown. The assignment column still shows the cached plan. Printago assigns with its own matcher.

**Error paths:**
- The operator cancels the confirm → the switch stays on and reservations stay.
- Cleanup fails → the control stays `Smart queue: On`, the banner shows `Smart queue update failed: ` plus the cause, and automatic matching stays off.

## Flow 10: Restart Buddy
**Actor:** operator
**Precondition:** The state file exists.
**Steps:**
1. Stop Buddy with SIGINT or SIGTERM and start it again. The state file is still there.
2. The control shows the saved choice. On resumes pause, reserve, and run. Off leaves Printago alone.
3. A new container with no state file follows Flow 1.
4. Prioritize marks that still refer to visible pending jobs are kept.

## Flow 11: Prioritize a job
**Actor:** operator
**Precondition:** `/queue` lists a managed pending job that is not yet prioritized.
**Steps:**
1. Click Prioritize. There is no confirm step.
2. The assignment cache recomputes immediately. In that job's lane, it now outweighs every job that is not prioritized. An older prioritized job in the same lane still outweighs it. A low-priority job still waits while any normal job can use the printer.
3. With the switch on, reservations are rewritten and one matcher pass runs for ready printers.
4. With the switch off, Buddy also moves the job to the front of the Printago queue, and does not write pins or park tags.
5. Click Prioritize on that same job again → the plan is unchanged, and Printago move-to-front is not called again.

**Error paths:**
- The state file cannot be saved, or, with the switch off, move-to-front fails → `/queue` redirects to `/queue?error=` with `Failed to prioritize job: ` plus the cause. The job is not marked prioritized.
- With the switch on, rewriting reservations fails → the mark is saved, the banner shows the poll error, and the column keeps the last fully written plan until a retry succeeds.

## Flow 12: Rows Buddy does not match
**Actor:** operator
**Precondition:** The pending list contains a merged-plate child and a managed job whose `createdAt` is not a timestamp.
**Steps:**
1. Open `/queue` after a poll → the child row says `Part of a merged plate`. The bad timestamp row says `Unreadable job age`.
2. With the switch on, neither job is patched.
