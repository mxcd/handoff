# Roadmap: Handoff

## Overview

Four phases deliver a working Handoff server: clean infrastructure and auth, then session lifecycle and result delivery, then phone UI and actions, then the Go client library and dev tooling. Each phase delivers a coherent, independently verifiable capability.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation** - Clean Go server, infrastructure endpoints, API key auth, in-memory cache wired up
- [x] **Phase 2: Session Core** - Caller can create sessions, receive URLs, and retrieve results via poll or WebSocket
- [x] **Phase 3: Phone UI and Actions** - Phone user can open a session URL, see the action UI, and complete photo or signature actions
- [x] **Phase 4: Client Library and Dev Tools** - Go client library in pkg/ and mock consumer server for end-to-end testing

## Phase Details

### Phase 1: Foundation
**Goal**: A running Go server with infrastructure endpoints, API key authentication, and in-memory session store ready for use
**Depends on**: Nothing (first phase)
**Requirements**: INFR-01, INFR-02, INFR-03, INFR-04, AUTH-01, AUTH-02
**Success Criteria** (what must be TRUE):
  1. Server starts, responds to `GET /health` with status, and responds to `GET /version` with deployment version
  2. Server shuts down cleanly on SIGINT/SIGTERM without dropping in-flight requests
  3. A request with a valid API key is accepted; a request with a missing or invalid key is rejected with 401
  4. Static assets are served from the embedded binary (no external files required at runtime)
**Plans**: TBD

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
**Plans**: TBD

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
**Plans**: TBD

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
**Plans**: TBD

Plans:
- [x] 04-01: Go client library in pkg/ — types, errors, client with builder pattern, session with WebSocket subscription
- [x] 04-02: Mock consumer server — browser dashboard with session creation, QR display, live status, result preview

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 3/3 | Complete | 2026-02-26 |
| 2. Session Core | 4/4 | Complete | 2026-02-26 |
| 3. Phone UI and Actions | 4/4 | Complete | 2026-02-26 |
| 4. Client Library and Dev Tools | 2/2 | Complete | 2026-02-26 |
