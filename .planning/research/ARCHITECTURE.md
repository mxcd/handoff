# Architecture Research

**Domain:** Document scanning integration into existing Handoff server
**Researched:** 2026-02-27
**Confidence:** HIGH — based on direct source inspection of the existing codebase

---

## Existing System Overview

```
Caller (backend app)
    |
    | POST /api/v1/sessions  (X-API-Key)
    v
┌──────────────────────────────────────────────────────────────┐
│  internal/server/  (Gin HTTP server)                         │
│                                                              │
│  ProtectedAPI group (/api/v1) — apiKeyAuth() middleware      │
│  ┌──────────────────┐  ┌──────────────────┐                  │
│  │session_controller│  │result_controller │                  │
│  │ POST /sessions   │  │ GET /sessions/   │                  │
│  │ GET  /sessions/  │  │     :id/result   │                  │
│  └──────┬───────────┘  └──────┬───────────┘                  │
│         │                    │                               │
│  Public routes (/s/:id)       │                               │
│  ┌──────────────────────────┐ │                               │
│  │ session_page_controller  │ │                               │
│  │  GET /s/:id              │ │                               │
│  │  GET /s/:id/action       │ │                               │
│  │  POST /s/:id/result      │ │                               │
│  └──────────┬───────────────┘ │                               │
│             │                 │                               │
│             v                 v                               │
│  ┌──────────────────────────────────────┐                     │
│  │           internal/store/            │                     │
│  │   sessions cache (go-cache)          │                     │
│  │   files cache    (go-cache)          │                     │
│  └──────────────────────────────────────┘                     │
│                                                              │
│  internal/ws/ Hub — WebSocket broadcast on status changes     │
│  internal/util/pdf.go — image/SVG to PDF conversion          │
│  internal/web/ — embedded templates + static assets          │
└──────────────────────────────────────────────────────────────┘
    |
    | GET /s/:id (phone opens QR code URL)
    v
Phone browser (server-rendered HTML, plain JS)
```

### Component Responsibilities

| Component | Responsibility | File |
|-----------|---------------|------|
| `session_controller.go` | Create/read sessions; validates ActionType and OutputFormat | `internal/server/session_controller.go` |
| `session_page_controller.go` | Renders phone-facing HTML pages; dispatches on `ActionType` in `renderActionPage()` | `internal/server/session_page_controller.go` |
| `result_controller.go` | Receives `POST /s/:id/result` from phone; decodes base64 items; converts to PDF if needed; marks session complete | `internal/server/result_controller.go` |
| `download_controller.go` | Serves stored files by `download_id` | `internal/server/download_controller.go` |
| `internal/store/store.go` | In-memory session + file store; TTL management | `internal/store/store.go` |
| `internal/model/session.go` | ActionType, OutputFormat, Session, ResultItem types | `internal/model/session.go` |
| `internal/util/pdf.go` | `ImageToPDF`, `SVGToPDF` — server-side conversion | `internal/util/pdf.go` |
| `internal/web/web.go` | `RenderPage()` — renders named template into base layout | `internal/web/web.go` |
| `internal/ws/hub.go` | WebSocket hub; broadcasts status + completion events | `internal/ws/hub.go` |

---

## Document Scanning Integration Map

### What Does Not Change

These components require zero modification:

- `cmd/server/main.go` — entrypoint wiring unchanged
- `internal/store/store.go` — file storage model is generic enough; `StoreFile`/`GetFile` work for any binary blob
- `internal/ws/hub.go` — broadcast interface is action-agnostic
- `internal/server/server.go` — no new routes needed (existing `POST /s/:id/result` handles scan submission)
- `internal/web/web.go` — `RenderPage()` is generic; just pass a new template name
- `base.html` — layout unchanged

### What Changes (Existing Files Modified)

#### 1. `internal/model/session.go` — Moderate change

Add `ActionTypeScan` constant and update two functions:

```go
// Add constant
ActionTypeScan ActionType = "scan"

// Update ValidateActionType — add scan case
case ActionTypeScan:
    return ActionTypeScan, nil

// Update ValidateOutputFormat — scan supports jpg, png, pdf
case ActionTypeScan:
    switch OutputFormat(format) {
    case OutputFormatJPG, OutputFormatPNG, OutputFormatPDF:
        return OutputFormat(format), nil
    default:
        return "", fmt.Errorf(...)
    }
```

Also add scan-specific session params to the `Session` struct:

```go
// New field — set at session creation, not mutable
MultiDocument bool `json:"multi_document,omitempty"`
```

The `Result` field on `Session` currently holds `[]ResultItem`. For scan, the result is nested: documents contain pages. Two options exist:

**Option A — Keep flat ResultItem, add document/page metadata fields:**
```go
type ResultItem struct {
    DownloadID   string `json:"download_id"`
    ContentType  string `json:"content_type"`
    Filename     string `json:"filename"`
    // New for scan — zero values are omitted for photo/signature
    DocumentIndex int   `json:"document_index,omitempty"`
    PageIndex     int   `json:"page_index,omitempty"`
}
```

**Option B — Introduce ScanResult as a parallel result field:**
```go
type ScanDocument struct {
    Pages []ResultItem `json:"pages"`
}

// In Session:
ScanResult []ScanDocument `json:"scan_result,omitempty"`
```

**Recommendation: Option B.** The caller API contract for existing action types (photo, signature) uses `items: []ResultItem`. Scan has fundamentally different nesting. Adding `scan_result` alongside `items` keeps backward compatibility and makes the structure self-documenting. The result controller emits `scan_result` for scan sessions and `items` for all others.

#### 2. `internal/server/session_controller.go` — Small change

Add scan-specific params to `createSessionRequest` and populate them on the `Session`:

```go
type createSessionRequest struct {
    ActionType    string `json:"action_type" binding:"required"`
    IntroText     string `json:"intro_text"`
    OutputFormat  string `json:"output_format" binding:"required"`
    SessionTTL    string `json:"session_ttl"`
    ResultTTL     string `json:"result_ttl"`
    // New — scan only; ignored for other action types
    MultiDocument bool   `json:"multi_document"`
}

// In handler, after validating ActionType:
session := model.Session{
    ...
    MultiDocument: req.MultiDocument, // zero-value false is correct default
}
```

#### 3. `internal/server/result_controller.go` — Significant change

The `submitResultHandler` currently accepts:
```json
{ "items": [ { "content_type": "...", "filename": "...", "data": "base64..." } ] }
```

For scan, the phone submits:
```json
{
  "documents": [
    {
      "pages": [
        { "content_type": "image/jpeg", "filename": "doc1_page1.jpg", "data": "base64..." }
      ]
    }
  ]
}
```

The handler needs to detect which format is being submitted (based on `session.ActionType`) and route accordingly:

```go
// New request type for scan
type submitScanPage struct {
    ContentType string `json:"content_type" binding:"required"`
    Filename    string `json:"filename" binding:"required"`
    Data        string `json:"data" binding:"required"`
}

type submitScanDocument struct {
    Pages []submitScanPage `json:"pages" binding:"required"`
}

type submitScanRequest struct {
    Documents []submitScanDocument `json:"documents" binding:"required"`
}
```

The handler branches on `session.ActionType == model.ActionTypeScan`:
- Parse `submitScanRequest` instead of `submitResultRequest`
- Store each page as a separate file (one `StoreFile` call per page)
- Build `[]model.ScanDocument` with nested page `ResultItem` slices
- Call `s.Store.MarkSessionCompletedScan(id, scanDocuments)` (new store method, see below)
- Broadcast completion with scan result

Multi-page PDF assembly (when `output_format == "pdf"` and multi-page scan): instead of calling `ImageToPDF` once per file, accumulate all page images for a document into a single multi-page PDF. This requires a new utility function:

```go
// internal/util/pdf.go — new function
func ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error)
```

#### 4. `internal/server/session_page_controller.go` — Small change

Add `ActionTypeScan` to the switch in `renderActionPage()`:

```go
case model.ActionTypeScan:
    templateName = "action_scan.html"
    // Pass scan-specific data
    data["MultiDocument"] = session.MultiDocument
```

#### 5. `internal/store/store.go` — Small addition

Add `MarkSessionCompletedScan` method (or extend `MarkSessionCompleted` to accept a `ScanResult` parameter):

```go
// Option: add separate method
func (s *Store) MarkSessionCompletedScan(id string, result []model.ScanDocument) error {
    sess, _ := s.GetSession(id)
    now := time.Now()
    sess.Status = model.SessionStatusCompleted
    sess.CompletedAt = &now
    sess.ScanResult = result
    return s.UpdateSession(sess)
}
```

Alternatively, extend `MarkSessionCompleted` to accept an interface or add a `ScanResult` parameter. Separate method is cleaner and avoids breaking the existing callers.

### What Is New (New Files Created)

#### 6. `internal/web/templates/action_scan.html` — New, complex

This is the primary new work. The scan UI requires multiple distinct states within a single template:

```
State machine (client-side JS):
  CAPTURE → [user takes photo] → CROP → [user adjusts corners] → PREVIEW
                                   ^                                |
                                   |_____ [re-crop] ______________|
                                                                   |
                                              (multi-doc) → NEXT_DOCUMENT
                                              (single-doc / last doc) → SUBMIT
```

Template data passed from server:
```go
data := map[string]interface{}{
    "SessionID":     session.ID,
    "SubmitURL":     fmt.Sprintf("/s/%s/result", session.ID),
    "OutputFormat":  string(session.OutputFormat),
    "MultiDocument": session.MultiDocument,
}
```

Key JS components within the template:

| Component | Purpose |
|-----------|---------|
| Camera capture (`<input capture="environment">`) | Acquire raw photo |
| Crop canvas | Display captured image; 4 draggable corner handles |
| Perspective warp (Canvas 2D + homography) | Client-side transform before upload |
| Magnifying glass overlay | Shows corner position at 2-3x zoom during drag |
| Preview canvas | Show warped result before committing |
| Document accumulator | JS array holding page blobs for current document |
| Multi-document state | Track document boundary; "Next Document" advances to new doc |
| Submit serializer | Encode all docs/pages to base64; build nested JSON payload |

#### 7. `internal/util/pdf.go` — New function (additive)

```go
// ImagesToPDF assembles multiple images into a single multi-page PDF.
// Each image becomes one page sized to fit the image dimensions.
func ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error)
```

This extends the existing `gofpdf`-based approach: loop through pages calling `pdf.AddPage()` + `pdf.ImageOptions()` for each.

---

## Data Flow: Document Scan Session

### Creation Flow

```
Caller POST /api/v1/sessions
  { action_type: "scan", output_format: "pdf", multi_document: true, ... }
      |
      v
session_controller.go
  ValidateActionType("scan") → ActionTypeScan
  ValidateOutputFormat(scan, "pdf") → OutputFormatPDF
  Session{ MultiDocument: true, ... }
      |
      v
store.CreateSession(session)
      |
      v
Response: { id: "uuid", url: "https://.../s/uuid", ... }
```

### Phone Capture Flow

```
Phone opens /s/:id
      |
      v
session_page_controller.sessionPageHandler()
  → renderActionPage() → "action_scan.html"
    data: { SessionID, SubmitURL, OutputFormat, MultiDocument }
      |
      v
Browser renders action_scan.html

User captures photo → crop canvas appears
User drags 4 corner handles → magnifier assists precision
User taps "Apply Crop" → JS runs perspective warp on Canvas
Warped image shown in preview → user confirms or re-crops

[Multi-document: user taps "Next Document" → accumulates current doc, returns to CAPTURE]
[Final document: user taps "Submit"]

JS builds payload:
{
  "documents": [
    { "pages": [ { "content_type": "image/jpeg", "filename": "doc1_page1.jpg", "data": "<base64>" } ] },
    { "pages": [ { "content_type": "image/jpeg", "filename": "doc2_page1.jpg", "data": "<base64>" } ] }
  ]
}

POST /s/:id/result (JSON body)
```

### Submission Flow

```
POST /s/:id/result
      |
      v
result_controller.submitResultHandler()
  GetSession(id) → verify scan session
  branch: session.ActionType == ActionTypeScan
      |
      v
  Parse submitScanRequest{ Documents: [ { Pages: [...] } ] }
  For each document:
    For each page: base64.Decode → fileData
    If OutputFormat == PDF && len(pages) > 1:
      util.ImagesToPDF(pageDataSlice, contentTypes) → pdfBytes
      StoreFile(downloadID, pdfBytes, "application/pdf", ResultTTL)
      Append ResultItem{DownloadID, ContentType: "application/pdf", Filename: "doc1.pdf"}
    Else (single page or image output):
      StoreFile(downloadID, fileData, contentType, ResultTTL)
      Append ResultItem{...}
    ScanDocument{ Pages: []ResultItem{...} }
      |
      v
  store.MarkSessionCompletedScan(id, []ScanDocument{...})
  ws.Hub.BroadcastCompletion(id, "completed", scanResult)
      |
      v
  Response: { "scan_result": [ { "pages": [...] } ] }
```

### Caller Retrieval Flow

```
Caller GET /api/v1/sessions/:id/result
      |
      v
result_controller.getResultHandler()
  session.Status == completed
  Response:
  {
    "status": "completed",
    "completed_at": "...",
    "scan_result": [ { "pages": [ { "download_id": "...", "content_type": "...", "filename": "..." } ] } ]
    // "items" field absent for scan sessions
  }
```

---

## Build Order (Dependency-Driven)

The scan feature has hard dependencies between layers. Build bottom-up to avoid integration gaps.

### Phase 1: Model Layer (no dependencies)

**Files:** `internal/model/session.go`

- Add `ActionTypeScan` constant
- Update `ValidateActionType` to accept "scan"
- Add `MultiDocument bool` to `Session`
- Add `ScanDocument` type and `ScanResult []ScanDocument` to `Session`
- Update `ValidateOutputFormat` for scan

This is zero-risk: purely additive changes. Existing action types are unaffected.

### Phase 2: Store Layer (depends on Phase 1)

**Files:** `internal/store/store.go`

- Add `MarkSessionCompletedScan(id string, result []model.ScanDocument) error`

Existing `MarkSessionCompleted` remains unchanged for photo/signature.

### Phase 3: PDF Utility (no dependencies)

**Files:** `internal/util/pdf.go`

- Add `ImagesToPDF(pages [][]byte, contentTypes []string) ([]byte, error)`

Can be built and unit-tested in isolation. Existing `ImageToPDF` and `SVGToPDF` unchanged.

### Phase 4: Session Creation (depends on Phase 1)

**Files:** `internal/server/session_controller.go`

- Add `MultiDocument bool` to `createSessionRequest`
- Populate `session.MultiDocument` from request

Straightforward field passthrough. Validates correctly once Phase 1 is done.

### Phase 5: Submission Handler (depends on Phases 1, 2, 3)

**Files:** `internal/server/result_controller.go`

- Add `submitScanPage`, `submitScanDocument`, `submitScanRequest` types
- Branch on `session.ActionType == model.ActionTypeScan`
- Implement scan decode-store-assemble loop
- Call `MarkSessionCompletedScan`
- Update `getResultHandler` to emit `scan_result` vs `items` based on action type

This is the most complex server-side change. Build after Phases 1-3.

### Phase 6: Page Controller (depends on Phase 1)

**Files:** `internal/server/session_page_controller.go`

- Add `ActionTypeScan` case to `renderActionPage` switch
- Pass `MultiDocument` in template data

One switch case and one data field. Trivially done once Phase 1 is in place.

### Phase 7: Scan UI Template (depends on Phase 6)

**Files:** `internal/web/templates/action_scan.html`

This is the largest and most independent piece of work. It runs entirely in the browser; the server just serves the template. Build order within the template:

1. Camera capture state (wire `<input capture>` → show raw image)
2. Crop canvas with 4-corner overlay (touch + mouse drag for handles)
3. Magnifier glass implementation
4. Perspective warp (homography on Canvas 2D)
5. Preview confirm/re-crop flow
6. Multi-document state machine (accumulate docs, "Next Document" button)
7. Submission serializer (base64 encode, build nested JSON, POST)

Can be developed against a stubbed/hardcoded session ID and a real running server instance for rapid iteration.

---

## Component Boundary Summary

| Layer | New | Modified | Unchanged |
|-------|-----|----------|-----------|
| Model | `ScanDocument` type | `Session`, `ActionType`, `ValidateActionType`, `ValidateOutputFormat` | `ResultItem`, `SessionStatus`, `OutputFormat` |
| Store | `MarkSessionCompletedScan` | `Session` embedded type | `CreateSession`, `GetSession`, `UpdateSession`, `StoreFile`, `GetFile`, `MarkSessionCompleted`, `MarkSessionOpened` |
| Util | `ImagesToPDF` | — | `ImageToPDF`, `SVGToPDF` |
| Server routes | — | — | All routes (no new endpoints needed) |
| Session controller | — | `createSessionRequest`, handler | `getSessionHandler` |
| Page controller | — | `renderActionPage` switch | `sessionPageHandler`, `sessionActionHandler` |
| Result controller | Scan request types | `submitResultHandler`, `getResultHandler` | Core file decode/store loop |
| Templates | `action_scan.html` | — | `base.html`, all others |
| Go client pkg | Session creation opts | Session model | All other client methods |

---

## Architectural Patterns to Follow

### Pattern 1: Action-Type Branch in Result Handler

**What:** Gate scan logic behind `if session.ActionType == model.ActionTypeScan` in `submitResultHandler`. Do not modify the existing flat-items code path.

**Why:** The existing path is correct for photo and signature. Interleaving conditional logic would make both paths harder to reason about.

**Implication:** `getResultHandler` also branches: scan sessions return `scan_result`, others return `items`. Both fields are omitempty on `Session`, so the JSON response is clean.

### Pattern 2: Client-Side Warp, Server Receives Final Image

**What:** All perspective correction happens in the browser Canvas before upload. The server receives already-warped JPEG bytes — no server-side image processing library needed.

**Why:** Avoids adding heavy image processing dependencies (libvips, OpenCV bindings) to a pure-Go server. The phone's GPU handles the transform efficiently in the browser via Canvas 2D `drawImage` with a homography transform.

**Implication:** `util/pdf.go` only needs to assemble pre-warped images into PDFs, not transform them.

### Pattern 3: Additive Model Changes, Not Replacement

**What:** Add `ScanResult []ScanDocument` alongside existing `Result []ResultItem` on `Session`. Do not rename or repurpose `Result`.

**Why:** Existing callers of `getResultHandler` for photo/signature sessions see `items` in the response. Changing the field name or structure breaks them.

**Implication:** The `Session` struct carries two optional result fields. Only one is ever populated for a given session. Both are `omitempty`.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Sending Raw (Uncropped) Photos to Server

**What people do:** Upload the raw camera image and let the server crop and warp.

**Why wrong:** Requires server-side image processing (large dependency footprint). Sends more data over the mobile network. Loses the user's precise manual crop intent in translation.

**Do this instead:** Warp on the Canvas client-side. Upload only the final cropped JPEG. The server never sees uncropped data.

### Anti-Pattern 2: New Route for Scan Submission

**What people do:** Add `POST /s/:id/scan-result` as a separate endpoint.

**Why wrong:** Fragments the URL structure. The existing `POST /s/:id/result` endpoint already handles auth, session lookup, expiry checks, and file storage. Duplicating that logic creates maintenance surface.

**Do this instead:** Branch inside `submitResultHandler` on `session.ActionType`. One endpoint, two code paths within it.

### Anti-Pattern 3: Assembling Multi-Page PDFs on the Client

**What people do:** Use a JS PDF library (jsPDF) to build the PDF in the browser before upload.

**Why wrong:** Large JS dependency in a no-JS-build-tool environment. Slow on low-end phones. Server already has gofpdf for PDF assembly.

**Do this instead:** Upload individual page images; assemble the multi-page PDF server-side in `ImagesToPDF`. This is a clean fit with the existing `util/pdf.go` pattern.

### Anti-Pattern 4: Storing Entire Document Set as One Blob

**What people do:** Combine all pages into one upload payload or one stored file.

**Why wrong:** The store model (`StoreFile`) is per-file with individual TTLs and download IDs. One blob per document page is the natural granularity — it maps directly to `ResultItem.DownloadID` and individual download URLs.

**Do this instead:** Store each page as a separate `StoreFile` entry. The nested `ScanDocument.Pages` structure in the result conveys the grouping without conflating storage granularity.

---

## Scaling Considerations

This milestone does not change the scaling profile of the server. All constraints remain:

| Concern | Current Approach | Impact of Scan |
|---------|-----------------|----------------|
| Session memory | In-memory cache, short TTL | Unchanged — scan sessions have same TTL model |
| File storage memory | In-memory, ResultTTL | Scan uploads larger (full-res JPEG per page); may need ResultTTL tuned shorter for scan sessions |
| Upload payload size | Currently unbounded | Multi-page scan payloads can be large (5-10 MB per document); consider a server-side body size limit in Gin |
| PDF assembly CPU | Per-conversion overhead | `ImagesToPDF` is synchronous in handler goroutine; acceptable for current scale (Gin creates a goroutine per request) |

The only new operational concern is upload payload size. Gin's default does not enforce a body limit. Adding `c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)` in the scan branch of `submitResultHandler` is recommended.

---

## Sources

- Direct source inspection: `internal/model/session.go`, `internal/store/store.go`, `internal/server/*.go`, `internal/web/web.go`, `internal/util/pdf.go`, `internal/ws/hub.go`
- `internal/web/templates/action_photo.html` — existing template as pattern reference for new scan template
- `PROJECT.md` — v1.1 requirements for scan action type

---

*Architecture research for: Document scanning integration — Handoff v1.1*
*Researched: 2026-02-27*
