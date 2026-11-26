# Testing strategy

Buddy is a Go daemon with server-rendered HTML. There is no browser test harness in the repo. Each flow is locked by a Go test that asserts the observable result: the cell text, the redirect, or the Printago calls a fake client recorded.

A later implementation pass that can reach a running daemon also loads `/queue` and uses the switch and Prioritize. That pass is in addition to the Go tests, not a substitute for them.

| Flow | Test |
|---|---|
| Flow 1 | `TestStartupMissingStateFileDoesNotCallPrintago` |
| Flow 2 | `TestQueuePageDoesNotMatchOnReload` |
| Flow 3 | `TestEnableSmartQueue`, `TestEnableConfirmCopy` |
| Flow 4 | `TestReadyChangeRunsQueueWithoutRecompute` |
| Flow 5 | `TestNewJobChangesAssignment` |
| Flow 6 | `TestIncompatibleJobIsParkedWhenEnabled` |
| Flow 7 | `TestApplyFailureKeepsPublishedPlan` |
| Flow 8 | `TestEmptyQueuePage` |
| Flow 9 | `TestDisableSmartQueue` |
| Flow 10 | `TestRestartResumesEnabled`, `TestShutdownDoesNotDisable` |
| Flow 11 | `TestPrioritizeBeatsOlderNormalJob`, `TestPrioritizeLowDoesNotBeatNormal`, `TestPrioritizeWhileOffMovesToFrontOnce` |
| Flow 12 | `TestSkippedRowsAreNotPatched` |

Matching tests also include `TestHeavyJobDoesNotTakeTheOnlyPrinterOfALighterJob`.

Domain skills: none. This repo is not Phoenix or Ash. UI behaviour is asserted with `net/http/httptest` against the queue template and the two POST handlers.

Package checks used by the tasks:

- `go test ./internal/smartqueue/ -count=1`
- `go test ./internal/printago/ -count=1`
- `go test ./internal/config/ -count=1`
- `go test ./internal/web/ -count=1`
- `go test ./... -count=1`

`go test ./... -count=1` is the batch gate after batch 4. Earlier tasks name the package test that covers that task.
