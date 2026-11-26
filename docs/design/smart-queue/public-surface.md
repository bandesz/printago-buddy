# Public surface

The daemon still has no login. Anyone who can open the port can call these routes, same as Cancel.

## `GET /queue`

Existing page. Compatibility: additive.

Response: HTML.

Always includes:

- Header control `#smart-queue-toggle` labeled `Smart queue: On` or `Smart queue: Off`.
- Column header `Assignment`.
- One assignment cell per visible pending job, using the text in `user-flows.md`.
- Prioritize and Cancel on each job row.

While the cached switch is off, the best-matching printers column is present.

While the cached switch is on, that column is absent.

The handler reads the cached plan. It does not call group matching, troubleshoot, settings, job tag updates, or queue run.

Valid: a browser `GET /queue` after a finished poll shows a printer name or one of the fixed cell strings.

Invalid result: there is no invalid `GET`. A missing plan shows `Calculating…`, not an empty column.

## `POST /smart-queue`

New route. Compatibility: new.

Form body: `enabled=on` or `enabled=off`.

Success: `303` to `/queue`.

`enabled` any other value: `303` to `/queue?error=invalid%20smart%20queue%20value`.

Enable or disable failure: `303` to `/queue?error=` with `Smart queue update failed: ` plus the cause.

The button is a form:

- `method="POST"` `action="/smart-queue"`
- hidden `enabled` is the opposite of the cached switch
- `onsubmit` confirms with the exact question in Flow 3 or Flow 9

Valid: `POST /smart-queue` with `enabled=on` while off saves the on state and redirects to `/queue`.

Invalid: `POST /smart-queue` with `enabled=maybe` redirects to `/queue?error=invalid%20smart%20queue%20value` and does not write the state file or call Printago.

## `POST /prioritize-job`

Existing route. Compatibility: additive.

Form body: `id`, a 24-character job id, unchanged.

Success: `303` to `/queue`.

Unknown or invalid id: the existing invalid-id redirect, unchanged.

Failure: `303` to `/queue?error=` with `Failed to prioritize job: ` plus the cause.

While the switch is off, the first click for a job calls Printago move-to-front and records the mark.

While the switch is on, the click records the mark and does not call move-to-front.

A second click for a job that is already marked does not call move-to-front and does not change the plan.

Valid: `POST /prioritize-job` with a pending managed job id while off records the mark and calls move-to-front once.

Invalid: `POST /prioritize-job` with `id=nope` redirects with the existing invalid-id error and does not record a mark.

## Printago calls made by the daemon

These are not Buddy routes. They are the contract with Printago while the switch is on, and the read-only calls while it is off.

| Call | When |
|---|---|
| `GET /v1/settings/store` | Turn on, before changing settings |
| `PATCH /v1/settings/store` | Turn on, turn off, and resume after restart |
| `PATCH /v1/print-jobs/{id}` with `requiredPrinterTags` | Switch on, when a managed job's reservation differs |
| `POST /v1/print-jobs/queue/group-matching` | A poll or prioritize recomputes the plan |
| `GET /v1/print-jobs/{id}/matching-troubleshoot` | A group's first failure is `tags-match` |
| `POST /v1/print-jobs/queue/run` body `{"manual": true}` | Switch on, after reservations match the plan, when a ready printer has an assigned job or a previous run failed |
| `PATCH /v1/print-jobs/move-to-queue-front` | Prioritize while the switch is off, first click only |

The API key needs `settings.edit` and `job.edit.all` in addition to the permissions Buddy already uses.
