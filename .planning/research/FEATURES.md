# Feature Research

**Domain:** Browser-based document scanning action type (mobile-first, touch UI)
**Researched:** 2026-02-27
**Confidence:** MEDIUM — Core UX patterns verified across multiple SDKs and implementations; canvas/homography feasibility verified via library discovery; some specifics (offset magnifier interaction model) rely on general canvas pattern knowledge rather than a direct reference implementation.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Camera capture via `<input type="file" capture="environment">` | Standard mobile web camera trigger; every scanner app uses this | LOW | Already proven in existing photo action. Reuse the same input pattern. |
| Display captured image for crop review | Users cannot crop what they cannot see; no scanner skips this step | LOW | Render captured image to a `<canvas>` element before crop interaction begins. |
| 4-corner crop handles on the image | All major scanner apps (Google Drive, MS Lens, iOS, Scanbot) expose editable quadrilateral corners after capture | MEDIUM | Handles must be individually draggable. Initial positions default to image corners or a centred inset. |
| Handles positioned outside the crop area | Fingers obscure the corner when placed directly on it; every production scanner offsets handles | MEDIUM | Handle hit target sits outside the document corner by ~24–32px. A connecting line is drawn between handle and actual corner point. |
| Perspective warp (client-side) | Users expect a flat, deskewed output — not a raw trapezoid photo | HIGH | Use a JavaScript homography library (e.g. `perspective-transform` or `Homography.js`) against a `<canvas>` drawImage call. CPU-bound; runs once per page on submit. |
| Preview of corrected result | All production scanners show the warped result before final submission; users need confidence | LOW | Render the warped canvas output as a preview image. Offer "Re-crop" (back to crop UI) and "Accept" buttons. |
| Re-crop from preview | Users catch perspective errors only when they see the result; re-entry into crop UI is standard | LOW | "Re-crop" button navigates back to crop step without retaking the photo. Existing image stays in memory. |
| Upload to server on accept | Page data must reach the backend before proceeding | LOW | POST the canvas blob (JPEG) to the session endpoint. Reuse the existing form submission pattern from photo action. |
| Session completion signal | Caller needs to know the session is done (existing WebSocket/polling) | LOW | Already implemented. Document scan action plugs into the same session lifecycle. |

### Differentiators (Competitive Advantage)

Features that set Handoff apart. Not required by baseline expectations, but high value for the stated use case.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Offset magnifying glass at corner position | Touch fingers are ~44px wide; users cannot see the exact corner under their fingertip. Offset loupe (positioned above or to the upper-left of the finger) solves this cleanly — it is the standard iOS/Android crop pattern but rare in web implementations | HIGH | Implement as a second canvas element or an absolutely-positioned `<div>` with `overflow: hidden`. On `touchmove`, draw a zoomed excerpt of the image centred on the actual corner position. Offset the loupe 80–100px above the touch point so it is never obscured. |
| User-selectable single vs multi-page mode | Gives the user control over how many pages to capture within one document; the caller does not need to predict this | MEDIUM | Offer a toggle or mode-selection screen before the first capture. Single-page: one capture → crop → submit. Multi-page: after accepting each page, show "Add Page" and "Done" buttons. |
| Multi-document mode (caller-controlled) | Lets a single session capture multiple separate documents (e.g. front + back of ID, invoice + attachment) — caller specifies at session creation | MEDIUM | After the user completes one document, show a "Next Document" button. Each document boundary is explicit. Result structure is `documents[].pages[]`. |
| Nested result structure (documents → pages) | Clean API contract — callers always get the same shape regardless of single/multi page or document mode. Single-page single-document is just `documents[0].pages[0]` | MEDIUM | Define a canonical result type. Server-side Go types + client library types both use this structure. Simplifies caller code significantly. |
| PDF output per document (server-side assembly) | Callers who receive multi-page documents expect a single PDF, not a zip of images — this matches how Scanbot, Google Drive, and iOS Scan all deliver results | MEDIUM | Assemble PDF server-side in Go (e.g. `unidoc/unipdf` or `jung-kurt/gofpdf`) from the ordered page images uploaded by the client. Client uploads JPEG pages; server assembles the PDF. PDF generation stays server-side to avoid large client-side JS bundle. |
| Per-page image output option | Callers doing OCR or their own PDF assembly prefer raw images without the PDF overhead | LOW | Already implied by the format selector at session creation. Store page images and return them directly when format is `image`. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Auto edge detection (OpenCV.js / ML-based) | Looks impressive; would remove need for manual crop | OpenCV.js WASM binary is 7–12 MB; loads slowly on mobile. Auto-detection accuracy varies badly on low-contrast documents, coloured backgrounds, and shadows — creates user-trust issues when detection is wrong. PROJECT.md explicitly defers this. | Manual crop with offset magnifying glass. Accurate, fast to load, no trust issues. Auto-detect can be added later as an enhancement to pre-position the handles. |
| Drag-to-reorder pages in the browser | Common in native app scanners (Scanbot RTU UI v2) | Significant interaction complexity for touch (long-press, drag-handle UX). No payoff for v1.1 — page order is capture order, which is natural. | Server-side page reordering could be added as a separate API endpoint if callers need it. |
| Client-side PDF generation (jsPDF) | Reduces server round-trips | jsPDF multipage image PDFs are large, poorly compressed, and the library adds ~200 KB to the page bundle. PDF quality is lower than server-side generation. | Server assembles PDF from uploaded JPEG pages using Go. Clean separation: client handles capture and warp, server handles assembly. |
| Live video perspective overlay (real-time detection) | "Smart" preview while aiming camera | Requires continuous frame processing in JS. Degrades on mid-range Android. No user benefit for a deliberate capture flow — they take the photo first, then crop. | Still-image crop UI after capture. Zero performance cost. |
| Configurable filter/enhancement (greyscale, contrast, shadow removal) | Found in Scanbot and Adobe Scan | Each filter is a separate canvas pass; UI complexity is high. Quality of browser-based shadow removal is low without OpenCV. Adds decision fatigue for users. | Defer. The immediate output (perspective-corrected colour JPEG) covers 90% of use cases. Add filters as a v1.2 enhancement if callers request it. |

---

## Feature Dependencies

```
[Camera capture]
    └──requires──> [Image display on canvas]
                       └──requires──> [4-corner crop handles]
                                          └──requires──> [Perspective warp]
                                                             └──requires──> [Warped preview]
                                                                                └──requires──> [Upload to server]

[Offset magnifying glass] ──enhances──> [4-corner crop handles]
    (precision aid, same interaction, no new flow state)

[User-selectable page mode] ──determines──> [Multi-page capture loop]
    └── [Multi-page loop]
            └──requires──> [Per-page: Camera capture → crop → warp → preview → accept/retake]
            └──accumulates──> [Page list]
                                  └──requires──> [Upload (all pages)]

[Caller-controlled multi-document mode] ──wraps──> [Multi-page capture loop]
    └── [Document list]
            └──requires──> ["Next Document" boundary UI]
            └──requires──> [Nested result structure (documents → pages)]

[PDF output] ──requires──> [All pages uploaded]
    └──assembled by──> [Server-side PDF generation]
    └──conflicts with──> [Client-side PDF generation]

[Image output] ──requires──> [All pages uploaded]
    └──simpler than──> [PDF output] (no assembly step)
```

### Dependency Notes

- **Camera capture requires image display on canvas:** The raw file from `<input capture>` must be decoded and drawn to a canvas before any crop interaction can happen.
- **Perspective warp requires 4 corner points:** The warp has no meaningful input until the user has positioned all four handles. Warp runs once on user confirmation, not continuously.
- **Offset magnifying glass enhances crop handles:** It operates in the same drag-handler event; no additional flow state, no new screen. It can be added or removed without breaking the crop flow.
- **Multi-page loop requires the per-page flow to be complete:** The loop is simply: repeat (capture → crop → warp → preview → accept) until user taps "Done". The per-page flow must be solid before implementing the loop.
- **Multi-document mode wraps multi-page:** A document boundary is just a "flush and start new document" event inside the session. Multi-document cannot exist without multi-page flow being defined first.
- **PDF output conflicts with client-side PDF generation:** Do not put both paths in. Pick one. Server-side wins — see anti-features.

---

## MVP Definition

This is a subsequent milestone (v1.1), not a greenfield MVP. "Launch with" means: the document scanning action is usable and ships.

### Launch With (v1.1 core)

- [x] Camera capture for document photo — reuses existing `<input type="file" capture="environment">` pattern
- [x] Image rendered to canvas for crop interaction
- [x] Draggable 4-corner handles, positioned outside the crop area
- [x] Offset magnifying glass on handle drag (touch precision)
- [x] Client-side perspective warp using a lightweight homography library
- [x] Warped result preview with Re-crop and Accept buttons
- [x] Single-page mode (one page per document, user captures once)
- [x] Multi-page mode (user can add pages; "Add Page" / "Done" buttons)
- [x] Single-document mode (caller default)
- [x] Multi-document mode (caller specifies at session creation)
- [x] Nested result structure: `documents[].pages[]` always
- [x] Output format: PDF (server-side assembly from JPEG pages)
- [x] Output format: individual page images (JPEG)
- [x] Plugs into existing session lifecycle (opened → action_started → completed → expired)
- [x] Go client library updated to support document scan session creation and result retrieval

### Add After Validation (v1.x)

- [ ] Auto edge detection to pre-position crop handles — add once manual crop UX is validated; use WASM OpenCV or a lighter alternative
- [ ] Per-page thumbnail carousel in multi-page review — useful UX but not blocking for first shipment
- [ ] Drag-to-reorder pages — add if callers report needing it
- [ ] Image filters (greyscale, contrast) — add when callers specifically request it

### Future Consideration (v2+)

- [ ] Auto-capture on detection (continuous video mode) — requires rethinking the capture flow entirely
- [ ] OCR integration — out of scope for Handoff; callers do their own OCR on received images
- [ ] Embeddable Vue component — already listed as deferred in PROJECT.md

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Camera capture (reuse existing) | HIGH | LOW | P1 |
| 4-corner crop handles | HIGH | MEDIUM | P1 |
| Handles outside crop area | HIGH | LOW | P1 |
| Offset magnifying glass | HIGH | MEDIUM | P1 |
| Client-side perspective warp | HIGH | MEDIUM | P1 |
| Warped result preview | HIGH | LOW | P1 |
| Re-crop from preview | HIGH | LOW | P1 |
| Single-page capture mode | HIGH | LOW | P1 |
| Multi-page capture mode | HIGH | MEDIUM | P1 |
| Multi-document mode | MEDIUM | MEDIUM | P1 |
| Nested result structure | HIGH | LOW | P1 |
| PDF output (server-side) | HIGH | MEDIUM | P1 |
| Per-page image output | MEDIUM | LOW | P1 |
| Go client library update | HIGH | LOW | P1 |
| Auto edge detection | MEDIUM | HIGH | P3 |
| Drag-to-reorder pages | LOW | HIGH | P3 |
| Image filters | LOW | MEDIUM | P3 |

---

## Existing Feature Dependencies (from v1.0)

The document scanning action type must integrate with, not replace, these already-shipped features:

| v1.0 Feature | How Document Scan Uses It |
|--------------|--------------------------|
| Session lifecycle (create → opened → action_started → completed → expired) | Document scan plugs in as a new action type; state machine is identical |
| Intro page with optional explanation text | Session creation can specify intro text for document scanning instructions |
| Result retrieval via polling and WebSocket | No change needed; result payload structure is different (nested) but delivery mechanism is the same |
| API key authentication | No change; document scan sessions are created via the same protected API |
| Go client library (`pkg/`) | Needs new method for `CreateDocumentScanSession` and a `DocumentScanResult` type |
| Server-rendered HTML templates | New templates required: crop UI, multi-page flow, multi-document flow |
| Embedded static assets | JS libraries (homography, any magnifier helper) must be embedded via `embed.FS` — no CDN |

---

## Sources

- [Scanbot SDK RTU UI v2.0 release notes](https://scanbot.io/blog/document-scanner-rtu-ui-v2-release/) — multi-page review flow, page management UX patterns (MEDIUM confidence)
- [Google ML Kit Document Scanner](https://developers.google.com/ml-kit/vision/doc-scanner) — reference for single/multi page modes, crop screen design (MEDIUM confidence)
- [Homography.js GitHub](https://github.com/Eric-Canas/Homography.js) — lightweight perspective transform library confirmed viable for browser (MEDIUM confidence)
- [perspective-transform npm](https://github.com/jlouthan/perspective-transform) — minimal JS homography matrix library, battle-tested (MEDIUM confidence)
- [jsPDF GitHub](https://github.com/parallax/jsPDF) — client-side PDF evaluated and rejected for server-side approach (HIGH confidence for anti-feature reasoning)
- [jscanify](https://colonelparrot.github.io/jscanify/) — reference implementation of manual crop + warp flow in vanilla JS (MEDIUM confidence)
- [HTML5 Canvas magnifying glass — Creative Bloq](https://www.creativebloq.com/netmag/how-create-digital-magnifying-glass-effect-31411047) — canvas drawImage pattern for loupe effect (MEDIUM confidence)
- [Scanbot SDK Web RTU UI](https://docs.scanbot.io/web/document-scanner-sdk/ready-to-use-ui/introduction/) — reference for what complete web-based scanning flows look like (MEDIUM confidence)
- [Box Blog: Building a Mobile Document Scanner](https://blog.box.com/building-document-scanning) — production mobile scanning architecture reference (MEDIUM confidence)

---

*Feature research for: Handoff v1.1 Document Scanning action type*
*Researched: 2026-02-27*
