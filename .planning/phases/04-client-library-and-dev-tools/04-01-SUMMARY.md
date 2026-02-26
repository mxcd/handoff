---
phase: 04-client-library-and-dev-tools
plan: 01
subsystem: api

tags: [go, client-library, websocket, qrcode, http-client, builder-pattern]

requires:
  - phase: 02-session-core
    provides: POST /api/v1/sessions, GET /api/v1/sessions/:id/result, WebSocket /api/v1/sessions/:id/ws, GET /api/v1/downloads/:id

provides:
  - Go client library at pkg/handoff/ importable as github.com/mxcd/handoff/pkg/handoff
  - NewClient(baseURL, apiKey) constructor with 30s timeout HTTP client
  - SessionBuilder fluent API: NewSession().WithAction().WithOutputFormat().Invoke(ctx)
  - Session.GenerateQR() returning base64 PNG data URL
  - Session.OnEvent() for thread-safe event callback registration
  - Session.WaitForResult() blocking until completion
  - Session.Close() for graceful shutdown
  - WebSocket subscription with automatic reconnect (3 attempts) and polling fallback
  - Custom error types with errors.Is() support for 401/404/409/410 responses

affects:
  - 04-02-dev-playground (will use this client to demonstrate usage)
  - consumers of pkg/handoff package

tech-stack:
  added:
    - github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
  patterns:
    - Builder pattern for session creation (SessionBuilder)
    - Sentinel error wrapping via Unwrap() for errors.Is() compatibility
    - WebSocket-first with polling fallback for event subscription
    - Exponential backoff retry (500ms, 1s, 2s) for transient HTTP errors

key-files:
  created:
    - pkg/handoff/types.go
    - pkg/handoff/errors.go
    - pkg/handoff/client.go
    - pkg/handoff/session.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "pkg/handoff types are re-declared independently — no import from internal/ to keep pkg/ importable externally"
  - "doRequest retries up to 3 times on 5xx and network errors; 4xx responses are not retried"
  - "WebSocket reconnects 3 times with backoff (1s, 2s, 4s) then falls back to 2s polling loop"
  - "resultCh is a buffered channel of size 1 — multiple WaitForResult callers only get one result"
  - "sessionResponse.SessionTTL and ResultTTL are int64 nanoseconds to match Go's time.Duration JSON marshaling"

patterns-established:
  - "Builder pattern: client.NewSession().WithX().Invoke(ctx) — consistent with pkg/ library ergonomics"
  - "Error wrapping: APIError.Unwrap() returns sentinel — callers use errors.Is(err, handoff.ErrNotFound)"

requirements-completed:
  - INFR-05

duration: 2min
completed: 2026-02-26
---

# Phase 4 Plan 1: Go Client Library Summary

**Typed Go client library in pkg/handoff with builder-pattern session creation, WebSocket event subscription with polling fallback, QR code generation, and errors.Is()-compatible error types**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-26T22:39:48Z
- **Completed:** 2026-02-26T22:41:52Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Created complete Go client library at pkg/handoff/ importable as github.com/mxcd/handoff/pkg/handoff
- Implemented SessionBuilder fluent API: NewSession().WithAction().WithOutputFormat().Invoke(ctx)
- Session events delivered via WebSocket with automatic reconnect and polling fallback
- QR code generation, WaitForResult blocking helper, and graceful Close()
- Custom error types (ErrSessionExpired, ErrNotFound, etc.) compatible with errors.Is()

## Task Commits

Each task was committed atomically:

1. **Task 1: Create types and error definitions** - `5a05c51` (feat)
2. **Task 2: Implement client with builder pattern and session with WebSocket subscription** - `5bf7663` (feat)

## Files Created/Modified
- `pkg/handoff/types.go` - Public types: ActionType, OutputFormat, SessionStatus, Event, ResultItem, CreateSessionRequest; unexported sessionResponse, resultPollResponse
- `pkg/handoff/errors.go` - Sentinel errors and APIError with Unwrap() for errors.Is() support
- `pkg/handoff/client.go` - Client struct, NewClient constructor, doRequest with retry, SessionBuilder builder pattern, GetSession, DownloadFile
- `pkg/handoff/session.go` - Session struct with GenerateQR, OnEvent, WaitForResult, Close, WebSocket goroutine, polling fallback
- `go.mod` - Added github.com/skip2/go-qrcode dependency
- `go.sum` - Updated checksums

## Decisions Made
- Re-declared types in pkg/handoff independently rather than importing from internal/ — keeps pkg/ usable as a standalone dependency
- doRequest retries only on 5xx and network errors; 4xx responses fail immediately
- WebSocket attempts 3 reconnects (1s/2s/4s backoff) then falls back to 2-second polling
- resultCh buffered size 1 — WaitForResult delivers to the first caller only
- SessionTTL/ResultTTL stored as int64 nanoseconds in sessionResponse to match Go's time.Duration JSON marshaling behavior

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- pkg/handoff client library is complete and compiles cleanly
- Ready for 04-02 dev playground that demonstrates the client
- External Go apps can now import github.com/mxcd/handoff/pkg/handoff

---
*Phase: 04-client-library-and-dev-tools*
*Completed: 2026-02-26*

## Self-Check: PASSED

- FOUND: pkg/handoff/types.go
- FOUND: pkg/handoff/errors.go
- FOUND: pkg/handoff/client.go
- FOUND: pkg/handoff/session.go
- FOUND: commit 5a05c51
- FOUND: commit 5bf7663
