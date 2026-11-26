# Error handling

The banner is the existing `.error` element. A `?error=` query is shown. A non-empty plan `Err` is shown as well when its text differs from the query.

Cell text and switch position follow `user-flows.md`.

## Rules

| Failure | Switch | Published plan | Printago matching | Retry |
|---|---|---|---|---|
| Poll cannot fetch jobs, printers, slots, or matching | Unchanged | Kept | Unchanged | Next poll |
| `createdAt` cannot be parsed | Unchanged | That job is `unreadable`. Other jobs update | Writes follow the new plan when the switch is on | No |
| `user.tags` is not an `&=` list | Unchanged | Kept | Matching stays as it was. No queue run | Next poll, `applyPending` |
| A reservation PATCH fails | Stays on | Kept, even if earlier jobs in the diff were patched | Stays off | Next poll, `applyPending` |
| Queue run fails after reservations match the plan | Stays on | Published | Stays off | Next poll, `dispatchPending` |
| State file cannot be saved on prioritize | Unchanged | Unchanged | move-to-front is not called | The click fails. No silent retry |
| State file cannot be saved on enable, before the flag is written | Stays off | Unchanged | Unchanged | The click fails |
| Enable fails after the flag is saved | Stays on | Kept until a full apply succeeds | Stays off once step 3 has succeeded. If step 3 did not succeed, matching is whatever the failed PATCH left | Next poll retries turn-on from the settings PATCH |
| Disable fails | Stays on | Kept | Stays off | The click fails. The operator clicks Off again. The poll may refresh pins while the flag stays on |
| Group matching stays `truncated` after a request that names specific job ids | Unchanged | Kept | Unchanged | Next poll |
| Troubleshoot fails for a `tags-match` group | Unchanged | Kept | Unchanged | Next poll |
| Confirm cancelled | Unchanged | Unchanged | Unchanged | No |

`applyPending` means the next poll retries reservation writes even when both signatures match.

`dispatchPending` means the next poll calls queue run even when both signatures match.

A successful apply clears `applyPending`. A successful queue run clears `dispatchPending`. Skipping queue run because no assigned printer is ready also clears `dispatchPending`.

Turn-off does not run half-way into `enabled: false`. The flag flips only after tag cleanup and the settings PATCH both succeed.

An unreadable job is not patched.

A tag expression Buddy does not understand is not rewritten. The pass stops before queue run.
