---
phase: 03-phone-ui-and-actions
plan: "04"
subsystem: web-ui
tags: [signature, canvas, ui, static-assets]
dependency_graph:
  requires: ["03-01"]
  provides: ["signature-pad-ui"]
  affects: ["internal/web/templates", "internal/web/html/public"]
tech_stack:
  added: ["signature_pad@5.0.4 (szimek, MIT, UMD bundle embedded as static asset)"]
  patterns: ["full-screen canvas with devicePixelRatio scaling", "stroke-level undo/redo via toData/fromData", "portrait orientation overlay hint"]
key_files:
  created:
    - internal/web/html/public/signature_pad.umd.min.js
  modified:
    - internal/web/templates/action_signature.html
decisions:
  - "signature_pad v5.0.4 UMD bundle downloaded from jsdelivr CDN and committed to repo as static asset — embedded in binary via existing go:embed directive"
  - "Undo/redo implemented via signaturePad.toData()/fromData() stroke array manipulation — no library-level undo API needed"
  - "PDF output_format treated as PNG capture — same as photo approach; caller handles PDF conversion"
  - "Canvas resizeCanvas() preserves existing strokes by saving/restoring toData() across resize"
metrics:
  duration: "2min"
  completed_date: "2026-02-26"
  tasks_completed: 2
  files_changed: 2
---

# Phase 3 Plan 4: Signature Pad UI Summary

Full-screen touch signature canvas using signature_pad v5.0.4 with stroke-level undo/redo, landscape orientation hint, and base64 JSON submission to the existing result endpoint.

## What Was Built

**Task 1 — Download and embed signature_pad library (142334d)**

Downloaded `signature_pad@5.0.4` UMD bundle from `cdn.jsdelivr.net` and placed it at `internal/web/html/public/signature_pad.umd.min.js`. The file is automatically embedded in the binary via the existing `//go:embed all:html` directive in `web.go` and served at `/static/public/signature_pad.umd.min.js`.

**Task 2 — Implement signature pad template (7573860)**

Replaced the 3-line placeholder `action_signature.html` with a 248-line complete implementation:

- `styles` block: CSS for full-screen container, canvas wrapper, bottom toolbar, rotation overlay, and spinner overlay
- `content` block: rotation hint overlay (with dismiss button), `sig-container` div with canvas and toolbar (undo/redo/clear/submit), spinner overlay
- `scripts` block: loads `signature_pad.umd.min.js`, initializes `SignaturePad` with white background, implements `resizeCanvas()` with `devicePixelRatio` support, `updateButtons()`, `undoStroke()`, `redoStroke()`, `clearPad()`, `submitSignature()` (async fetch), and orientation check logic

## Key Design Details

- **Canvas sizing**: `devicePixelRatio`-aware width/height set in pixels; CSS keeps it at 100%/100% — crisp on high-DPI phones.
- **Undo/redo**: `signaturePad.toData()` returns `PointGroup[]`. Undo pops last group to `undoneStrokes[]`. Redo pushes back. `endStroke` event clears redo stack on new input.
- **Landscape hint**: Shown when `window.innerHeight > window.innerWidth`. Dismissed permanently by clicking "Continue Anyway" (sets `hintDismissed = true`). Re-evaluates on `resize` and `orientationchange`.
- **Output format**: `svg` → `toSVG()` + `btoa()` + `image/svg+xml`; anything else (png, pdf) → `toDataURL('image/png')` + `image/png`.
- **Submission**: POST JSON `{items: [{content_type, filename, data}]}` to `{{.SubmitURL}}`. On `response.ok` → redirect to `/s/{{.SessionID}}` (success page). On error → hide spinner, re-enable submit, `alert()` with error message.

## Template Variables Used

| Variable | Source | Usage |
|----------|--------|-------|
| `.SessionID` | `renderActionPage` | Redirect URL after submit |
| `.SubmitURL` | `renderActionPage` | POST endpoint (`/s/:id/result`) |
| `.OutputFormat` | `renderActionPage` | Drives SVG vs PNG export |

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check

- [x] `internal/web/html/public/signature_pad.umd.min.js` — exists, non-empty, contains `SignaturePad`
- [x] `internal/web/templates/action_signature.html` — 248 lines (min 100), contains `SignaturePad`, script tag, `.SubmitURL`
- [x] `go build ./...` — passes
- [x] `go vet ./...` — passes
- [x] Commit 142334d — Task 1
- [x] Commit 7573860 — Task 2

## Self-Check: PASSED
