---
phase: 05-scan-server-infrastructure
plan: 01
subsystem: model
tags: [go, gofpdf, pdf, in-memory-store, scan, document-scanning]

# Dependency graph
requires: []
provides:
  - ActionTypeScan constant and updated ValidateActionType
  - ScanDocumentMode / ScanOutputFormat types with validation functions
  - ScanResult / ScanDocument / ScanPage nested result types
  - Session struct scan-specific fields (document_mode, scan_output_format, scan_result)
  - phpdave11/gofpdf v1.4.3 replacing archived jung-kurt/gofpdf/v2
  - ImagesToPDF multi-page PDF assembly function
  - SCAN_UPLOAD_MAX_BYTES and SCAN_MAX_PAGES config values
  - Store.AddScanPage / GetScanPages / GetScanPageCount / ClearScanPages / MarkScanSessionCompleted
affects:
  - 05-02-scan-upload-controller
  - 05-03-client-library

# Tech tracking
tech-stack:
  added:
    - github.com/phpdave11/gofpdf v1.4.3 (drop-in replacement for archived jung-kurt/gofpdf/v2)
  patterns:
    - Scan-specific fields on Session use omitempty so they are absent on photo/signature sessions
    - ScanPageData (store-internal) vs model.ScanPage (result type) are distinct types
    - scanPages is a third in-memory cache on Store, keyed by "scanpages:{sessionID}"
    - Per-page TTL on AddScanPage matches remaining session TTL

key-files:
  created: []
  modified:
    - internal/model/session.go
    - internal/util/pdf.go
    - internal/util/config.go
    - internal/store/store.go
    - go.mod
    - go.sum

key-decisions:
  - "phpdave11/gofpdf v1.4.3 used as drop-in replacement for archived jung-kurt/gofpdf/v2 — identical API, same package name"
  - "ScanPageData (raw bytes) kept separate from model.ScanPage (result URL type) to avoid coupling store internals to model result shape"
  - "ValidateOutputFormat ActionTypeScan case returns empty string without error, since scan sessions use ScanOutputFormat instead"
  - "scanPages cache initialized with tombstoneTTL (24h) as default to match session lifetime ceiling"

patterns-established:
  - "Scan fields on Session are additive with omitempty — backwards-compatible with existing photo/signature callers"
  - "ImagesToPDF uses AddPageFormat per page to support variable dimensions across scan pages"

requirements-completed: [SCAN-01, SCAN-02, SCAN-03, INFR-11, INFR-12]

# Metrics
duration: 3min
completed: 2026-02-27
---

# Phase 5 Plan 01: Scan Server Infrastructure Summary

**Scan model types, phpdave11/gofpdf migration with multi-page ImagesToPDF, and in-memory scan page accumulation in Store**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-27T15:54:40Z
- **Completed:** 2026-02-27T15:57:11Z
- **Tasks:** 3
- **Files modified:** 6 (session.go, pdf.go, config.go, store.go, go.mod, go.sum)

## Accomplishments
- Extended session model with ActionTypeScan, ScanDocumentMode/ScanOutputFormat types, validation functions, and ScanResult/ScanDocument/ScanPage nested result types added as omitempty fields on Session
- Migrated PDF library from archived jung-kurt/gofpdf/v2 to phpdave11/gofpdf v1.4.3 (identical API), added ImagesToPDF for multi-page per-image-dimension PDF assembly
- Extended Store with a third scanPages cache and AddScanPage/GetScanPages/GetScanPageCount/ClearScanPages/MarkScanSessionCompleted methods

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend session model with scan types and validation** - `116acef` (feat)
2. **Task 2: Migrate PDF library and add multi-page assembly** - `233bad3` (feat)
3. **Task 3: Add scan config values and store scan page accumulation** - `f5a15c5` (feat)

## Files Created/Modified
- `internal/model/session.go` - Added ActionTypeScan, ScanDocumentMode, ScanOutputFormat, ScanPage, ScanDocument, ScanResult types; extended Session struct; updated validation functions
- `internal/util/pdf.go` - Migrated to phpdave11/gofpdf import; removed unused io import; added ImagesToPDF multi-page function
- `internal/util/config.go` - Added SCAN_UPLOAD_MAX_BYTES (20MB default) and SCAN_MAX_PAGES (50 default)
- `internal/store/store.go` - Added ScanPageData type, scanPages cache, scanPagesKey helper, AddScanPage/GetScanPages/GetScanPageCount/ClearScanPages/MarkScanSessionCompleted methods
- `go.mod` - phpdave11/gofpdf v1.4.3 added, jung-kurt/gofpdf/v2 removed
- `go.sum` - Updated checksums after go mod tidy

## Decisions Made
- Used phpdave11/gofpdf v1.4.3 as direct drop-in replacement — same package name `gofpdf`, identical type and function signatures, no other code changes needed
- Kept ScanPageData (store-level raw bytes) separate from model.ScanPage (result URL type) to avoid coupling store internals to the API result shape
- ValidateOutputFormat for ActionTypeScan returns empty OutputFormat with no error — scan sessions use ScanOutputFormat, not the shared OutputFormat field
- scanPages cache initialized with tombstoneTTL (24h) ceiling to safely cover any session lifetime

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All model contracts are established for Plan 05-02 (scan upload/finalize controllers)
- ImagesToPDF is ready for use in the finalize handler
- Store methods AddScanPage/GetScanPages/ClearScanPages/MarkScanSessionCompleted provide the full page accumulation API
- Config values SCAN_UPLOAD_MAX_BYTES and SCAN_MAX_PAGES are accessible for request validation
- go build ./... and go test -race ./... pass with no regressions

---
*Phase: 05-scan-server-infrastructure*
*Completed: 2026-02-27*
