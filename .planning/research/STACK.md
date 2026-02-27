# Stack Research

**Domain:** Document scanning with manual crop, perspective warp, multi-page capture, PDF assembly
**Researched:** 2026-02-27
**Confidence:** MEDIUM (JavaScript library maintenance uncertain; Go PDF library upgrade verified HIGH)

---

## Context: What Already Exists

These capabilities are validated and must NOT be changed:

| Component | Current Implementation | Location |
|-----------|----------------------|----------|
| Go HTTP server | Gin v1.11.0 | `internal/server/` |
| In-memory session cache | `patrickmn/go-cache` v2.1.0 | `internal/server/session_*.go` |
| PDF generation | `jung-kurt/gofpdf/v2` v2.17.3 | `internal/util/pdf.go` |
| Static assets | `go:embed` HTML/JS/CSS | `internal/web/html/public/` |
| JS delivery pattern | UMD bundle dropped into `/public/` | `signature_pad.umd.min.js` |
| Form submission | JSON body with base64-encoded file data | `action_photo.html` script |

**Constraint:** No npm build pipeline. All JS must be single-file UMD bundles served as static assets and embedded in the Go binary via `go:embed`.

---

## Stack Additions Needed

### 1. Client-Side Perspective Warp

**Recommended:** `homography` (Homography.js) v1.8.1 by Eric-Canas

| Property | Value |
|----------|-------|
| Package | `homography` on npm |
| Version | 1.8.1 (latest as of research date) |
| License | MIT |
| Bundle file | `Homography.js` (~115 kB unminified; lightweight variant available) |
| Dependencies | None |
| Browser delivery | UMD bundle via jsDelivr or self-hosted |

**Why this over alternatives:**

- `perspective-transform` (jlouthan) is v1.1.3, last updated ~10 years ago, unmaintained. It only computes the matrix coefficients — you still implement the per-pixel warp yourself using `getImageData`/`putImageData`. Given the project already uses `signature_pad.umd.min.js` as a self-contained bundle, replacing dead code risk with an actively maintained library is correct.
- `perspective.js` (wanadev) v1.0.4 is ~22 kB and only maps a rectangle to a quad — it cannot take an arbitrary quad as source (the document in the photo). That is the exact operation needed: quad-to-rect warp.
- `opencv.js` is ~8 MB and inappropriate for a minimal-JS embedded server. Ruled out entirely.
- **Homography.js handles quad-to-quad projective transforms** (the exact operation: map 4 user-placed corners on the captured image to a rectangle). It returns `ImageData` for `putImageData` onto a canvas. It has no dependencies, MIT license, and is actively maintained at v1.8.1.

**Integration:** Download `Homography.js` UMD build, place in `internal/web/html/public/`, reference from `action_scan.html` template via `<script src="/static/Homography.js">`. No build step.

**Confidence:** MEDIUM — version 1.8.1 confirmed via npm search result; bundle format confirmed as UMD; dependency-free confirmed. Maintenance activity not independently verified via official source.

---

### 2. Touch-Friendly Crop Handles + Magnifying Glass

**Recommended:** Vanilla Canvas 2D API — no library needed.

The crop UI (4 draggable corner handles with an offset magnifying glass) is custom interaction logic specific to this product's UX. No library fits this pattern without bringing more complexity than it solves:

- `interact.js` is a general drag-and-drop library (~50 kB). Its abstraction is wrong here: the handles must be rendered ON the canvas, not as DOM elements, because they overlay a camera frame being drawn to canvas.
- `cropperjs` is DOM-image-only with a fixed rectangular crop region. Cannot do perspective quad selection.

**Implementation approach:**

- Render camera capture in a `<canvas>` element
- Draw 4 corner handle circles directly on canvas (not DOM elements)
- Use `pointermove`/`pointerdown`/`pointerup` events (unified mouse+touch, no library needed)
- Handle hit-testing manually (`Math.hypot(px - hx, py - hy) < radius`)
- Magnifying glass: second `<canvas>` element (or off-screen canvas) that calls `drawImage(sourceCanvas, ...)` with a scaled/clipped region centered on the active corner. This is a well-documented Canvas 2D pattern requiring no library.

**Why no library:** The touch event model is 3 events (`pointerdown`, `pointermove`, `pointerup`). The interaction is ~80 lines of vanilla JS. Adding a drag library for this would introduce a dependency for code you could write in an afternoon.

**Confidence:** HIGH — all APIs used are standard browser Canvas 2D and Pointer Events, both with full mobile browser support.

---

### 3. Multi-Page Image Handling in Browser

**Recommended:** Vanilla JS with an in-memory array — no library needed.

Pages are accumulated in a JavaScript array before submission. Each page is a canvas that has been warped. The "Add Page" / "Next Document" flow is pure state machine logic:

```javascript
// Pseudocode — no library required
const pages = [];  // [{blob, dataURL}, ...]

function addPage(warpedCanvas) {
  warpedCanvas.toBlob(blob => {
    pages.push({ blob, dataURL: URL.createObjectURL(blob) });
    renderThumbnails();
  }, 'image/jpeg', 0.92);
}
```

`canvas.toBlob(callback, 'image/jpeg', quality)` is a native browser API with full mobile support. No library needed.

**Confidence:** HIGH — standard browser API.

---

### 4. Multi-Page PDF Assembly — Server Side

**Recommended:** Migrate `internal/util/pdf.go` from `jung-kurt/gofpdf/v2` to `phpdave11/gofpdf` v1.4.3

| Property | Value |
|----------|-------|
| Package | `github.com/phpdave11/gofpdf` |
| Version | v1.4.3 (released May 1, 2025) |
| License | MIT |
| API compatibility | Drop-in replacement for jung-kurt/gofpdf |
| Dependencies | None beyond Go stdlib |

**Why migrate:**

`jung-kurt/gofpdf` was archived by its owner on November 13, 2021 and has not been updated since. The `go.mod` currently references it as an indirect dependency via `jung-kurt/gofpdf/v2`. The `phpdave11/gofpdf` fork is the community-maintained continuation, with v1.4.3 released May 2025. For a single-page PDF, the risk of staying on the archived version is low, but the document scanning milestone requires multi-page PDF assembly — new code being written should target the maintained fork.

**Migration scope:** Only `internal/util/pdf.go` (62 lines). API is identical: same `gofpdf.New()`, `AddPage()`, `RegisterImageOptionsReader()`, `ImageOptions()` calls. Change the import path. Rename the module dependency in `go.mod`.

**Confidence:** HIGH — archival status verified via GitHub; v1.4.3 release date verified via GitHub fetch; API compatibility confirmed (it is a direct fork, not a rewrite).

**Multi-page assembly pattern** (no change from existing single-page pattern, just loop):

```go
func ImagesToPDF(images []ImageInput) ([]byte, error) {
    pdf := gofpdf.NewCustom(&gofpdf.InitType{...})
    for i, img := range images {
        pdf.AddPage()
        pdf.RegisterImageOptionsReader(fmt.Sprintf("img%d", i), ...)
        pdf.ImageOptions(...)
    }
    var buf bytes.Buffer
    pdf.Output(&buf)
    return buf.Bytes(), nil
}
```

---

### 5. Image Upload from Browser to Go Server

**Recommended:** JSON body with per-page base64 items — extend existing pattern.

The existing `action_photo.html` already sends:

```json
{
  "items": [{"content_type": "image/jpeg", "filename": "photo.jpg", "data": "<base64>"}]
}
```

For multi-page document scanning, extend this to a nested structure:

```json
{
  "documents": [
    {
      "pages": [
        {"content_type": "image/jpeg", "filename": "page1.jpg", "data": "<base64>"},
        {"content_type": "image/jpeg", "filename": "page2.jpg", "data": "<base64>"}
      ]
    }
  ]
}
```

This matches what the server already decodes and avoids any new upload mechanism. The base64 overhead (~33%) is acceptable for typical document scans at 1–3 MB JPEG.

**Alternative considered:** `FormData` with binary blobs. Rejected because the existing submission pattern is JSON, changing to multipart would require server-side parsing changes beyond the PDF assembly work, and JSON with base64 is simpler to handle in the Go `ScanResult` struct.

**Confidence:** HIGH — reuses existing, validated submission pattern.

---

## Recommended Stack Delta

| Addition | Technology | Why |
|----------|------------|-----|
| Perspective warp JS | `homography` v1.8.1 UMD bundle | Only active library that does quad-to-quad projective warp without dependencies |
| Crop + magnifier UI | Vanilla Canvas 2D + Pointer Events | 3 events, ~80 lines, no library fits the canvas-rendered-handle pattern anyway |
| Multi-page state | Vanilla JS array + `canvas.toBlob()` | Native browser API, no abstraction adds value |
| Go PDF library | `phpdave11/gofpdf` v1.4.3 | Drop-in replacement for archived `jung-kurt/gofpdf`; needed for multi-page assembly |
| Upload protocol | Extend existing JSON+base64 | Consistent with photo action pattern, no new server parsing |

---

## What NOT to Add

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `opencv.js` | ~8 MB bundle, heavyweight for what is needed | `homography` for the warp; manual corner placement eliminates auto-detection need |
| `perspective-transform` (jlouthan) | Unmaintained v1.1.3 (~10 years old), only computes coefficients, not the warp | `homography` |
| `cropperjs` | DOM-only, rectangular crop, no perspective | Vanilla canvas with custom handles |
| `interact.js` | DOM drag abstraction, wrong model for canvas-rendered handles | Native Pointer Events |
| `fabric.js` | 900 kB, entire canvas framework for a single interaction | Vanilla canvas |
| Client-side PDF generation (`jspdf`) | Adds ~250 kB JS, duplicates Go PDF logic, inconsistent output quality | Server-side assembly via `phpdave11/gofpdf` |
| `jung-kurt/gofpdf/v2` (new code) | Archived since 2021 | `phpdave11/gofpdf` v1.4.3 |

---

## Integration Points

### Go: `go.mod` change

```bash
# Remove:
github.com/jung-kurt/gofpdf/v2 v2.17.3

# Add:
github.com/phpdave11/gofpdf v1.4.3
```

Then update the import in `internal/util/pdf.go`:
```go
// Before:
import "github.com/jung-kurt/gofpdf/v2"

// After:
import "github.com/phpdave11/gofpdf"
```

### JS: Asset delivery

```
internal/web/html/public/
├── signature_pad.umd.min.js   (existing)
└── Homography.js              (new — download from jsDelivr or npm)
```

Reference in the `action_scan.html` template:
```html
<script src="/static/Homography.js"></script>
```

### Template structure

New template `action_scan.html` follows the `action_photo.html` pattern with three `{{define}}` blocks: `styles`, `content`, `scripts`. No new Go template machinery required.

---

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| `homography` v1.8.1 | All modern mobile browsers (iOS Safari 14+, Chrome Android 90+) | Uses Canvas 2D `putImageData`, Pointer Events — both widely supported |
| `phpdave11/gofpdf` v1.4.3 | Go 1.18+ | Repo states go 1.18 minimum; project uses Go 1.25 — no conflict |
| `canvas.toBlob()` | iOS Safari 11+, Chrome 50+ | Full mobile coverage; no polyfill needed |
| Pointer Events API | iOS Safari 13+, Chrome Android 55+ | Covers target device range; replaces touchstart/touchmove/touchend |

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `homography` v1.8.1 | `perspective-transform` v1.1.3 | Only if bundle size is critical and you are willing to implement the pixel-loop yourself |
| `phpdave11/gofpdf` | `gopdf` (signintech) | If Unicode font support or TTF embedding becomes a requirement |
| `phpdave11/gofpdf` | `unipdf` (UniDoc) | If PDF reading, form filling, or digital signatures are needed — overkill for image-to-PDF |
| Vanilla Pointer Events | `hammer.js` | If multi-finger gesture recognition (pinch zoom) is needed on the crop UI |
| Server-side PDF | `jspdf` (client-side) | If offline operation or zero-latency PDF preview is a hard requirement |

---

## Sources

- [homography npm package](https://www.npmjs.com/package/homography) — version 1.8.1 confirmed, MIT license (MEDIUM confidence: version from search result, not direct pkg.go.dev equivalent)
- [Eric-Canas/Homography.js GitHub](https://github.com/Eric-Canas/Homography.js) — API surface, dependency-free status, warp via `putImageData`
- [phpdave11/gofpdf GitHub](https://github.com/phpdave11/gofpdf) — v1.4.3 released May 1, 2025, actively maintained (HIGH confidence: verified via direct GitHub fetch)
- [jung-kurt/gofpdf GitHub](https://github.com/jung-kurt/gofpdf) — archived November 13, 2021, read-only (HIGH confidence: verified via direct GitHub fetch)
- [perspective-transform npm](https://www.npmjs.com/package/perspective-transform) — v1.1.3, last updated ~10 years ago, minimal maintenance (MEDIUM confidence: from search results)
- [perspective.js wanadev](https://github.com/wanadev/perspective.js) — v1.0.4, 22 kB, rect-to-quad only (MEDIUM confidence: from search results)
- [MDN HTMLCanvasElement.toBlob()](https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/toBlob) — native API, no polyfill needed for target devices
- [Canvas 2D Pointer Events MDN](https://developer.mozilla.org/en-US/docs/Games/Techniques/Control_mechanisms/Mobile_touch) — standard touch/pointer event handling

---

*Stack research for: Handoff v1.1 Document Scanning*
*Researched: 2026-02-27*
