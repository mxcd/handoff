---
phase: 05-scan-server-infrastructure
plan: 02
subsystem: server
tags: [scan, upload, finalize, pdf, result-delivery, endpoints]
dependency_graph:
  requires: ["05-01"]
  provides: ["scan-upload-endpoint", "scan-finalize-endpoint", "scan-result-delivery"]
  affects: ["internal/server/scan_controller.go", "internal/server/session_controller.go", "internal/server/result_controller.go", "internal/server/server.go"]
tech_stack:
  added: []
  patterns: ["multipart/form-data upload", "PDF assembly via ImagesToPDF", "WebSocket broadcast on finalization"]
key_files:
  created:
    - internal/server/scan_controller.go
    - internal/web/templates/action_scan.html
  modified:
    - internal/server/session_controller.go
    - internal/server/session_page_controller.go
    - internal/server/result_controller.go
    - internal/server/server.go
decisions:
  - "Finalize endpoint placed on /s/:id/scan/finalize (public, phone-facing) — mirrors /s/:id/result pattern; session UUID provides security"
  - "Page limit response returns structured JSON {error, limit, current} for actionable client feedback"
  - "BroadcastCompletion called with ScanResult struct (not pointer) matching interface{} signature"
metrics:
  duration: "3 min"
  completed_date: "2026-02-27"
  tasks_completed: 3
  files_changed: 6
---

# Phase 5 Plan 02: Scan Upload, Finalize, and Result Delivery Summary

Scan server endpoints implemented — multipart page upload with size/count limits, PDF assembly or image pass-through on finalize, scan_result included in result polling response.

## Tasks Completed

| # | Task | Commit | Key Files |
|---|------|--------|-----------|
| 1 | Extend session creation for scan action type | 727fa1d | session_controller.go, session_page_controller.go, action_scan.html |
| 2 | Create scan upload and finalize controllers | aa03812 | scan_controller.go, server.go |
| 3 | Update result delivery to include scan_result | a839e1f | result_controller.go |

## What Was Built

**Task 1 — Session creation for scan:**
- `createSessionRequest` extended with `DocumentMode string` field
- `OutputFormat` binding tag changed from `required` to optional; manual validation per action type
- Scan branch: validates `document_mode` (default `single`) and `output_format` (default `pdf`) using scan-specific validators
- Photo/signature branch: unchanged, `output_format` still required
- `renderActionPage` switch extended with `ActionTypeScan` case routing to `action_scan.html`
- Placeholder `action_scan.html` template created (Phase 6 will replace with full scan UI)

**Task 2 — Scan upload and finalize:**
- `scan_controller.go` — `scanUploadHandler` and `scanFinalizeHandler`
- Upload handler: validates session exists/is-scan/not-expired/not-completed; enforces `SCAN_UPLOAD_MAX_BYTES` via `http.MaxBytesReader` (returns 413); enforces `SCAN_MAX_PAGES` page count limit (returns 400 with `{error, limit, current}`); forces `document_index=0` in single-document mode; stores page with remaining session TTL
- Finalize handler: groups pages by `DocumentIndex`, sorts by `PageIndex`; assembles multi-page PDF per document via `util.ImagesToPDF` (pdf mode) or stores individual images (images mode); calls `MarkScanSessionCompleted`, `BroadcastCompletion`, `ClearScanPages`
- Routes registered: `POST /s/:id/scan/upload` and `POST /s/:id/scan/finalize` (public, phone-facing)

**Task 3 — Result delivery:**
- `getResultHandler`: scan sessions now include `scan_result` field alongside `items` in completed response
- Photo/signature sessions response unchanged

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] jsonError takes string not gin.H**
- **Found during:** Task 2
- **Issue:** Plan instructed passing `gin.H{}` to `jsonError()` for page-limit response, but `jsonError` signature only accepts a `string` message
- **Fix:** Used `c.JSON` directly for the page-limit exceeded case to include `{error, limit, current}` structure
- **Files modified:** internal/server/scan_controller.go

**2. [Rule 1 - Bug] Session.RemainingTTL() method does not exist**
- **Found during:** Task 2
- **Issue:** Plan referenced `session.RemainingTTL()` method which was not defined on the Session model
- **Fix:** Calculated remaining TTL inline: `time.Until(session.CreatedAt.Add(session.SessionTTL))`
- **Files modified:** internal/server/scan_controller.go

## Self-Check: PASSED

- internal/server/scan_controller.go: FOUND
- internal/web/templates/action_scan.html: FOUND
- .planning/phases/05-scan-server-infrastructure/05-02-SUMMARY.md: FOUND
- Commit 727fa1d (Task 1): FOUND
- Commit aa03812 (Task 2): FOUND
- Commit a839e1f (Task 3): FOUND
