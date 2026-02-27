# Handoff

## What This Is

Handoff is a standalone utility server that enables web applications to delegate phone-specific tasks (document scanning, photo capture, e-signatures) to a mobile device via QR code. A calling backend creates a session via API, receives a URL to encode as a QR code, and the phone user completes the action on Handoff's server-rendered UI. Results flow back to the caller via polling or WebSocket.

## Core Value

A calling backend can create a session, generate a QR code URL, and receive the completed action result (scan, photo, signature) — reliably and without requiring the calling app to build any mobile-facing UI.

## Requirements

### Validated

<!-- Shipped in v1.0 and confirmed working -->

- ✓ Calling backends authenticate via API keys (multiple keys, each identifying a caller) — v1.0
- ✓ Backend creates a session specifying: action type, optional intro text, result format, TTLs — v1.0
- ✓ Session creation returns a unique URL suitable for QR code encoding — v1.0
- ✓ Phone user scans QR, sees optional intro/explanation page, then performs the action — v1.0
- ✓ Action: take a generic photo via phone camera — v1.0
- ✓ Action: sign a document using full-screen touch signature field (undo/redo support) — v1.0
- ✓ Results transmitted from phone UI to Handoff backend via HTTP form submissions — v1.0
- ✓ Caller retrieves results via API polling or real-time WebSocket notification — v1.0
- ✓ Session validity defaults to 30 minutes (configurable per session) — v1.0
- ✓ Result availability defaults to 1 minute after completion (configurable) — v1.0
- ✓ Result format is configurable by the caller at session creation — v1.0
- ✓ Server-rendered UI using Go HTML templates with plain HTML/CSS/JavaScript — v1.0
- ✓ Static assets embedded in Go binary via `embed.FS` — v1.0
- ✓ Go client library (`pkg/`) for easy integration from other Go applications — v1.0
- ✓ Health and version API endpoints — v1.0
- ✓ Graceful shutdown on SIGINT/SIGTERM — v1.0

### Active

- [ ] Action: document scanning with manual crop and perspective correction
- [ ] Caller specifies single-document or multi-document mode at session creation
- [ ] Phone user chooses single-page or multi-page capture mode
- [ ] Crop UI with 4 corner handles positioned outside the crop area
- [ ] Offset magnifying glass shows exact corner position under user's finger
- [ ] Client-side perspective warp via Canvas before upload
- [ ] Preview of cropped result with ability to re-crop
- [ ] Multi-document sessions: "Next Document" button separates document boundaries
- [ ] Results structured as nested array of documents, each with array of pages
- [ ] Output format: multi-page PDF per document or individual page images (caller's choice)

### Out of Scope

- Database persistence — sessions use in-memory cache (`mxcd/go-cache`)
- Vue.js embeddable component — deferred to future release
- OAuth/OIDC authentication — API key auth only for v1
- Mobile native app — web-only, phone accesses via browser
- Real-time chat or streaming — simple request/response with WebSocket for status updates
- Auto edge detection for document scanning — deferred to future milestone, manual crop first

## Context

- **Existing scaffold:** Project is bootstrapped from a previous project ("defector"). The directory structure (`cmd/server/`, `internal/server/`, `internal/util/`, `internal/web/`) and core patterns should be preserved. Old imports referencing `defector`, `repository`, `storage`, `inference`, etc. need to be replaced with Handoff-specific code.
- **MXCD libraries to keep:**
  - `github.com/mxcd/go-config` — environment/config loading with typed values, defaults, and sensitive masking
  - `github.com/mxcd/go-cache` — synchronized in-memory cache for session state (not yet in go.mod, needs adding)
  - `github.com/rs/zerolog` — structured logging
- **Web framework:** Gin (`github.com/gin-gonic/gin`) with CORS support, gzip compression, embedded static file serving
- **Signature library:** User has a specific JS library in mind — will provide later
- **Build tooling:** `justfile` for common tasks, `air` for hot reload during development
- **Deployment:** Single Go binary + Docker image (user has Docker best-practice templates)

## Constraints

- **Tech stack**: Go with Gin, no database, no external JS frameworks — plain HTML/CSS/JS with Go HTML templates
- **Statelessness**: No persistent storage; `mxcd/go-cache` for in-memory session state only
- **Existing patterns**: Must preserve the project structure and MXCD library usage from the scaffold
- **Extensibility**: Action type system should be designed so new action types can be added later without major refactoring

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| In-memory cache via `mxcd/go-cache` instead of database | Stateless design, sessions are ephemeral with short TTLs | — Pending |
| API key auth (not OAuth/OIDC) | Simplicity for machine-to-machine integration | — Pending |
| Server-rendered HTML (not SPA) | Minimal JS, no build step for frontend, embedded in binary | — Pending |
| Go client library in `pkg/` | First-class Go integration alongside HTTP API | — Pending |
| Result delivery via polling + WebSocket | Caller chooses push or pull; no webhook complexity for v1 | — Pending |

## Current Milestone: v1.1 Document Scanning

**Goal:** Add document scanning action type with manual crop, perspective correction, multi-page and multi-document support.

**Target features:**
- Document scanning with manual 4-corner crop and client-side perspective warp
- Single-page and multi-page capture modes (user's choice)
- Single-document and multi-document modes (caller specifies)
- Touch-friendly crop handles with offset magnifying glass for precision
- Nested result structure (documents → pages) with PDF or image output

---
*Last updated: 2026-02-27 after v1.1 milestone start*
