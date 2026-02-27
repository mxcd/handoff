# Pitfalls Research

**Domain:** Document scanning with manual crop UI, perspective transform, multi-page management, and nested result API — added to an existing Go/HTML mobile-first session server
**Researched:** 2026-02-27
**Confidence:** HIGH (integration pitfalls verified against existing codebase; browser quirks verified against official MDN, WebKit bug tracker, and primary sources)

---

## Critical Pitfalls

### Pitfall 1: EXIF Orientation Ignored by Canvas drawImage

**What goes wrong:**
A phone camera encodes rotation in JPEG EXIF metadata (e.g., Orientation = 6 = 90 degrees clockwise). When the photo is loaded into an `<img>` element or drawn via `canvas.drawImage()`, browsers do NOT apply the EXIF rotation — they render the raw pixel data. The result: the crop UI displays a sideways or upside-down image, and the user places crop corners on a rotated version. Even if the crop math is correct, the final warped output is wrong.

**Why it happens:**
Developers assume what they see in the gallery (where the OS applies EXIF rotation) is what the browser renders. Browsers apply CSS `image-orientation: from-image` on `<img>` tags since Chrome 81 / Firefox 26, which corrects the display — but `canvas.drawImage()` draws raw pixels, ignoring the CSS property. The perspective warp operates on canvas pixel coordinates, not on the CSS-displayed orientation.

**How to avoid:**
Read the EXIF orientation tag from the captured File using a small library (e.g., `exifr` — ~14KB, no deps) or parse the JPEG SOF/APP1 segment manually. Before drawing the image to the crop canvas, apply the necessary canvas rotation transform to normalize orientation to "upright". This normalization step must happen before the crop UI is shown and before the perspective warp is applied.

**Warning signs:**
- On Android (common phone orientations), photos appear upright because Android often bakes rotation into pixels. On iOS, rotation is stored only in EXIF. Test specifically with photos taken in portrait mode on an iPhone.
- If the preview looks correct on desktop (dragging an existing JPEG) but wrong after camera capture on iOS, EXIF is the culprit.

**Phase to address:**
The phase that implements the crop UI. Must be the very first step after the user selects the photo — normalize orientation before displaying the crop overlay.

---

### Pitfall 2: Touch Events Scroll the Page Instead of Moving Crop Handles

**What goes wrong:**
When a user drags a crop corner handle, `touchmove` fires. If `preventDefault()` is not called on the event, the browser interprets the gesture as a scroll and moves the page instead of (or in addition to) moving the handle. The handle jumps unpredictably and the user cannot position it precisely.

**Why it happens:**
Since Chrome 55, `touchstart` and `touchmove` listeners are added as passive by default to improve scroll performance. Passive listeners cannot call `preventDefault()`, so calling it has no effect, and the browser scrolls anyway.

**How to avoid:**
Register touch listeners with `{ passive: false }` explicitly:
```javascript
handle.addEventListener('touchmove', onHandleMove, { passive: false });
```
Inside the handler, call `event.preventDefault()` before any coordinate math. Also set `touch-action: none` on the handle elements via CSS — this tells the browser before any JS runs that touch gestures on these elements should not trigger scrolling:
```css
.crop-handle { touch-action: none; }
```
The crop canvas/overlay container should also use `touch-action: none` if the entire surface is an interaction zone.

**Warning signs:**
- Testing only on desktop (mouse events behave differently — no passive scroll conflict).
- Handles appear to "snap back" to original position after drag.
- Page scrolls when user tries to drag handles.

**Phase to address:**
The phase that implements the crop handle interaction. This must be verified on a real iOS Safari and Android Chrome device, not in browser devtools device emulation.

---

### Pitfall 3: Perspective Warp Coordinate System Mismatch

**What goes wrong:**
The user positions 4 corner handles on a canvas that displays the image scaled to fit the screen. The pixel coordinates of the handles are in CSS/display space, not in the source image's native pixel space. When these display-space coordinates are fed directly into the perspective transform matrix (e.g., via `perspective-transform` library or manual homography calculation), the warp produces a cropped region with wrong scale, position, or aspect ratio.

**Why it happens:**
The image is drawn to the crop canvas via `drawImage`, which scales the image to fit. The displayed image may be 400×600 pixels on screen but the source image is 3024×4032 (phone camera). Handle positions are in display-space pixels (0–400, 0–600). The perspective warp must operate in source-image pixel space (0–3024, 0–4032).

**How to avoid:**
Track the scaling factor and offset applied when drawing the image to the crop canvas. When the image is fitted via `drawImage(img, offsetX, offsetY, drawWidth, drawHeight)`, store the scale factor: `scale = drawWidth / img.naturalWidth`. Convert handle positions before warping: `srcX = (handleX - offsetX) / scale`, `srcY = (handleY - offsetY) / scale`. Perform the perspective warp at full source resolution. Downscale the output only after warping.

**Warning signs:**
- Warped output looks correct on a small test image but wrong on a full-resolution phone photo.
- The output document is a sliver or misaligned.
- Works in desktop browser where test images are small, fails on device with 12MP photos.

**Phase to address:**
The phase implementing the perspective warp. Unit test the coordinate transform separately with a known input/output pair at a fixed scale factor before integrating with the live crop UI.

---

### Pitfall 4: iOS Safari Canvas Memory Limit Crashes the Page

**What goes wrong:**
When multiple full-resolution phone photos are loaded into canvas elements (for crop UI, warp output, and preview), iOS Safari hits its canvas memory limit and silently fails or crashes the tab. The error message in older iOS versions is "Total canvas memory use exceeds the maximum limit." Canvas operations silently produce blank output. In severe cases, Safari kills the tab.

**Why it happens:**
A 12MP photo (4032×3024) decoded as RGBA takes 4032 × 3024 × 4 = ~47MB per canvas. In a multi-page capture flow, keeping 3–5 captured pages plus a crop canvas plus an output canvas simultaneously in memory means 200–300MB of canvas memory — which exceeds the limit on older/lower-end iPhones. (Note: Safari 174+ lifted the artificial 1/4 RAM limit, but this only helps on newer iOS. Older devices and older iOS versions are unaffected.)

**How to avoid:**
- Keep only one canvas in existence at a time during the crop flow. Release previous canvases with `canvas.width = 0` before allocating a new one (this forces the browser to release GPU memory backing).
- Revoke object URLs with `URL.revokeObjectURL()` as soon as the image is drawn to the canvas.
- Downscale images before displaying in the crop UI. A 1500-pixel-wide display canvas is sufficient for manual corner placement — there is no benefit to rendering a 4032-wide image in the crop overlay.
- Store captured pages as compressed JPEG blobs (via `canvas.toBlob('image/jpeg', 0.85)`) rather than keeping raw ImageData or large object URLs in an array.
- The full-resolution warp should be performed in a single canvas, then immediately serialized to blob and the canvas released.

**Warning signs:**
- Works on a single page but fails when user adds a second or third page.
- Works on modern iPhone but fails on iPhone X or older.
- Canvas `getContext('2d')` returns null (silent failure when memory is exhausted).

**Phase to address:**
The phase implementing multi-page state management. Design the page array as blobs from the start — never store decoded ImageBitmap or canvas references in the page array.

---

### Pitfall 5: Result API Response Shape Break for Existing Callers

**What goes wrong:**
The current result API returns `{ "status": "completed", "items": [ ... ] }` where `items` is a flat array of `ResultItem`. The document scanning milestone introduces a nested structure: documents → pages. If the existing `items` field is replaced with a `documents` array of objects containing `pages` arrays, all existing callers (photo, signature action types) break immediately — their result parsing code gets `undefined` where they expected an array.

**Why it happens:**
It is tempting to redesign the result shape to be "correct" for the new use case and apply it uniformly. The existing `[]ResultItem` in `model.Session` is simple and doesn't accommodate the nested structure, so the reflex is to change it.

**How to avoid:**
Preserve the existing `items` field for `photo` and `signature` action types. Add a separate `documents` field to the result response that is only populated for `scan` action type sessions. The Go `Session` model should have both fields, using `omitempty` so non-scan sessions never see the new field:
```go
Result    []ResultItem     `json:"items,omitempty"`     // photo, signature
Documents []ScannedDocument `json:"documents,omitempty"` // scan
```
The `getResultHandler` selects which field to populate based on `session.ActionType`. Existing callers reading `items` are unaffected. The Go client library (`pkg/`) will need a new method for reading scan results, but the old methods remain valid.

**Warning signs:**
- Any refactor that touches `model.ResultItem` or renames the `Result` field on `Session`.
- Any change to `submitResultHandler` that restructures the response body for all action types.

**Phase to address:**
The phase that defines the scan result model and submission endpoint. The model design decision must be made before writing any scan submission or result retrieval code.

---

### Pitfall 6: In-Memory Store Memory Exhaustion with Multiple Large Image Files

**What goes wrong:**
A multi-page, multi-document scan session may generate 10–20 warped page images at ~500KB–2MB each (compressed JPEG), plus one or more assembled PDFs. All of these are stored as `[]byte` in `go-cache` (the `files` cache). Under concurrent load — even modest, since each session can produce ~20MB of binary data — the process RSS climbs quickly. The `go-cache` file store has a 5-minute default expiry and no size cap.

**Why it happens:**
The existing store was designed for single-file results (one photo, one signature). Multiplying by 10–20 files per session is a qualitative change. `go-cache` stores pointers to byte slices; Go's GC will not free them until they expire and the cleanup goroutine removes the entries. Under load, many in-flight sessions each hold their full file sets in memory simultaneously.

**How to avoid:**
- In the session creation request, add an optional `max_pages` parameter (e.g., default 10, max 20) so the server can refuse sessions that would produce unreasonable amounts of data.
- Apply an HTTP request body size limit in the Gin middleware for the scan submit endpoint. A single base64-encoded multipage scan submission can easily exceed the default `http.MaxBytesReader` limit. Use gin's `c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50*1024*1024)` or the `gin-contrib/size` middleware.
- Log file store size (count and approximate bytes) at completion so memory growth is observable in production.

**Warning signs:**
- Process memory grows and does not fall between sessions during load testing.
- Server OOM-killed under concurrent sessions with multi-page scans.
- Large JSON payloads return 413 in production if deployed behind nginx with default `client_max_body_size`.

**Phase to address:**
The phase implementing multi-page submission. Add the request body size limit before the endpoint is tested with real device payloads.

---

### Pitfall 7: 100vh / Viewport Height Layout Breaks in iOS Safari

**What goes wrong:**
The crop UI is a fullscreen canvas overlay. If its height is set to `100vh`, it overflows on iOS Safari — the browser's address bar and toolbar are included in `100vh`, causing the bottom portion of the canvas to be hidden under the toolbar. On iOS Safari, the toolbar appears/disappears as the user scrolls, causing the layout to shift mid-interaction, which repositions the canvas while the user is dragging a crop handle.

**Why it happens:**
`100vh` in iOS Safari has historically referred to the "maximum viewport" (toolbar hidden), not the "current viewport" (toolbar visible). This is a long-standing WebKit behavior. The `dvh` (dynamic viewport height) unit was introduced to address this and is supported in iOS Safari 16+, but is not supported on older iOS.

**How to avoid:**
Use `height: 100dvh` as the primary value with `height: 100vh` as a fallback. For the crop canvas specifically, size it via JavaScript using `window.visualViewport.height` (when available) and listen to `visualViewport.resize` events to reflow the canvas when the toolbar appears/disappears. Alternatively, structure the crop UI as `position: fixed; top: 0; left: 0; right: 0; bottom: 0` — fixed positioning respects the current visual viewport on modern iOS Safari better than percentage heights.

**Warning signs:**
- Crop UI looks correct in devtools mobile emulation but bottom handles are inaccessible on a real iPhone.
- Layout jumps when user scrolls slightly in the crop view.

**Phase to address:**
The phase implementing the crop UI layout. Verify on a real iPhone (not simulator) before considering layout complete.

---

### Pitfall 8: Pinch-to-Zoom Conflicts with Crop Handle Dragging

**What goes wrong:**
While a user is dragging a crop corner, the browser may interpret a second-finger incidental touch as a pinch-to-zoom gesture. The page zooms, shifting all visible elements. The user's drag coordinates now refer to a zoomed coordinate space. The crop handle teleports to an incorrect position, and the user's sense of where "the corner" is breaks.

**Why it happens:**
The viewport meta tag `user-scalable=no` was the traditional solution, but iOS Safari has ignored this attribute since iOS 10, respecting user preference for accessibility. Android Chrome respects it but degrades accessibility.

**How to avoid:**
Do not rely on `user-scalable=no`. Instead, intercept the pinch gesture at the JS level for the duration of the crop interaction: listen for `touchstart` events with 2 or more touches on the crop overlay and call `preventDefault()` on them. This requires `{ passive: false }`. Apply `touch-action: none` to the crop overlay element to prevent all browser-native gestures. Ensure the crop UI is `position: fixed` at zoom level 1 — if the user zoomed before entering the crop UI, reset zoom via `window.scrollTo(0, 0)` and ensure the crop canvas fills the fixed viewport.

**Warning signs:**
- Handles work correctly on desktop but become uncontrollable on device when user accidentally touches with two fingers.
- Testing only with single-finger drags misses this entirely.

**Phase to address:**
The phase implementing crop handle interaction. Test with two-finger interaction patterns deliberately.

---

### Pitfall 9: base64-Encoding Multi-Page Results Exceeds Gin / Proxy Request Size Limits

**What goes wrong:**
The existing photo submit endpoint accepts base64-encoded image data in a JSON body. A single photo is typically 1–3MB, or 1.3–4MB as base64. A 10-page document scan at 1MB/page base64-encodes to ~13MB of JSON. Gin's default `MaxBytesReader` is 32MB, but nginx `client_max_body_size` defaults to 1MB. The result: the submission request is rejected with a 413 error before Gin even sees it.

**Why it happens:**
The base64-in-JSON pattern scales linearly with page count. It was fine for single-file submissions but the 33% base64 overhead compounds over many pages. Proxy size limits that were never noticed become blocking failures.

**How to avoid:**
- Change the multi-page scan submission to `multipart/form-data` instead of base64 JSON. This is more bandwidth-efficient and does not hit base64 overhead.
- Alternatively, keep base64 JSON but document that nginx `client_max_body_size` must be set to at least `50m` for scan sessions, and add a Gin middleware size check that returns a clear error before the body is read.
- Add explicit Go request body size limits via `http.MaxBytesReader` on the scan submit handler.

**Warning signs:**
- Single-page submissions work but multi-page submissions silently fail or get a "connection reset" error from the phone.
- 413 errors appear in nginx logs but not in Go application logs.

**Phase to address:**
The phase that implements multi-page scan submission to the server.

---

### Pitfall 10: gofpdf Holds All Page Images in RAM During Multi-Page PDF Assembly

**What goes wrong:**
The existing `ImageToPDF` function in `internal/util/pdf.go` uses `gofpdf`. For a 10-page document where each page is a 2MB JPEG, `gofpdf` buffers all image data in memory simultaneously until `pdf.Output()` is called. Peak memory usage for assembling the PDF is the sum of all page image sizes plus the encoded PDF data — potentially 30–40MB per session, held for the duration of assembly.

**Why it happens:**
`gofpdf` accumulates all content in-memory (by design — it writes the entire PDF on `Output()`). There is no streaming write. For single-image PDFs this is not an issue; for 10-page multi-image PDFs it becomes significant.

**How to avoid:**
- Restrict document page count (enforce `max_pages` on the server). A 5-page document is the practical limit for comfortable in-memory assembly.
- Consider switching to `pdfcpu` or `unipdf` for large multi-page PDF assembly if `gofpdf` memory usage proves problematic in testing — but verify before switching since `gofpdf` is already working for the existing use cases.
- Profile actual memory usage before optimizing. A 10-page scan at 500KB/page compressed is only 5MB of input — manageable. Profile with realistic device image output sizes before deciding whether a library change is needed.

**Warning signs:**
- Server memory spikes on multi-page PDF assembly under concurrent sessions.
- OOM on small container deployments (256MB containers).

**Phase to address:**
The phase implementing server-side PDF assembly for scan results. Profile before optimizing; `gofpdf` may be adequate for typical page counts.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Store page images as object URLs in JS array instead of blobs | Simpler code, no async serialization | Memory exhaustion on 4+ pages; iOS Safari canvas limit | Never — use blobs from the start |
| Reuse the same `canvas` element for crop UI and warp output | Fewer DOM elements | State confusion between crop and output steps; forces sequential not parallel operations | Never in multi-page flow |
| Apply the nested result structure to all action types at once | Unified schema | Immediate backwards compatibility break for photo/signature callers | Never — use `omitempty` fields keyed by action type |
| Skip coordinate space conversion (use display-space directly) | Faster initial implementation | Wrong warp output at full resolution; subtle bug only visible on real devices | Never |
| Use `user-scalable=no` to stop pinch zoom | Simple one-line fix | Ignored by iOS Safari 10+; creates accessibility issues | Never — use `touch-action: none` |
| Submit all pages in a single large JSON body | Matches existing submit endpoint contract | 413 errors behind nginx; 33% base64 overhead | Only if page count is bounded to 3 and body limit is configured |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Existing `submitResultHandler` | Adding scan-specific logic inline, creating branching spaghetti | Create a separate `submitScanResultHandler` for scan action type, registered on its own route |
| Existing `model.Session.Result []ResultItem` | Repurposing for nested documents by changing the type | Add a separate `Documents []ScannedDocument` field with `omitempty` |
| `store.StoreFile()` | Calling it once per page in a loop without checking total session file count | Enforce a page limit before the loop; fail early with 422 if exceeded |
| `pkg/` Go client library | Forgetting to add a `GetScanResult()` method when adding scan action type | The client library must have feature parity with the HTTP API for callers |
| gofpdf `ImageToPDF` | Calling it for each page individually and concatenating PDFs | Extend `ImageToPDF` (or add `ImagesToPDF`) to accept multiple images and produce a single multi-page PDF in one pass |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Keeping full-res image decoded in JS memory for crop UI | Tab crashes or freezes on older iPhones | Display downscaled image in crop UI; warp from original only at submission | 2+ captured pages on iPhone X or older |
| base64 encoding large images on the UI thread | UI freezes for 1–3 seconds during submit | Use `FileReader.readAsDataURL` asynchronously, or better: send as `multipart/form-data` | Images > 3MB on low-end Android |
| Drawing full-resolution image into crop canvas | Slow initial render; canvas allocation failure | Scale image to display dimensions before drawing, keep source for warp | 12MP+ photos from any modern phone |
| Holding raw `[]byte` page images in `go-cache` across many concurrent sessions | Server RSS grows unbounded | Enforce per-session page limit; set tight ResultTTL for scan results | 3+ concurrent sessions with 5+ pages each |
| Single-threaded PDF assembly blocking the Gin goroutine | Request timeout for large scans | Run PDF assembly in a goroutine with a timeout context | 10+ pages per document |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| No request body size limit on scan submit endpoint | Malicious client sends 500MB payload, exhausting server memory | Add `http.MaxBytesReader` to scan submit handler; document nginx `client_max_body_size` requirement |
| Accepting arbitrary MIME types in scan page data | Client submits non-image data labeled as `image/jpeg`; server passes to gofpdf which panics or corrupts output | Validate decoded bytes with `http.DetectContentType()` before PDF assembly |
| No max_pages enforcement at session creation | Single session can generate unlimited file entries in `go-cache` | Add `max_pages` to `createSessionRequest`, cap at a reasonable limit (e.g., 20), store in `Session` model |
| Exposing download IDs for all pages without checking session ownership | Any client knowing a download ID can retrieve page images | Existing `GetFile` endpoint has no auth — this is by design for the phone to access results, but document that download IDs should be treated as secrets |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Crop handles positioned exactly at the corner (under the finger) | User cannot see what's under their finger; cannot place the corner precisely | Position handles outside the corner by 20–30px; show a magnified offset view of the corner area while dragging |
| No visual feedback when perspective warp is computing | User taps "Use this crop" and sees nothing for 1–2 seconds | Show a spinner or "Processing..." state immediately on tap; warp is synchronous canvas operation but can feel slow on large images |
| Submitting all pages at once with no progress indicator | Upload of 10 pages can take 10–20 seconds on a slow mobile connection; user thinks it failed | Show per-file upload progress or a progress bar during submission |
| No re-crop option after warp preview | User sees the warped result is misaligned but cannot go back to adjust | Always provide a "Re-crop" button on the warp preview screen |
| "Next Document" button not clearly differentiated from "Next Page" | Multi-document mode users create one document with many pages instead of separate documents | Use distinct visual treatment (e.g., color, icon, label) for document vs. page boundary actions |
| No indication of page count while capturing | User loses track of how many pages they've captured in multi-page mode | Show a persistent page counter badge (e.g., "Page 3 of ?") throughout the capture flow |

---

## "Looks Done But Isn't" Checklist

- [ ] **EXIF orientation normalization:** Works in Chrome DevTools device emulation (where test files have no EXIF) — verify by capturing a real portrait-mode photo on an iPhone and confirming crop UI shows upright image.
- [ ] **Coordinate space conversion:** Warp output looks correct on small test images — verify with a 12MP photo (actual phone camera output) and compare warp output dimensions to expected document aspect ratio.
- [ ] **Touch handle drag:** Handles move on desktop mouse events — verify `touch-action: none` and `{ passive: false }` on a real iOS Safari device with page scrolling enabled.
- [ ] **Viewport height layout:** Crop UI fills screen in Chrome DevTools — verify on real iPhone that bottom handles are not obscured by the Safari toolbar.
- [ ] **Multi-page memory:** Works for 3 pages in desktop — verify with 10 pages on an iPhone X or older device.
- [ ] **Backwards compatibility:** `GET /api/v1/sessions/:id/result` returns `items` for photo/signature sessions — verify existing photo workflow end-to-end after any model changes.
- [ ] **Request body size limit:** Multi-page submission works locally (no proxy) — verify with nginx in front, or document configuration requirement clearly.
- [ ] **PDF multi-page:** `ImageToPDF` produces a single multi-page PDF (not a single-page PDF with only the first image) — verify page count in output PDF.
- [ ] **Go client library:** `pkg/` has methods for creating scan sessions and reading scan results — verify `pkg/` is updated alongside server code.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| EXIF orientation missed | MEDIUM | Add normalization step before crop display; all existing cropped-and-submitted photos may have been warped from wrong orientation — cannot recover already-submitted sessions |
| Coordinate space mismatch | MEDIUM | Fix scale factor conversion; warp logic is self-contained so change is isolated to the warp function |
| Touch events scroll page | LOW | Add `{ passive: false }` and `touch-action: none` to handle elements; purely CSS/JS change |
| API shape break for existing callers | HIGH | Requires versioning the result endpoint or restoring the original field — if callers are already deployed against the new shape, a coordinated rollback is needed |
| Memory exhaustion from large files | MEDIUM | Add request body size limit and page count cap; existing stored sessions are unaffected |
| iOS Safari canvas crash | MEDIUM | Refactor page array to use blobs instead of canvas references; requires rethinking multi-page state model |
| PDF assembly OOM | LOW-MEDIUM | Reduce max page limit or optimize to use multi-image PDF function instead of per-page calls |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| EXIF orientation ignored by canvas | Crop UI phase — image loading | Capture portrait photo on iOS; verify crop UI shows upright image |
| Touch events scroll page | Crop UI phase — handle interaction | Drag handles on real iOS Safari; confirm page does not scroll |
| Perspective warp coordinate mismatch | Crop UI phase — warp implementation | Warp a 12MP photo; confirm output aspect ratio matches document |
| iOS Safari canvas memory limit | Multi-page state management phase | Capture 10 pages on iPhone X; confirm no blank canvas or tab crash |
| Result API backwards compatibility | Scan model definition phase — before any code | Run existing photo/signature end-to-end test after model change |
| In-memory store exhaustion | Multi-page submission phase | Load test with 5 concurrent sessions × 10 pages; monitor RSS |
| 100vh / viewport layout | Crop UI layout phase | Test on real iPhone with Safari toolbar visible |
| Pinch-to-zoom conflicts | Crop UI interaction phase | Two-finger gesture test on real device during handle drag |
| base64 + proxy 413 | Multi-page submission phase | Submit 10 pages behind nginx with default config; confirm error or configuration fix |
| gofpdf multi-page PDF RAM | PDF assembly phase | Profile RSS before and after 10-page PDF assembly; compare to baseline |

---

## Sources

- [PQINA: Total Canvas Memory Use Exceeds The Maximum Limit](https://pqina.nl/blog/total-canvas-memory-use-exceeds-the-maximum-limit) — iOS Safari canvas memory limits
- [Apple Developer Forums: Total canvas memory use](https://developer.apple.com/forums/thread/112218) — iOS Safari canvas limit discussion
- [MDN: Touch events](https://developer.mozilla.org/en-US/docs/Web/API/Touch_events) — touch event model and `{ passive: false }`
- [Chrome for Developers: Making touch scrolling fast by default](https://developer.chrome.com/blog/scrolling-intervention) — passive event listener default
- [MDN: touch-action CSS property](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/touch-action) — disabling browser gestures on elements
- [GitHub: perspective-transform JS library](https://github.com/jlouthan/perspective-transform) — 4-point perspective warp in JS
- [GitHub: Homography.js](https://github.com/Eric-Canas/Homography.js) — high-performance canvas perspective transform
- [DEV Community: Fix mobile keyboard overlap with VisualViewport](https://dev.to/franciscomoretti/fix-mobile-keyboard-overlap-with-visualviewport-3a4a) — viewport height fix
- [HTMHell: Control the Viewport Resize Behavior with interactive-widget](https://www.htmhell.dev/adventcalendar/2024/4/) — dvh unit and iOS safari layout
- [Medium: Image Rotation Problem From Mobile Phone Camera](https://medium.com/@dollyaswin/image-rotation-problem-from-mobile-phone-camera-javascript-flutter-e38fcba58c5f) — EXIF orientation on mobile
- [GitHub: go-cache concurrent map write issue](https://github.com/patrickmn/go-cache/issues/60) — go-cache thread safety
- [GitHub: gofpdf](https://github.com/jung-kurt/gofpdf) — in-memory PDF assembly model
- [Google AIP-180: Backwards compatibility](https://google.aip.dev/180) — API compatibility guidelines
- [Dynamsoft: Web Document Scanner with OpenCV.js](https://www.dynamsoft.com/codepool/web-document-scanner-with-opencvjs.html) — canvas coordinate systems in document scanning

---
*Pitfalls research for: Document scanning action type added to Handoff Go/HTML session server*
*Researched: 2026-02-27*
