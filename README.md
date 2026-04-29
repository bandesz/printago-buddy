# printago-buddy

A lightweight Go daemon that runs background jobs against the [Printago](https://printago.io) API.
It is designed to run as a Docker container alongside your existing Printago setup.

## What it does

### FilamentTagger job

Runs every minute. For every printer in your store it:

1. Reads all filament slots (AMS slots and external spools).
2. Resolves each slot to a material and, where available, a specific variant.
3. Writes tags of the form `filament_<snake_case_name>` to the printer (e.g. `filament_pla_basic_magenta`).

Any tags that do **not** start with `filament_` are preserved unchanged, so your own custom tags are never removed.

This lets you search and filter printers in Printago by their currently loaded filament — for example, to find all printers that have a specific colour or material type loaded.

### Print Queue UI

A lightweight web server runs alongside the cron jobs and exposes a read/write queue management interface at `http://localhost:8889` (port is configurable via `WEB_PORT`).

- **Queue view** — lists all pending print jobs grouped into normal-priority and low-priority sections, each with a thumbnail preview and the filament colours required by the job.
- **Printer matching** — for every queued job the UI shows up to three best-matching printers, ranked by how many of the job's filament requirements they satisfy.
- **Cancel** — removes a job from the queue.
- **Prioritise** — moves a job to the front of the queue.
- **Auto-refresh** — the page refreshes itself every 10 seconds and displays a last-refreshed timestamp.

## Requirements

- A [Printago](https://printago.io) account with an API key (see [API key permissions](#api-key-permissions) below)
- Docker (recommended) **or** Go 1.26+

## Quick start with Docker Compose

1. Copy the example environment file and fill in your credentials:

   ```sh
   cp .env.example .env
   # edit .env
   ```

   `.env` contents:

   ```
   PRINTAGO_API_KEY=your_api_key_here
   PRINTAGO_STORE_ID=your_store_id_here
   ```

2. Start the daemon:

   ```sh
   docker compose up -d
   ```

The container will restart automatically unless explicitly stopped.

## Quick start with Docker

```sh
docker run -d --restart unless-stopped \
  -e PRINTAGO_API_KEY=your_api_key_here \
  -e PRINTAGO_STORE_ID=your_store_id_here \
  -p 8889:8889 \
  bandesz/printago-buddy:latest
```

## Configuration

The daemon is configured entirely through environment variables:

| Variable | Required | Description |
|---|---|---|
| `PRINTAGO_API_KEY` | yes | Printago API key |
| `PRINTAGO_STORE_ID` | yes | Printago store ID |
| `WEB_PORT` | no | Port for the web UI (default: `8889`) |

The process exits immediately on startup if either required variable is missing.

## API key permissions

Create a Printago API key with the following permissions. All are required — the daemon will fail to function correctly if any are missing.

| Permission | Why it is needed |
|---|---|
| `printer.view` | Read the list of printers and their filament slots |
| `printer.edit` | Write filament tags back to each printer |
| `material.view` | Resolve slot references to material names and variants |
| `queue.view` | Fetch the pending print job queue for the web UI |
| `part.view` | Read per-job filament requirements for printer matching |
| `queue.manage` | Cancel jobs from the web UI |
| `queue.override` | Move a job to the front of the queue |

The key is passed via the `PRINTAGO_API_KEY` environment variable (see [Configuration](#configuration)).

## Development

**Prerequisites:** Go 1.26+, [`golangci-lint`](https://golangci-lint.run/) (for linting)

```sh
# Build
make build          # output: bin/printago-buddy

# Run locally (requires env vars set in your shell)
go run ./cmd/printago-buddy

# Test
make test

# Lint
make lint
```

## Releasing

```sh
make release VERSION=1.2.3
```

This will:
1. Create and push a `v1.2.3` git tag.
2. Build Docker images tagged `1.2.3` and `latest`.
3. Push both images to Docker Hub.

## Project structure

```
cmd/printago-buddy/   entry point
internal/
  config/             environment variable loading
  printago/           Printago REST API client, types, and caching layer
  jobs/               cron job implementations
  web/                HTTP server and print queue UI
docs/                 Printago OpenAPI spec (reference only)
```

## License

[MIT](LICENSE)
