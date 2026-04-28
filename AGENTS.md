# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

`printago-buddy` is a Go daemon that runs in a Docker container. The module is `github.com/bandesz/printago-buddy` and requires Go 1.26+.

It runs cron-like background jobs against the [Printago API](https://developers.printago.io/). The Printago OpenAPI spec is saved at `docs/printago-api-swagger.json`.

### Current jobs

| Job | Schedule | Description |
|-----|----------|-------------|
| `FilamentTaggerJob` | every minute (`* * * * *`) | Queries all printers and their loaded filaments (AMS slots and external spools), then updates each printer's tags with entries of the form `filament_<snake_case_name>` (e.g. `filament_pla_basic_magenta`). Non-filament tags are preserved. |

## Printago API Calls & Required Permissions

All requests are authenticated with `Authorization: ApiKey <key>` and scoped to a store via `X-Printago-StoreId`. The API key must carry the following permissions:

| Method | Path | Permission | Client method |
|--------|------|------------|---------------|
| `GET` | `/v1/printers` | `printer.view` | `GetPrinters` |
| `GET` | `/v1/printer-slots` | `printer.view` | `GetPrinterSlots` |
| `GET` | `/v1/materials` | `material.view` | `GetMaterials` |
| `GET` | `/v1/materials/variants` | `material.view` | `GetMaterialVariants` |
| `PATCH` | `/v1/printers/{id}` | `printer.edit` | `UpdatePrinterTags` |

## Commands

**Build**
```sh
go build -o bin/printago-buddy ./cmd/printago-buddy
```

**Run** (requires env vars — see Configuration)
```sh
go run ./cmd/printago-buddy
```

**Test**
```sh
go test ./...
```

**Run a single test**
```sh
go test ./path/to/package -run TestFunctionName
```

**Lint** (requires `golangci-lint`)
```sh
golangci-lint run ./...
```

## Configuration

The daemon is configured exclusively via environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `PRINTAGO_API_KEY` | yes | Printago API key (prefixed `ApiKey` internally) |
| `PRINTAGO_STORE_ID` | yes | Printago store ID |

The process exits immediately on startup if either variable is missing.

## Code Structure

- `cmd/printago-buddy/main.go` — entry point; loads config, registers cron jobs, blocks on `SIGINT`/`SIGTERM`, shuts down gracefully
- `internal/config/` — loads and validates environment-variable configuration
- `internal/printago/` — Printago REST API client and type definitions
  - `types.go` — `Printer`, `PrinterSlot`, `Material`, `MaterialVariant`
  - `client.go` — `Client` with methods `GetPrinters`, `GetPrinterSlots`, `GetMaterials`, `GetMaterialVariants`, `UpdatePrinterTags`; also exports `ClientInterface` (the interface satisfied by `Client`, used for dependency injection in jobs)
- `internal/jobs/` — cron job implementations
  - `filament_tagger.go` — `FilamentTaggerJob`
- `docs/printago-api-swagger.json` — Printago OpenAPI 3.1 spec (do not edit manually)
- Build output goes to `bin/printago-buddy`; the `bin/` directory is gitignored
- No vendor directory; dependencies are managed via Go modules (`go.mod`)

## Adding a new job

1. Create `internal/jobs/<name>.go` implementing a `Run()` method.
   - Accept `printago.ClientInterface` (not `*printago.Client`) so the job is straightforward to unit-test with a mock.
2. Register it in `cmd/printago-buddy/main.go` with `c.AddJob(schedule, job)`.
