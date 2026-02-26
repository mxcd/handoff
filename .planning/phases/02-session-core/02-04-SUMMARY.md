---
phase: 02-session-core
plan: "04"
subsystem: api
tags: [websocket, gorilla-websocket, real-time, notifications, hub]

# Dependency graph
requires:
  - phase: 02-session-core/02-03
    provides: result submission and polling endpoints; MarkSessionCompleted in store

provides:
  - WebSocket hub (internal/ws/hub.go) managing per-session subscriber lists
  - WebSocket upgrade endpoint GET /api/v1/sessions/:id/ws with inline API key auth
  - BroadcastCompletion called from submitResultHandler on session completion
  - Hub wired into Server struct and initialised in NewServer

affects:
  - 03-phone-ui (status update broadcasts for opened/action_started will be added here)
  - 04-client-library (WS connection logic for Go client)

# Tech tracking
tech-stack:
  added:
    - gorilla/websocket v1.5.3
  patterns:
    - WebSocket auth via inline key validation before upgrade (header or query param)
    - Read-loop-as-disconnect-detector pattern in wsHandler goroutine
    - Hub Broadcast acquires write lock once for all subscribers; failed writers removed atomically

key-files:
  created:
    - internal/ws/hub.go
    - internal/server/ws_controller.go
  modified:
    - internal/server/server.go
    - internal/server/result_controller.go
    - go.mod
    - go.sum

key-decisions:
  - "WS endpoint on Engine directly (not ProtectedAPI) — API key validated inline before upgrade to support query param auth"
  - "Initial status_update sent immediately after successful upgrade so subscriber knows current state without race"
  - "Hub.Broadcast holds write lock for entire fan-out; failed connections cleaned up within same lock acquisition"
  - "Write deadline of 10s per write prevents slow/stalled clients from blocking the broadcast"

patterns-established:
  - "Hub pattern: per-session subscriber map protected by sync.RWMutex; Subscribe/Unsubscribe/Broadcast methods"
  - "WS handler pattern: auth → session check → upgrade → subscribe → send initial state → read loop"

requirements-completed:
  - RESL-02

# Metrics
duration: 2min
completed: 2026-02-26
---

# Phase 2 Plan 4: WebSocket Notifications Summary

**Per-session WebSocket hub with gorilla/websocket, inline API key auth, and completion broadcast from submitResultHandler**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-26T20:32:42Z
- **Completed:** 2026-02-26T20:34:52Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments

- WebSocket hub managing per-session subscriber lists with mutex-protected broadcast fan-out
- GET /api/v1/sessions/:id/ws endpoint upgrading HTTP to WebSocket with API key validation before upgrade
- Immediate initial status message sent after connection so subscriber is never out of sync
- BroadcastCompletion integrated into submitResultHandler to push full result metadata to all WS subscribers
- gorilla/websocket added as the only new dependency

## Task Commits

1. **Task 1: WebSocket hub for per-session subscriber management** - `bdf52eb` (feat)
2. **Task 2: WS upgrade handler, hub wiring, completion broadcast** - `473daf8` (feat)

**Plan metadata:** (docs commit below)

## Files Created/Modified

- `internal/ws/hub.go` — Hub struct with Subscribe/Unsubscribe/Broadcast/BroadcastStatusUpdate/BroadcastCompletion/CloseSession
- `internal/server/ws_controller.go` — wsHandler with inline auth, session check, upgrade, initial status send, read loop
- `internal/server/server.go` — Hub field added to Server struct; Hub initialised in NewServer; WS route registered
- `internal/server/result_controller.go` — BroadcastCompletion called after MarkSessionCompleted succeeds
- `go.mod` / `go.sum` — gorilla/websocket v1.5.3 added

## Decisions Made

- WS endpoint registered on `s.Engine` (not `s.ProtectedAPI`) so that query-param API key works; auth is performed inline before upgrade rather than via middleware
- Initial `status_update` message sent synchronously after upgrade and subscribe to eliminate any status race for subscribers who connect mid-session
- Hub.Broadcast holds the write lock across all fan-out writes; failed connections are collected and cleaned up before releasing the lock — single traversal, no lock re-entry

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 3 (phone UI) can trigger `BroadcastStatusUpdate` for `opened` and `action_started` status changes; hub and method are already in place
- Phase 4 (Go client library) can implement WS subscription using the documented endpoint and message schema
- Polling endpoint (`GET /api/v1/sessions/:id/result`) remains fully functional as a fallback alongside WebSocket

---
*Phase: 02-session-core*
*Completed: 2026-02-26*

## Self-Check: PASSED

- internal/ws/hub.go: FOUND
- internal/server/ws_controller.go: FOUND
- .planning/phases/02-session-core/02-04-SUMMARY.md: FOUND
- Commit bdf52eb: FOUND
- Commit 473daf8: FOUND
