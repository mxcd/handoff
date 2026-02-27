---
phase: 05-scan-server-infrastructure
plan: "03"
subsystem: api
tags: [go, client-library, scan, websocket, polling]

# Dependency graph
requires:
  - phase: 05-scan-server-infrastructure
    provides: "05-01: scan model types, store extension; 05-02: scan upload and result endpoints"
provides:
  - "ActionTypeScan constant in pkg/handoff/types.go"
  - "ScanDocumentMode, ScanOutputFormat types with constants"
  - "ScanResult, ScanDocumentResult, ScanPageResult nested types"
  - "WithDocumentMode and WithScanOutputFormat builder methods on SessionBuilder"
  - "WaitForScanResult method for typed scan result retrieval"
  - "WebSocket and poll result parsing for scan sessions"
affects:
  - "06-scan-ui: phone UI integration uses scan session types"
  - "future consumer apps using pkg/handoff"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "ScanOutputFormat cast to OutputFormat in Invoke() for JSON serialization — keeps type safety at builder level"
    - "scanResult stored on Session struct under mu; WaitForScanResult reads it after resultCh receives"
    - "Dual-attempt JSON unmarshaling: try []ResultItem first, fallback to ScanResult for completed WebSocket messages"

key-files:
  created: []
  modified:
    - pkg/handoff/types.go
    - pkg/handoff/client.go
    - pkg/handoff/session.go

key-decisions:
  - "ScanOutputFormat cast to OutputFormat for JSON: keeps type safety at builder, server receives output_format field for both scan and non-scan sessions"
  - "WaitForScanResult reads scanResult from Session struct after resultCh signal — avoids new channel, reuses existing completion signaling"
  - "Dual JSON unmarshal in readWebSocketMessages: []ResultItem then ScanResult — photo/signature unaffected"

patterns-established:
  - "scan session creation: client.NewSession().WithAction(ActionTypeScan).WithDocumentMode(ScanDocumentModeSingle).WithScanOutputFormat(ScanOutputFormatPDF).Invoke(ctx)"
  - "scan result retrieval: session.WaitForScanResult(ctx)"

requirements-completed: [CLIB-01, CLIB-02]

# Metrics
duration: 3min
completed: 2026-02-27
---

# Phase 5 Plan 03: Go Client Library Scan Support Summary

**Typed scan session creation and result retrieval in pkg/handoff via WithDocumentMode/WithScanOutputFormat builder methods and WaitForScanResult**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-02-27T10:19:27Z
- **Completed:** 2026-02-27T10:22:47Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Added ActionTypeScan, ScanDocumentMode, ScanOutputFormat constants and all nested scan result types to types.go
- Extended SessionBuilder with WithDocumentMode and WithScanOutputFormat; Invoke correctly serializes scan sessions (defaults output_format to "pdf") while keeping photo/signature unchanged
- WaitForScanResult provides typed *ScanResult retrieval; WebSocket and polling paths both parse and store scan_result

## Task Commits

Each task was committed atomically:

1. **Task 1: Add scan types to client library** - `34f0044` (feat)
2. **Task 2: Extend SessionBuilder and result parsing for scan sessions** - `7c6db7c` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `/Users/mapa/github.com/mxcd/handoff/pkg/handoff/types.go` - ActionTypeScan, ScanDocumentMode, ScanOutputFormat, ScanPageResult, ScanDocumentResult, ScanResult types; DocumentMode on CreateSessionRequest; ScanResult on Event and resultPollResponse; scan fields on sessionResponse
- `/Users/mapa/github.com/mxcd/handoff/pkg/handoff/client.go` - documentMode/scanOutputFormat fields on SessionBuilder; WithDocumentMode/WithScanOutputFormat methods; scan branch in Invoke(); scan fields on SessionInfo and sessionResponseToInfo
- `/Users/mapa/github.com/mxcd/handoff/pkg/handoff/session.go` - scanResult field on Session; WaitForScanResult method; dual-unmarshal in readWebSocketMessages; scanResult storage in pollResult; ScanResult on polling event

## Decisions Made
- ScanOutputFormat cast to OutputFormat in Invoke() so the server receives `output_format` JSON field for scan sessions, maintaining server API compatibility while providing type safety at the builder level
- Dual-attempt JSON unmarshal ([]ResultItem first, then ScanResult) preserves existing photo/signature behavior with zero code changes to those paths
- WaitForScanResult reuses resultCh signaling — avoids new channel complexity, consistent with WaitForResult pattern

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

Pre-existing build errors in `internal/server/scan_controller.go` (unused import, wrong jsonError signature, undefined RemainingTTL) were present before this plan. They are out of scope and logged to deferred items. The `pkg/handoff` client library builds and vets cleanly.

## Next Phase Readiness
- Client library fully supports scan sessions
- Go applications can create scan sessions, set document mode and output format, and receive typed ScanResult
- Ready for Phase 6 scan UI work (phone capture, crop, warp, multi-page)
- Deferred: pre-existing scan_controller.go errors need fixing in a future plan

## Self-Check: PASSED

- FOUND: pkg/handoff/types.go
- FOUND: pkg/handoff/client.go
- FOUND: pkg/handoff/session.go
- FOUND: commit 34f0044 (feat(05-03): add scan types to client library)
- FOUND: commit 7c6db7c (feat(05-03): extend SessionBuilder and result parsing for scan sessions)
- go vet ./pkg/handoff/... PASSED
- go build ./pkg/... PASSED

---
*Phase: 05-scan-server-infrastructure*
*Completed: 2026-02-27*
