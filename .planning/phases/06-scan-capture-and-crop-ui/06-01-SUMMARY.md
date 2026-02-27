---
phase: 06-scan-capture-and-crop-ui
plan: 01
subsystem: scan-ui
tags: [canvas, crop-ui, exif, camera, pointer-events, magnifier]
dependency_graph:
  requires:
    - 05-scan-server-infrastructure (POST /s/:id/scan/upload, POST /s/:id/scan/finalize)
    - internal/model/session.go (ScanDocumentMode, ScanOutputFormat)
  provides:
    - Scan capture + crop UI in action_scan.html
    - ScanDocumentMode/ScanUploadURL/ScanFinalizeURL wired through session_page_controller.go
  affects:
    - internal/server/session_page_controller.go
    - internal/web/templates/action_scan.html
tech_stack:
  added:
    - Browser Canvas 2D API (crop rendering, overlay, loupe)
    - Browser Pointer Events API (drag with setPointerCapture)
    - CSS image-orientation: from-image (EXIF normalization)
  patterns:
    - Multi-screen state machine (captureView/cropView/previewView/reviewView/spinnerView)
    - Offscreen canvas EXIF normalization (CSS img + drawImage)
    - Composite operation destination-out for quad punch-out
    - Image-pixel-space coordinate storage for downstream warp
key_files:
  created: []
  modified:
    - internal/server/session_page_controller.go
    - internal/web/templates/action_scan.html
decisions:
  - "accept=\"image/*\" without capture attribute — Android 14/15 Chrome hides camera with capture attr"
  - "CSS image-orientation: from-image on hidden img then drawImage — zero JS weight EXIF normalization"
  - "Canvas coordinates NOT multiplied by devicePixelRatio — CSS pixels for simpler hit testing (per research pitfall #2)"
  - "cropPoints stored in image pixel space — not canvas coords — for downstream homography warp"
  - "confirmCrop() is a stub logging to console — warp + preview deferred to plan 06-02"
metrics:
  duration: 3 min
  completed_date: "2026-02-27"
  tasks_completed: 2
  files_modified: 2
---

# Phase 6 Plan 1: Scan Capture and Crop UI Summary

**One-liner:** Capture-to-crop UI with EXIF-normalized offscreen canvas, L-bracket corner handles with quad overlay, dim cutout rendering, and 3x magnifying glass via Canvas 2D + Pointer Events.

## What Was Built

### Task 1: Scan template data + state machine scaffold

Added scan-specific template data in `renderActionPage()`:

```go
case model.ActionTypeScan:
    data["ScanDocumentMode"] = string(session.ScanDocumentMode)
    data["ScanUploadURL"] = fmt.Sprintf("/s/%s/scan/upload", session.ID)
    data["ScanFinalizeURL"] = fmt.Sprintf("/s/%s/scan/finalize", session.ID)
    templateName = "action_scan.html"
```

Replaced the `action_scan.html` placeholder with a full multi-screen state machine:
- **captureView** — full-screen dark background, centered camera button, `<input type="file" accept="image/*">` (no `capture` attr for Android 14/15 compatibility)
- **cropView** — canvas filling the viewport above a floating Confirm button, `touch-action: none`
- **previewView** — placeholder div (06-02)
- **reviewView** — placeholder div (06-03)
- **filmstripBar** — placeholder bottom strip (06-03)
- **spinnerView** — blocking overlay with CSS spinner

EXIF normalization via `loadNormalizedImage(file)`: creates a hidden `<img>` with `image-orientation: from-image`, waits for `onload`, draws into an offscreen canvas via `drawImage`, revokes the object URL, and resolves the promise with the orientation-corrected canvas stored as `currentSourceCanvas`.

### Task 2: Crop canvas rendering and interaction

Full crop UI in the `{{define "scripts"}}` block:

**`drawCropFrame()`** renders in order:
1. Source image scaled and letterboxed into the canvas
2. 45% opacity dark overlay (`rgba(0,0,0,0.45)`) over the whole canvas
3. Punch out the crop quad with `globalCompositeOperation = 'destination-out'`
4. Re-draw source image inside quad only (canvas clip path)
5. White 1.5px connecting lines between all 4 corners
6. L-bracket handles at each corner via `drawHandles()`
7. Magnifying glass loupe when a handle is being dragged

**`drawHandles()`** draws two perpendicular 20px arms per corner, pointing outward from the quad center. Corner directions: TL=left+up, TR=right+up, BR=right+down, BL=left+down.

**`drawLoupe()`** renders a 3x zoom circular magnifier (50px radius) using `ctx.drawImage` with source/dest region math for zoom, clipped to an arc. Smart positioning: default 80px above+right of finger; shifts below near top edge; shifts left/right near lateral edges. White 2px border, red 3px dot at center.

**Pointer events:**
- `pointerdown`: hits test all 4 corners (30px radius), sets `activeHandle`, calls `setPointerCapture`
- `pointermove`: back-projects canvas CSS-pixel position to image pixel coords, clamps to image bounds, updates `cropPoints[activeHandle]`, redraws
- `pointerup`/`pointercancel`: clears `activeHandle`, redraws without loupe

**`confirmCrop()`** stub: logs crop point coordinates to console; actual warp + preview implementation deferred to plan 06-02.

## Commits

| Hash | Description |
|------|-------------|
| 67afccb | feat(06-01): add scan template data and action_scan.html state machine scaffold |
| b2ceed7 | feat(06-01): implement crop canvas with L-bracket handles, dim overlay, and magnifying glass |

## Deviations from Plan

None — plan executed exactly as written.

## Verification

- [x] `go build -o /dev/null ./cmd/server` passes
- [x] `go vet ./...` passes with no warnings
- [x] session_page_controller.go passes ScanDocumentMode, ScanUploadURL, ScanFinalizeURL to scan template
- [x] action_scan.html defines styles, content, and scripts blocks
- [x] Capture input uses `accept="image/*"` without capture attribute
- [x] EXIF normalization uses CSS `image-orientation: from-image` + canvas drawImage
- [x] Crop canvas draws: image, overlay, quad cutout, connecting lines, L-bracket handles
- [x] Pointer events use setPointerCapture for reliable drag
- [x] Magnifier is 3x zoom with smart positioning, visible only during drag
- [x] cropPoints stored in image pixel coordinates
- [x] action_scan.html is 472 lines (> 200 minimum)
