---
phase: 06-scan-capture-and-crop-ui
plan: 03
subsystem: scan-ui
tags: [filmstrip, drag-reorder, review-screen, batch-upload, multi-page, multi-document, pointer-events]
dependency_graph:
  requires:
    - 06-02 (pages[] array stub, currentDocIndex, currentWarpedBlob, currentThumbnailURL, acceptPage multi-page branch)
    - 05-scan-server-infrastructure (POST /s/:id/scan/upload with document_index/page_index, POST /s/:id/scan/finalize)
  provides:
    - Filmstrip thumbnail strip with page numbers and document separators
    - Tap-to-select and delete (with URL revocation) for filmstrip pages
    - Long-press drag-to-reorder with pointer clone and insertion line
    - Document boundary management via nextDocument() incrementing currentDocIndex
    - Full-screen review screen grouping pages by document with back/submit controls
    - submitAllPages() sequential upload with progress ("Uploading page N of M...") then finalize
    - Complete multi-page and multi-document scan flow end-to-end
  affects:
    - internal/web/templates/action_scan.html
tech_stack:
  added: []
  patterns:
    - Filmstrip rebuilt declaratively from pages[] array on every state change (renderFilmstrip)
    - Long-press drag via setTimeout(500ms) + pointer movement threshold cancel (5px)
    - Pointer clone follows finger; insertion line shows drop target; reorder on pointerup
    - Review screen built dynamically from pages[] grouped by docIndex
    - Sequential fetch loop with per-iteration spinner text for upload progress feedback
    - Object URL lifecycle: revoke on delete, revoke all on successful submission

key-files:
  created: []
  modified:
    - internal/web/templates/action_scan.html

key-decisions:
  - "renderFilmstrip() rebuilds entire filmstrip DOM from pages[] on each change — simple, correct, no stale state"
  - "Delete shows inline X button on selected thumbnail rather than a separate floating trash button"
  - "Long-press drag does NOT call preventDefault on pointerdown — preserves horizontal scroll behavior"
  - "reorderPages() adjusts target index for the removed element to avoid off-by-one"
  - "filmstripSubmitRow visibility tied to same conditions as filmstripBar (pages.length > 0 + capture/crop/preview state)"
  - "showScreen() now uses a name->id map for reliable state-to-DOM mapping"

patterns-established:
  - "Declarative filmstrip rendering: always call renderFilmstrip() after any pages[] mutation"
  - "Long-press drag pattern: 500ms timer, 5px movement cancel, pointer capture on drag start"

requirements-completed:
  - CAPT-03
  - PAGE-02
  - PAGE-03
  - DOCS-01
  - DOCS-02

duration: 3min
completed: "2026-02-27"
---

# Phase 6 Plan 3: Filmstrip, Review Screen, and Batch Submission Summary

**Multi-page filmstrip with drag-to-reorder, document boundary separators, full-screen review grouped by document, and sequential batch upload with per-page progress feedback.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-27T16:43:03Z
- **Completed:** 2026-02-27T16:46:17Z
- **Tasks:** 2 (implemented together in one commit — inseparable)
- **Files modified:** 1

## Accomplishments

- Filmstrip horizontal strip with thumbnail images, page number badges, and document separator dividers between doc groups
- Tap to select a page, inline delete (X) button on selected thumbnail revokes its object URL and splices from pages[]
- Long-press (500ms) initiates drag mode with floating clone following pointer and insertion line indicator; reorder applied on pointerup
- nextDocument() increments currentDocIndex for multi-document sessions; filmstrip separator auto-inserted between doc groups
- Review screen dynamically builds from pages[] grouped by docIndex; doc headers shown only when multiple documents
- submitAllPages() assigns page_index per document group, uploads each page sequentially with "Uploading page N of M..." spinner text, finalizes, redirects
- Error handling: re-enables Submit button and returns to review screen on any upload/finalize failure
- All object URLs revoked on deletion and after successful submission

## Task Commits

Both tasks implemented atomically in a single commit — filmstrip state machine and review/submit logic are interleaved throughout the same function set:

1. **Tasks 1+2: Filmstrip, review screen, batch submission** - `f8efabd` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `internal/web/templates/action_scan.html` — Complete multi-page scan UI: filmstrip CSS+HTML, renderFilmstrip(), drag-reorder, deletePage(), nextDocument(), showReview()/hideReview(), submitAllPages(), updated showScreen() with name->id map, all window.* exports

## Decisions Made

- `renderFilmstrip()` rebuilds the entire filmstrip DOM from `pages[]` on every state change — simple and correct, no risk of stale state diverging from data
- Delete button shown inline on the selected thumbnail (X overlay) rather than a separate floating trash button above the filmstrip — saves vertical space
- Long-press drag does NOT call `preventDefault` on `pointerdown` to preserve the horizontal scroll gesture on the filmstrip (per plan's pitfall note)
- `reorderPages(fromIndex, toIndex)` adjusts the insert index after splice to avoid off-by-one: `adjustedTo = toIndex > fromIndex ? toIndex - 1 : toIndex`
- `showScreen()` now uses an explicit name-to-ID map (`{ capture: 'captureView', ... }`) — more robust than string concatenation
- "Review & Submit" button lives in a separate `filmstripSubmitRow` div above the filmstrip strip for visual separation and tap target clarity

## Deviations from Plan

None — plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 6 is complete: all scan UI functionality implemented across plans 06-01 through 06-03
- Complete flow working: capture -> crop -> warp -> preview -> accept -> filmstrip -> (repeat or next doc) -> review -> submit -> success
- Single-page, multi-page, and multi-document workflows all supported
- Server infrastructure (Phase 5) and UI (Phase 6) fully integrated

## Self-Check: PASSED

---
*Phase: 06-scan-capture-and-crop-ui*
*Completed: 2026-02-27*
