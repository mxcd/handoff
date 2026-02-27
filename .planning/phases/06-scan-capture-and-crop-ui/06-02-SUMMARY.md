---
phase: 06-scan-capture-and-crop-ui
plan: 02
subsystem: scan-ui
tags: [canvas, homography, perspective-warp, preview, upload, formdata, esm]
dependency_graph:
  requires:
    - 06-01 (currentSourceCanvas, cropPoints, state machine scaffold)
    - 05-scan-server-infrastructure (POST /s/:id/scan/upload, POST /s/:id/scan/finalize)
  provides:
    - Perspective warp via Homography.js producing flat document preview
    - Preview screen with Retake/Re-crop/Accept three-button control bar
    - Single-page scan flow: capture -> crop -> warp -> preview -> upload -> finalize -> success
    - Multi-page page accumulation stub (pages array) for plan 06-03
  affects:
    - internal/web/templates/action_scan.html
tech_stack:
  added:
    - Homography.js v1.8.1 (ESM, CDN) — projective perspective warp
    - Browser FormData API — multipart/form-data upload without manual Content-Type
  patterns:
    - ESM script type=module with window.* exports for onclick compatibility
    - Projective homography with normalized 0..1 source/destination points
    - Edge-length output sizing (max of top/bottom widths, max of left/right heights)
    - Max 1500px long-side cap before warp to keep mobile performance acceptable
    - Two-canvas warp pipeline: Homography produces ImageData, scaled into output canvas
    - Blob object URL lifecycle management (create, use, revoke after upload/retake)
key_files:
  created: []
  modified:
    - internal/web/templates/action_scan.html
decisions:
  - "script type=module for ESM Homography.js import; window.* exports for inline onclick compatibility"
  - "Warp output sized from quad edge lengths (max of opposite sides), capped at 1500px long side"
  - "h.warp() returns ImageData at original image ratio; scaled down into final canvas via drawImage"
  - "Thumbnail 56x64 max generated from warped canvas for filmstrip in plan 06-03"
  - "acceptPage() handles 410 (expired) and 409 (completed) HTTP statuses as special redirects"
  - "Multi-page mode accumulates pages[] and returns to capture; filmstrip rendering deferred to 06-03"
metrics:
  duration: 4 min
  completed_date: "2026-02-27"
  tasks_completed: 2
  files_modified: 1
---

# Phase 6 Plan 2: Homography Warp, Preview Screen, and Single-Page Flow Summary

**One-liner:** Perspective warp via Homography.js ESM with edge-length output sizing, preview screen with three-button control bar, and complete single-page upload+finalize flow using FormData.

## What Was Built

### Task 1: Integrate Homography.js and implement perspective warp + preview screen

Changed `{{define "scripts"}}` block to `<script type="module">` to enable ESM imports, then imported Homography.js v1.8.1 from jsDelivr CDN.

**`confirmCrop()` implementation:**
1. Shows spinner overlay with "Processing..." label
2. Computes output dimensions from quad edge lengths: `topLen = Math.hypot(TR-TL)`, `leftLen = Math.hypot(BL-TL)`, takes max of opposite sides
3. Applies 1500px long-side cap for warp performance on mobile (per research pitfall #3)
4. Gets `ImageData` from `currentSourceCanvas` via `getContext('2d').getImageData()`
5. Normalizes source crop points to 0..1: `src = cropPoints.map(([x,y]) => [x/W, y/H])`
6. Sets destination as unit rectangle: `[[0,0],[1,0],[1,1],[0,1]]` (TL, TR, BR, BL)
7. Creates `new Homography("projective")`, calls `setReferencePoints(src, dst, imageData)`, then `warp()`
8. Renders warped ImageData into temp canvas, then scales into final `outCanvas` at `finalW x finalH`
9. Converts to JPEG blob at quality 0.9 via `toBlob`, stores as `currentWarpedBlob`
10. Generates 56x64 thumbnail stored as `currentThumbnailURL` for filmstrip in plan 06-03
11. Sets `previewImg.src` to blob object URL, calls `showScreen('previewView')`

**Preview screen HTML and CSS:**
- Full-screen `#previewView` with `<img id="previewImg">` filling available space via `object-fit: contain`
- Three-button control bar at bottom with gradient overlay:
  - "Retake" (left, `btn-retake-scan`) — `retakeScan()`: revokes URLs, clears all state, returns to captureView
  - "Re-crop" (center, `btn-recrop-scan`) — `recropScan()`: returns to cropView with same source canvas and crop points, calls `drawCropFrame()`
  - "Accept" (right, `btn-accept-scan` white/primary) — `acceptPage()`: single-page upload+finalize or multi-page accumulate

**State initialization additions:**
```javascript
let pages = [];               // [{docIndex, blob, thumbnailURL}]
let currentDocIndex = 0;
let currentWarpedBlob = null;
let currentThumbnailURL = null;
```

**Spinner enhancement:** Added `<span id="spinnerLabel">` and `showSpinner(label)` helper that shows the spinner overlay with a contextual message ("Processing..." during warp, "Uploading..." during upload).

**Window exports:** All action functions exported via `window.*` for inline onclick compatibility with ESM module scope:
```javascript
window.confirmCrop = confirmCrop;
window.retakeScan  = retakeScan;
window.recropScan  = recropScan;
window.acceptPage  = acceptPage;
```

### Task 2: Implement single-page upload and finalize flow

**`acceptPage()` function for `documentMode === 'single'`:**
1. Disables Accept button and shows "Uploading..." spinner
2. Creates `FormData` with `file` (blob as `scan.jpg`), `document_index: '0'`, `page_index: '0'`
3. POSTs to `uploadURL` — no Content-Type header set, browser sets multipart boundary automatically
4. Handles HTTP status codes: 410 → alert + redirect to `/s/${sessionID}`; 409 → redirect; other non-ok → throw with server error message
5. POSTs to `finalizeURL` with no body on upload success
6. Handles finalize errors (410, non-ok)
7. Revokes all object URLs (`previewImg.src`, `currentThumbnailURL`) before redirect
8. Redirects to `/s/${sessionID}` on success

**Error recovery:** On any catch, re-shows `previewView` and re-enables Accept button with an alert.

**`acceptPage()` for multi-page mode:**
- Pushes `{docIndex, blob, thumbnailURL}` to `pages` array
- Clears current state (`currentWarpedBlob`, `currentThumbnailURL`, `currentSourceCanvas`, `cropPoints`)
- Returns to captureView for next page
- Filmstrip rendering and page index assignment handled in plan 06-03

## Commits

| Hash | Description |
|------|-------------|
| 1bc0174 | feat(06-02): integrate Homography.js warp and preview screen with Retake/Re-crop/Accept |

Note: Both tasks were implemented together in a single atomic commit since the upload/finalize logic (`acceptPage`) is inseparable from the preview screen it is invoked from.

## Deviations from Plan

None — plan executed exactly as written.

## Verification

- [x] `go build -o /dev/null ./cmd/server` passes
- [x] `go vet ./...` passes with no warnings
- [x] Homography.js loads via ESM import in `<script type="module">`
- [x] `confirmCrop()` computes perspective warp with edge-length output sizing, 1500px cap
- [x] Preview shows warped image (`<img>`) with Retake, Re-crop, Accept buttons
- [x] Re-crop returns to crop UI with same source image and current crop corner positions
- [x] Retake clears all state and returns to capture view
- [x] Single-page Accept uploads via FormData to /s/:id/scan/upload without manual Content-Type
- [x] Successful upload calls /s/:id/scan/finalize
- [x] Successful finalize redirects to /s/{sessionID}
- [x] HTTP 410 and 409 handled as special-case redirects (not generic errors)
- [x] Network/server errors show alert and re-enable Accept button
- [x] Object URLs revoked after successful upload and on Retake
- [x] Thumbnail generated (56x64) and stored as `currentThumbnailURL` for plan 06-03
- [x] `pages[]` array and multi-page accumulation stub in place for plan 06-03

## Self-Check: PASSED
