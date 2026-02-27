# Project Research Summary

**Project:** Handoff v1.1 — Document Scanning Action Type
**Domain:** Browser-based document scanning with manual crop, perspective warp, multi-page capture, and server-side PDF assembly
**Researched:** 2026-02-27
**Confidence:** HIGH (architecture from direct codebase inspection; pitfalls verified against primary sources; stack MEDIUM on JS library maintenance)

## Executive Summary

Handoff v1.1 adds a document scanning action type to an existing Go/Gin server that already handles photo and signature capture sessions. The product pattern — phone user opens a QR-code URL, completes an action, backend polls or subscribes for the result — is already fully implemented. The work is additive: a new `ActionType`, a new HTML template with a multi-step client-side UI, and server-side extensions to handle nested document/page results and multi-image PDF assembly. No new infrastructure, no new routes, no new frameworks. The dominant implementation risk is the browser-side scan UI, which carries 10 verified pitfalls related to EXIF orientation, touch event conflicts, canvas coordinate spaces, and iOS Safari memory limits.

The recommended approach is client-side-warp / server-side-PDF: all perspective correction happens in the browser using a lightweight homography library (Homography.js v1.8.1, ~115 kB, MIT, no build step), and the server receives already-warped JPEG pages which it assembles into multi-page PDFs using `phpdave11/gofpdf` v1.4.3 (the actively maintained fork of the archived `jung-kurt/gofpdf`). This keeps the Go binary free of image processing dependencies. The existing base64-in-JSON submission pattern is extended to a nested `documents[].pages[]` structure. The existing in-memory session and file stores require no architectural changes, only the addition of a `MarkSessionCompletedScan` method and a `max_pages` guard.

The primary API contract risk is backwards compatibility: the existing result endpoint returns `items: []` for photo and signature sessions. The scan result must use a separate `documents` field with `omitempty`, never replacing or aliasing `items`. This decision must be locked in at the model layer before any other scan code is written. A secondary operational risk is upload payload size: a 10-page document scan with base64 encoding can easily exceed nginx's default 1 MB `client_max_body_size`. Explicit body size limits must be applied in the Gin handler, and deployment documentation must call out the required nginx configuration.

## Key Findings

### Recommended Stack

The existing stack (Gin, `go-cache`, `go:embed`, server-rendered HTML templates, UMD JS bundles) requires only two targeted additions. For perspective correction, Homography.js v1.8.1 is the only actively maintained dependency-free library that performs quad-to-quad projective transforms and returns `ImageData` for `putImageData` — the exact operation needed. All other candidates are either unmaintained (perspective-transform), architecturally wrong for this use case (perspective.js, cropperjs), or massively oversized (OpenCV.js at 8 MB). The crop handle UI and multi-page accumulator are pure vanilla Canvas 2D and Pointer Events — no additional library adds value. For PDF assembly, `phpdave11/gofpdf` v1.4.3 is a drop-in import-path replacement for the archived `jung-kurt/gofpdf/v2`; the migration touches only `internal/util/pdf.go` (62 lines) and `go.mod`.

**Core technologies:**
- `homography` v1.8.1 UMD bundle: client-side perspective warp — only active quad-to-quad warp library without dependencies
- Vanilla Canvas 2D + Pointer Events: crop handle UI and magnifier — 3 events, ~80 lines, no library fits canvas-rendered handles anyway
- `canvas.toBlob()` + JS array: multi-page state accumulation — native browser API, full mobile coverage
- `phpdave11/gofpdf` v1.4.3: server-side multi-page PDF assembly — drop-in replacement for archived dependency, v1.4.3 released May 2025
- Extended JSON+base64 body: upload protocol — consistent with existing photo action pattern, nested `documents[].pages[]` structure

### Expected Features

All document scanning action type features ship in v1.1. Auto edge detection (OpenCV.js) and per-page filters are explicitly deferred to v1.x/v2+.

**Must have (table stakes):**
- Camera capture via `<input type="file" capture="environment">` — reuses existing photo action pattern
- Display captured image on canvas for crop review — no scanner skips this step
- 4-corner crop handles positioned outside the document corners — fingers obscure corners; offset is the production standard
- Client-side perspective warp to produce a deskewed image — users expect flat output, not a raw trapezoid photo
- Warped result preview with Re-crop and Accept buttons — all production scanners offer this confidence check
- Single-page mode and multi-page mode (Add Page / Done) — core capture flows
- Server-side PDF assembly from uploaded JPEG pages — callers expect a single PDF, not a zip of images
- Nested result structure `documents[].pages[]` — clean API contract regardless of single/multi page/document
- Go client library update with `CreateDocumentScanSession` and `GetDocumentScanResult` — feature parity with HTTP API

**Should have (competitive):**
- Offset magnifying glass during handle drag — touch fingers are 44 px wide; loupe is the standard iOS/Android crop pattern, rare in web implementations
- Multi-document mode (caller-controlled at session creation) — captures front+back of ID, invoice+attachment in one session
- Per-page image output option — callers doing OCR prefer raw images over PDF

**Defer (v2+):**
- Auto edge detection to pre-position crop handles — OpenCV.js WASM is 8 MB, poor on low-contrast documents; add as enhancement to manual crop
- Per-page thumbnail carousel and drag-to-reorder — useful UX, not blocking for first shipment
- Image filters (greyscale, contrast, shadow removal) — add when callers specifically request; browser quality without OpenCV is low

### Architecture Approach

The feature integrates into the existing codebase using a strict additive pattern: no existing routes change, no existing handler logic is modified, no existing model fields are renamed. New scan-specific logic branches off `ActionTypeScan` checks in `result_controller.go` and `session_page_controller.go`. The result model adds a separate `ScanResult []ScanDocument` field alongside the existing `Result []ResultItem`, both `omitempty`, so the JSON response is self-selecting based on action type. The scan UI template (`action_scan.html`) is entirely new and is the largest single unit of work; it implements a 5-state client-side machine (CAPTURE → CROP → PREVIEW → [multi-doc: NEXT_DOCUMENT] → SUBMIT). Build order is model → store/PDF utility → session controller → result controller → page controller → scan template → client library.

**Major components:**
1. `internal/model/session.go` — add `ActionTypeScan`, `ScanDocument`, `ScanResult []ScanDocument`, `MultiDocument bool`; update validation functions
2. `internal/server/result_controller.go` — branch on `ActionTypeScan`; parse nested submission; call `ImagesToPDF` for PDF output; call `MarkSessionCompletedScan`
3. `internal/web/templates/action_scan.html` — new; implements the full 5-state scan UI with Homography.js, canvas crop handles, magnifier, multi-page and multi-document state machine
4. `internal/util/pdf.go` — add `ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error)`; migrate import to `phpdave11/gofpdf`
5. `pkg/` Go client library — add `CreateDocumentScanSession` and `DocumentScanResult` type

### Critical Pitfalls

1. **EXIF orientation ignored by canvas `drawImage`** — iOS stores camera rotation only in EXIF; canvas renders raw pixels sideways. Read EXIF orientation tag (use `exifr` ~14 kB or manual JPEG parsing) and apply canvas rotation transform to normalize to upright before showing the crop UI.
2. **Perspective warp coordinate space mismatch** — display canvas is scaled down; handle positions are in display-space, not source-image space. Track `scale = drawWidth / img.naturalWidth` and offset when drawing; convert handle coordinates before feeding into the homography matrix. Ship test with a 12 MP photo.
3. **iOS Safari canvas memory limit crash** — a 12 MP photo decoded as RGBA is ~47 MB per canvas; 3–5 pages hits the Safari limit. Store captured pages as compressed JPEG blobs immediately via `canvas.toBlob()`; release canvases with `canvas.width = 0` after serialization; never hold decoded `ImageBitmap` or canvas references in the page array.
4. **Touch events scroll the page instead of moving handles** — Chrome 55+ adds `touchstart`/`touchmove` as passive by default. Register with `{ passive: false }` and call `preventDefault()`; set `touch-action: none` on the crop overlay via CSS.
5. **Result API backwards compatibility break** — replacing or aliasing the `items` field on the session result breaks all existing photo and signature callers. Use a separate `documents` field with `omitempty`; the `items` field must remain structurally unchanged for non-scan sessions.

## Implications for Roadmap

Based on research, the architecture's explicit build-order guidance and the pitfall-to-phase mapping suggest a 6-phase delivery structure.

### Phase 1: Model and API Contract

**Rationale:** The nested result structure and `ActionTypeScan` constant are dependencies for every other server-side change. The backwards-compatibility decision (separate `documents` field with `omitempty`) must be locked in before any other code is written — this is the highest-recovery-cost pitfall in the research.
**Delivers:** `ActionTypeScan` constant; `ScanDocument` type; `ScanResult []ScanDocument` on `Session`; `MultiDocument bool`; updated `ValidateActionType`/`ValidateOutputFormat`; `MarkSessionCompletedScan` on store; import migration to `phpdave11/gofpdf`
**Addresses:** Nested result structure (table stakes); multi-document mode (differentiator)
**Avoids:** Result API backwards compatibility break (Pitfall 5); blocked downstream development

### Phase 2: PDF Utility and Session Creation

**Rationale:** `ImagesToPDF` has no dependencies within the codebase and can be built and unit-tested in isolation before the submission handler that calls it. Session creation changes are minimal and unlock the ability to create test scan sessions. Both can proceed in parallel after Phase 1.
**Delivers:** `ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error)` in `internal/util/pdf.go`; `multi_document` param in `createSessionRequest`; updated session controller handler
**Uses:** `phpdave11/gofpdf` v1.4.3
**Avoids:** gofpdf multi-page RAM accumulation (Pitfall 10) — profile before adding page limit knob

### Phase 3: Submission Handler and Result Retrieval

**Rationale:** Depends on Phases 1 and 2 (model types and `ImagesToPDF`). This is the most complex server-side change: parse the nested submission JSON, decode pages, enforce `max_pages`, apply body size limit, call `ImagesToPDF` for PDF output, call `MarkSessionCompletedScan`, update `getResultHandler` to emit `documents` vs `items`.
**Delivers:** Full server-side scan submission handling; PDF and image output paths; body size limit enforcement; `max_pages` guard
**Addresses:** PDF output (table stakes); per-page image output (differentiator)
**Avoids:** In-memory store exhaustion (Pitfall 6); base64 + proxy 413 errors (Pitfall 9)

### Phase 4: Scan UI Template — Core Capture Flow

**Rationale:** The scan template is the largest single work unit and can be developed in parallel with Phases 2 and 3 against a running server with stubbed session IDs. Building the core single-page capture flow first (camera → crop → warp → preview → submit) establishes the base before multi-page and multi-document layers.
**Delivers:** `action_scan.html` template with CAPTURE → CROP → PREVIEW → SUBMIT states; Homography.js integration; 4-corner handles positioned outside corners; client-side perspective warp; warped preview with Re-crop/Accept
**Uses:** `Homography.js` v1.8.1 UMD bundle; Vanilla Canvas 2D + Pointer Events
**Avoids:** EXIF orientation ignored (Pitfall 1 — normalize before showing crop); coordinate space mismatch (Pitfall 3 — track scale factor from the start); touch scroll conflict (Pitfall 2 — `passive: false` + `touch-action: none`); pinch zoom conflict (Pitfall 8); `100vh` iOS Safari layout break (Pitfall 7)

### Phase 5: Scan UI Template — Multi-Page and Multi-Document

**Rationale:** Multi-page flow depends on the single-page flow being solid (per the feature dependency graph). Multi-document wraps multi-page. Both introduce the iOS Safari canvas memory limit risk, which is a multi-page-specific concern.
**Delivers:** Add Page / Done flow; document accumulator JS array with blob storage; multi-document state machine with "Next Document" boundary; page counter badge; submission serializer for nested JSON payload
**Addresses:** Multi-page mode (table stakes); multi-document mode (differentiator); offset magnifying glass (differentiator — add in this phase alongside handle interaction)
**Avoids:** iOS Safari canvas memory crash (Pitfall 4 — blob-based page array from the start, never raw canvas refs)

### Phase 6: Go Client Library Update

**Rationale:** The client library (`pkg/`) must have feature parity with the HTTP API. This is independent of the server-side phases and can be done in parallel with Phase 4/5, but must be complete before the milestone ships.
**Delivers:** `CreateDocumentScanSession` method; `DocumentScanResult` type; `GetDocumentScanResult` method; updated session model in `pkg/`
**Addresses:** Go client library update (table stakes)
**Avoids:** Missing `GetScanResult()` integration gotcha (noted in PITFALLS.md)

### Phase Ordering Rationale

- Model changes are a hard prerequisite for all server-side and client-side work (ActionTypeScan constant, result type) — Phase 1 must be first.
- PDF utility has no internal dependencies and benefits from isolated testing before integration — Phase 2 begins immediately after Phase 1.
- The submission handler depends on model types and `ImagesToPDF` — Phase 3 follows Phase 2.
- The scan UI template is the most independent piece of work and the longest; beginning it in Phase 4 (possibly overlapping Phase 2/3) keeps the critical path short.
- Multi-page and multi-document state build on the single-page flow — Phase 5 follows Phase 4.
- Client library has no internal dependencies and can be parallelized — Phase 6 can overlap Phases 4 and 5.

### Research Flags

Phases that will benefit from deeper research during planning:
- **Phase 4 (Scan UI Core):** The EXIF normalization approach, coordinate space conversion, and magnifier implementation each have multiple valid techniques. Request phase research before implementation planning to lock in the specific approach (exifr vs. manual parsing; `visualViewport` vs. `dvh` layout).
- **Phase 5 (Multi-page/Multi-document):** The blob-based page accumulator pattern and the exact submission serialization strategy (base64 JSON vs. multipart/form-data) need a firm decision before coding. The multipart alternative removes the base64 overhead and proxy size issue but requires server-side parsing changes.

Phases with standard, well-documented patterns (can skip deeper research-phase):
- **Phase 1 (Model and API Contract):** Pattern is identical to how `ActionTypePhoto` and `ActionTypeSignature` were added. Direct codebase analogy, no novel decisions.
- **Phase 2 (PDF Utility and Session Creation):** `gofpdf` multi-image loop is a mechanical extension of the existing single-image function. Import migration is one line.
- **Phase 3 (Submission Handler):** Handler branch pattern matches what already exists for photo vs. signature; the new part is the JSON struct tree and the `ImagesToPDF` call. Well-defined in ARCHITECTURE.md.
- **Phase 6 (Client Library):** New methods follow the existing builder pattern in `pkg/`. No novel architecture.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | MEDIUM | Homography.js v1.8.1 verified via npm; maintained status confirmed via GitHub. `phpdave11/gofpdf` v1.4.3 HIGH — release date and API compatibility verified directly. Vanilla Canvas 2D + Pointer Events HIGH — standard browser APIs. |
| Features | MEDIUM | Core UX patterns verified across Scanbot, Google ML Kit, and open-source implementations. Offset magnifier implementation relies on general Canvas 2D pattern knowledge rather than a direct reference implementation in this context. |
| Architecture | HIGH | Based on direct source inspection of every file listed. Component boundaries, data flow, and build order are derived from the actual codebase, not assumptions. |
| Pitfalls | HIGH | Browser quirks (EXIF, canvas memory, touch events, viewport) verified against MDN, WebKit bug tracker, Chrome developer documentation, and primary sources. Integration pitfalls verified against codebase source. |

**Overall confidence:** HIGH

### Gaps to Address

- **Homography.js UMD bundle format:** Version 1.8.1 confirmed via npm search result. Before committing to the library, download and verify the UMD bundle is suitable for `<script src>` delivery without a build step. Fallback: vendor the relevant matrix computation (~50 lines) directly into the template.
- **base64 vs. multipart submission for multi-page scans:** The research documents both options. The base64 extension of the existing pattern is simpler but risks proxy size limits and 33% overhead. The multipart alternative is more robust but requires server-side parsing changes. This decision should be made at Phase 3 planning time with an explicit call on the maximum page count.
- **`max_pages` default value:** Research recommends 10–20 pages but does not validate this against actual device JPEG sizes and server memory constraints. Profile a 10-page scan at realistic JPEG quality (0.85) on the target deployment to set the right default before shipping.
- **EXIF library choice:** Research recommends `exifr` (~14 kB). Verify bundle deliverability as a UMD asset (no build pipeline) before committing. Manual JPEG APP1 segment parsing is the fallback if `exifr` requires a module bundler.

## Sources

### Primary (HIGH confidence)
- Direct source inspection: `internal/model/session.go`, `internal/store/store.go`, `internal/server/*.go`, `internal/util/pdf.go`, `internal/ws/hub.go`, `internal/web/web.go`, `internal/web/templates/action_photo.html`
- [phpdave11/gofpdf GitHub](https://github.com/phpdave11/gofpdf) — v1.4.3 release date and API compatibility verified
- [jung-kurt/gofpdf GitHub](https://github.com/jung-kurt/gofpdf) — archived November 13, 2021 verified
- [MDN: HTMLCanvasElement.toBlob()](https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/toBlob) — native API, mobile support verified
- [MDN: Touch events](https://developer.mozilla.org/en-US/docs/Web/API/Touch_events) — passive listener model
- [Chrome for Developers: Making touch scrolling fast by default](https://developer.chrome.com/blog/scrolling-intervention) — passive event listener default
- [PQINA: Total Canvas Memory Use Exceeds The Maximum Limit](https://pqina.nl/blog/total-canvas-memory-use-exceeds-the-maximum-limit) — iOS Safari canvas memory limits
- [Apple Developer Forums: Total canvas memory use](https://developer.apple.com/forums/thread/112218) — iOS Safari canvas limit details

### Secondary (MEDIUM confidence)
- [homography npm package](https://www.npmjs.com/package/homography) — v1.8.1 from search result; UMD format from GitHub
- [Eric-Canas/Homography.js GitHub](https://github.com/Eric-Canas/Homography.js) — API surface, dependency-free status
- [Scanbot SDK RTU UI v2.0 release notes](https://scanbot.io/blog/document-scanner-rtu-ui-v2-release/) — multi-page UX patterns
- [Google ML Kit Document Scanner](https://developers.google.com/ml-kit/vision/doc-scanner) — single/multi page mode reference
- [jscanify](https://colonelparrot.github.io/jscanify/) — manual crop + warp flow reference implementation
- [HTMHell: Control the Viewport Resize Behavior](https://www.htmhell.dev/adventcalendar/2024/4/) — dvh unit and iOS Safari layout
- [Medium: Image Rotation Problem From Mobile Phone Camera](https://medium.com/@dollyaswin/image-rotation-problem-from-mobile-phone-camera-javascript-flutter-e38fcba58c5f) — EXIF orientation on mobile

### Tertiary (LOW confidence)
- [perspective-transform npm](https://www.npmjs.com/package/perspective-transform) — v1.1.3, last updated ~10 years ago; evaluated and rejected
- [perspective.js wanadev](https://github.com/wanadev/perspective.js) — v1.0.4, rect-to-quad only; evaluated and rejected

---
*Research completed: 2026-02-27*
*Ready for roadmap: yes*
