# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

`printago-buddy` is a Go CLI application. The module is `github.com/bandesz/printago-buddy` and requires Go 1.26+. Currently the project is in early/stub stage with a single entry point.

## Commands

**Build**
```sh
go build -o bin/printago-buddy ./cmd/printago-buddy
```

**Run**
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

## Code Structure

- `cmd/printago-buddy/main.go` — entry point; the binary is named `printago-buddy`
- Build output goes to `bin/printago-buddy`; the `bin/` directory is gitignored
- No vendor directory; dependencies are managed via Go modules (`go.mod`)
