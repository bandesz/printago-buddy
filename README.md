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
  bandesz/printago-buddy:latest
```

## Configuration

The daemon is configured entirely through environment variables:

| Variable | Required | Description |
|---|---|---|
| `PRINTAGO_API_KEY` | yes | Printago API key |
| `PRINTAGO_STORE_ID` | yes | Printago store ID |

The process exits immediately on startup if either variable is missing.

## API key permissions

Create a Printago API key with the following permissions. All three are required — the daemon will fail to function correctly if any are missing.

| Permission | Why it is needed |
|---|---|
| `printer.view` | Read the list of printers and their filament slots |
| `printer.edit` | Write filament tags back to each printer |
| `material.view` | Resolve slot references to material names and variants |

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
  printago/           Printago REST API client and types
  jobs/               cron job implementations
docs/                 Printago OpenAPI spec (reference only)
```

## License

[MIT](LICENSE)
