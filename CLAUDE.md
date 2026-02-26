# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Handoff is a Go server that enables backend applications to collect photos and signatures from phone users via session URLs. Backend apps create sessions via API, users complete actions on a phone-friendly web UI, and results are retrieved via polling or WebSocket.

## Build & Run Commands

```bash
go build -o server ./cmd/server          # build
go vet ./...                              # lint / static analysis
go test -race ./...                       # run all tests
just test                                 # same via justfile
just air                                  # hot-reload dev server (requires air)
just generate                             # run go generate
```

Build with version info (matches Dockerfile):
```bash
go build -ldflags "-X github.com/mxcd/handoff/internal/util.Version=v1.0.0 -X github.com/mxcd/handoff/internal/util.Commit=$(git rev-parse --short HEAD)" -o server ./cmd/server
```

Required env vars to start: `API_KEYS` (comma-separated), `BASE_URL`. Optional: `DEV=true`, `PORT` (default 8080), `LOG_LEVEL`, `SESSION_TTL`, `RESULT_TTL`.

## Architecture

- **`cmd/server/main.go`** — Entrypoint. Initializes config, logger, server, then handles graceful shutdown on SIGINT/SIGTERM.
- **`internal/server/`** — Gin HTTP server. `server.go` defines `Server` struct, route registration, and the `/api/v1` base path. Controllers are `*_controller.go` files (methods on `Server`). `middleware.go` has `apiKeyAuth()` which validates `X-API-Key` header against configured keys. `util.go` has the `jsonError` helper.
- **`internal/util/`** — Config loading via `mxcd/go-config` (env vars + `.env` files), zerolog logger setup, and build-time version/commit vars.
- **`internal/web/`** — Embedded static assets via `//go:embed all:html`. Files in `internal/web/html/public/` are served at `/static`.

## Key Patterns

- **Config**: All config values are declared in `internal/util/config.go` using `mxcd/go-config`. Access via `config.Get().String("KEY")`, `config.Get().Bool("KEY")`, etc.
- **Routes**: Public routes (`/health`, `/version`) are registered directly on the engine. Protected routes go on `s.ProtectedAPI` router group which has `apiKeyAuth()` middleware.
- **Logging**: Use `github.com/rs/zerolog/log` throughout. DEV mode enables colored console output.
- **Error responses**: Use `jsonError(c, statusCode, "message")` for consistent JSON error format `{"error": "..."}`.
- **Module path**: `github.com/mxcd/handoff` — used in ldflags and imports.

## Git Conventions

- Use [Conventional Commits](https://www.conventionalcommits.org/) format: `type(scope): description` (e.g. `feat(server): add session endpoints`, `fix(auth): handle empty API key list`).
- Do NOT add `Co-Authored-By` lines or any Anthropic/Claude attribution to commit messages.
