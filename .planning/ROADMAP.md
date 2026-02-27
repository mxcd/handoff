# Roadmap: Handoff

## Milestones

- ✅ **v1.0 MVP** - Phases 1-4 (shipped 2026-02-26)
- 🚧 **v1.1 Document Scanning** - Phases 5-6 (in progress)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-4) - SHIPPED 2026-02-26</summary>

### Phase 1: Foundation
**Goal**: A running Go server with infrastructure endpoints, API key authentication, and in-memory session store ready for use
**Depends on**: Nothing (first phase)
**Requirements**: INFR-01, INFR-02, INFR-03, INFR-04, AUTH-01, AUTH-02
**Success Criteria** (what must be TRUE):
  1. Server starts, responds to `GET /health` with status, and responds to `GET /version` with deployment version
  2. Server shuts down cleanly on SIGINT/SIGTERM without dropping in-flight requests
  3. A request with a valid API key is accepted; a request with a missing or invalid key is rejected with 401
  4. Static assets are served from the embedded binary (no external files required at runtime)
**Plans**: 3 plans

Plans:
- [x] 01-01: Scaffold cleanup — remove defector imports, wire Handoff-specific structure
- [x] 01-02: Infrastructure endpoints, graceful shutdown, embed.FS static assets
- [x] 01-03: API key config via go-config and Gin auth middleware

### Phase 2: Session Core
**Goal**: A calling backend can create sessions, receive unique URLs, and retrieve completed results via polling or WebSocket
**Depends on**: Phase 1
**Requirements**: SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, RESL-01, RESL-02, RESL-03
**Success Criteria** (what must be TRUE):
  1. Caller POSTs to create a session with action type, optional intro text, TTLs, and result format — receives a unique URL back
  2. Caller GETs the result endpoint and receives the result once the session is complete, or a not-ready response while pending
  3. Caller receives a WebSocket notification the moment a session completes
  4. Sessions that exceed their TTL are no longer accessible; result availability expires independently after its own TTL
  5. All session state lives in-memory with no database dependency
**Plans**: 4 plans

Plans:
- [x] 02-01: Session/result model, in-memory store via go-cache, TTL expiry and tombstones
- [x] 02-02: Session creation (POST) and retrieval (GET) API with URL generation
- [x] 02-03: Result polling, result submission, and file download endpoints
- [x] 02-04: Per-session WebSocket notifications for real-time result delivery

### Phase 3: Phone UI and Actions
**Goal**: A phone user can scan the QR code URL, see the correct action UI, and submit a completed photo or signature back to Handoff
**Depends on**: Phase 2
**Requirements**: UI-01, UI-02, UI-03, UI-04, PHOT-01, PHOT-02, SIGN-01, SIGN-02, SIGN-03
**Success Criteria** (what must be TRUE):
  1. Phone user visits the session URL and sees an optional intro/explanation page when configured
  2. Phone user sees an expired/invalid session page when the session TTL has passed
  3. Phone user can capture a photo via the device camera and the photo is submitted to the Handoff backend
  4. Phone user sees a full-screen touch signature field, can undo and redo strokes, and the completed signature is submitted to the backend
  5. The action UI rendered matches the action type specified when the session was created
**Plans**: 4 plans

Plans:
- [x] 03-01: Go HTML template system, routing for session UI, expired session page
- [x] 03-02: Intro page and action-type dispatch logic
- [x] 03-03: Photo capture UI and HTTP form submission
- [x] 03-04: Signature UI (full-screen touch field, undo/redo) and HTTP form submission

### Phase 4: Client Library and Dev Tools
**Goal**: Go applications can integrate with Handoff via a typed client library, and the full flow can be tested locally end-to-end
**Depends on**: Phase 3
**Requirements**: INFR-05, DEV-01
**Success Criteria** (what must be TRUE):
  1. A Go application can import pkg/ and create sessions, receive URLs, and poll or subscribe for results without writing raw HTTP calls
  2. The mock consumer server starts, creates a session, displays the QR code URL, and receives the completed result — providing a full local e2e test harness
**Plans**: 2 plans

Plans:
- [x] 04-01: Go client library in pkg/ — types, errors, client with builder pattern, session with WebSocket subscription
- [x] 04-02: Mock consumer server — browser dashboard with session creation, QR display, live status, result preview

</details>

### 🚧 v1.1 Document Scanning (In Progress)

**Milestone Goal:** Add document scanning as a first-class action type — manual 4-corner crop, client-side perspective warp, multi-page capture, multi-document grouping, server-side PDF assembly, and nested result structure.

#### Phase 5: Scan Server Infrastructure
**Goal**: The server fully supports scan sessions end-to-end — session creation, multipart upload, PDF assembly, nested result delivery, body size limits, PDF library migration, and Go client library scan support — so the phone UI phase can build against a complete, stable server contract
**Depends on**: Phase 4
**Requirements**: SCAN-01, SCAN-02, SCAN-03, INFR-11, INFR-12, RESL-11, RESL-12, RESL-13, RESL-14, CLIB-01, CLIB-02
**Success Criteria** (what must be TRUE):
  1. Caller can create a scan session specifying single or multi-document mode and PDF or image output format; existing photo and signature sessions are unchanged
  2. Server accepts multipart/form-data scan page uploads, stores pages per session and document boundary, and enforces a configurable request body size limit
  3. When output format is PDF, server assembles all pages of each document into a single multi-page PDF; when format is images, server returns individual page files — both delivered via the nested `documents` array in the API response
  4. A Go application can use pkg/ to create scan sessions with document mode and output format options, and parse a typed nested scan result without writing raw HTTP or JSON code
  5. Server builds and all tests pass with `phpdave11/gofpdf` replacing the archived `jung-kurt/gofpdf`
**Plans**: TBD

Plans:
- [x] 05-01: Scan model types, phpdave11/gofpdf migration with ImagesToPDF, store scan page accumulation
- [x] 05-02: Scan session creation, multipart upload endpoints, PDF assembly, result delivery with nested documents
- [x] 05-03: Go client library scan support — ActionTypeScan, ScanDocumentMode, ScanOutputFormat, WaitForScanResult

#### Phase 6: Scan Capture and Crop UI
**Goal**: A phone user can capture document pages via the device camera, manually crop each page with 4-corner handles and an offset magnifying glass, preview the perspective-corrected result, manage multiple pages and documents, and submit the completed scan to the server
**Depends on**: Phase 5
**Requirements**: CAPT-01, CAPT-02, CAPT-03, CROP-01, CROP-02, CROP-03, CROP-04, CROP-05, PAGE-01, PAGE-02, PAGE-03, DOCS-01, DOCS-02
**Success Criteria** (what must be TRUE):
  1. Phone user captures a document photo and the image displays upright in the crop UI regardless of device orientation at capture time (EXIF normalization applied)
  2. Phone user sees 4 draggable corner handles positioned outside the document corners; an offset magnifying glass appears while dragging to show the exact corner position under the finger
  3. Phone user sees a perspective-corrected flat preview of the cropped document and can either accept it or return to re-crop
  4. In multi-page mode, phone user can capture additional pages, review all pages, remove any page, and choose single-page or multi-page mode before starting
  5. In multi-document mode, phone user can separate document boundaries with a "Next Document" button, review all documents and pages, and submit the complete multi-document scan
**Plans**: TBD

Plans:
- [ ] 06-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 5 → 6

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation | v1.0 | 3/3 | Complete | 2026-02-26 |
| 2. Session Core | v1.0 | 4/4 | Complete | 2026-02-26 |
| 3. Phone UI and Actions | v1.0 | 4/4 | Complete | 2026-02-26 |
| 4. Client Library and Dev Tools | v1.0 | 2/2 | Complete | 2026-02-26 |
| 5. Scan Server Infrastructure | v1.1 | 3/TBD | In progress | - |
| 6. Scan Capture and Crop UI | v1.1 | 0/TBD | Not started | - |
