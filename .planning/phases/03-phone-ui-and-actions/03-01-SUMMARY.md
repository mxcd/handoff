---
phase: 03-phone-ui-and-actions
plan: 01
subsystem: ui
tags: [html/template, embed.FS, gin, phone-ui, session-routing]

# Dependency graph
requires:
  - phase: 02-session-core
    provides: Store.GetSession, Store.MarkSessionOpened, model.Session, ws.Hub.BroadcastStatusUpdate
provides:
  - Go html/template system with base layout and block-override page templates
  - RenderPage function that clones base and overlays page-specific content block
  - GET /s/:id route that dispatches to correct template based on session state
  - GET /s/:id/action route for post-intro continuation
  - Expired, success, error, intro, action_photo, action_signature templates embedded in binary
affects: [03-02, 03-03, 03-04]

# Tech tracking
tech-stack:
  added: [html/template]
  patterns: [base template with block overrides via clone-parse-execute, page templates define only content/styles/scripts blocks]

key-files:
  created:
    - internal/web/templates/base.html
    - internal/web/templates/expired.html
    - internal/web/templates/success.html
    - internal/web/templates/error.html
    - internal/web/templates/intro.html
    - internal/web/templates/action_photo.html
    - internal/web/templates/action_signature.html
    - internal/server/session_page_controller.go
  modified:
    - internal/web/web.go
    - internal/server/server.go

key-decisions:
  - "Template rendering uses clone-parse-execute pattern: base.html parsed fresh each call, page template parsed into clone — enables block overrides without global template set"
  - "baseTemplateContent loaded once at init() and reused per-request for efficiency"
  - "Content-Type set per-response before c.Status() to ensure correct header ordering with gin"
  - "MarkSessionOpened failure is non-fatal — page renders regardless so phone user is not blocked"
  - "sessionActionHandler is a separate route (GET /s/:id/action) for post-intro continuation without re-triggering opened logic"

patterns-established:
  - "Page templates contain only {{define}} blocks (content, styles, scripts) — base.html owns full HTML structure"
  - "RenderPage is the single rendering entry point in web package — controllers call web.RenderPage(c.Writer, name, data)"
  - "All HTML responses set Content-Type text/html; charset=utf-8 explicitly before c.Status()"

requirements-completed: ["UI-01", "UI-04"]

# Metrics
duration: 2min
completed: 2026-02-26
---

# Phase 3 Plan 01: Phone UI Template System and Session Routing Summary

**Go html/template system with base layout, block-override page templates, and GET /s/:id routing that dispatches by session state and marks sessions opened**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-26T20:29:11Z
- **Completed:** 2026-02-26T20:31:00Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Template system using html/template with base layout and block-override page files embedded via embed.FS
- RenderPage function providing safe clone-parse-execute rendering for any page template
- Session page controller routing phone users to expired/success/error/intro/action templates based on session state
- WebSocket status broadcast triggered when session transitions from pending to opened on first visit

## Task Commits

Each task was committed atomically:

1. **Task 1: Create HTML templates and update web.go for template loading** - `c6e5300` (feat)
2. **Task 2: Create session page controller and wire route into server** - `ef5dd61` (feat)

## Files Created/Modified
- `internal/web/templates/base.html` - Base HTML layout with viewport meta, inline CSS, and content/styles/scripts blocks
- `internal/web/templates/expired.html` - Expired session page with clock icon and "This session has expired" message
- `internal/web/templates/success.html` - Success page with green circle SVG checkmark and "Done!" message
- `internal/web/templates/error.html` - Generic error page rendering .Message data field
- `internal/web/templates/intro.html` - Intro page with IntroText and Continue button to /s/:id/action
- `internal/web/templates/action_photo.html` - Placeholder for Plan 03-03
- `internal/web/templates/action_signature.html` - Placeholder for Plan 03-04
- `internal/web/web.go` - Added templateFS embed, init() to load base template, RenderPage function
- `internal/server/session_page_controller.go` - sessionPageHandler, sessionActionHandler, renderActionPage
- `internal/server/server.go` - Registered GET /s/:id and GET /s/:id/action routes

## Decisions Made
- Template rendering uses clone-parse-execute: `base.html` is parsed fresh each call and page template parsed into the clone. This enables `{{define "content"}}` blocks to override `{{block "content"}}` placeholders without managing a global template set.
- `baseTemplateContent` loaded once at `init()` and reused per request to avoid repeated FS reads.
- Content-Type header set per-response before `c.Status()` to ensure correct header ordering with gin.
- `MarkSessionOpened` failure is non-fatal — the page renders regardless so the phone user is not blocked by a cache write error.
- `sessionActionHandler` is a separate route that does not re-trigger the opened status broadcast, preventing duplicate events when the user navigates from intro to action.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Template system and routing infrastructure complete — Plans 03-02, 03-03, 03-04 can implement intro page, photo capture, and signature pad by replacing placeholder templates and adding handlers.
- Action templates (action_photo.html, action_signature.html) are placeholders that compile and serve minimal content until their respective plans implement them.

---
*Phase: 03-phone-ui-and-actions*
*Completed: 2026-02-26*
