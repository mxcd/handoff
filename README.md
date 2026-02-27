# Handoff

Handoff is a standalone Go server that lets backend applications collect photos, signatures, and document scans from phone users. A backend creates a session via API, the user completes the action on a phone-friendly web UI served by Handoff, and the backend retrieves the result via polling or WebSocket.

Everything runs in a single binary with no external dependencies — sessions and files are stored in memory with configurable TTLs.

## How it works

1. Your backend creates a session via the REST API (or Go client library)
2. You get back a URL and optionally generate a QR code
3. The user opens the URL on their phone and completes the action
4. Your backend receives the result via WebSocket push or polling

```
Backend                    Handoff Server                 Phone
  │                             │                           │
  ├─ POST /api/v1/sessions ────►│                           │
  │◄── session URL + ID ────────┤                           │
  │                             │                           │
  │  (display QR code)          │◄── user scans QR ─────────┤
  │                             │── render action UI ──────►│
  │◄── WebSocket: "opened" ─────┤                           │
  │◄── WebSocket: "completed" ──┤◄── submit result ─────────┤
  │                             │                           │
  ├─ GET /api/v1/downloads/:id ►│                           │
  │◄── file data ───────────────┤                           │
```

## Quick start

### Run with Docker

```bash
docker run -p 8080:8080 \
  -e API_KEYS=my-secret-key \
  -e BASE_URL=http://localhost:8080 \
  ghcr.io/mxcd/handoff:latest
```

### Build from source

```bash
go build -o server ./cmd/server

API_KEYS=my-secret-key BASE_URL=http://localhost:8080 ./server
```

## Configuration

All configuration is via environment variables. A `.env` file in the working directory is also loaded.

| Variable | Required | Default | Description |
|---|---|---|---|
| `API_KEYS` | Yes | — | Comma-separated list of API keys for authentication |
| `BASE_URL` | Yes | — | Public URL of the server, used to generate session URLs |
| `PORT` | No | `8080` | HTTP port |
| `DEV` | No | `false` | Enable development mode (colored log output) |
| `LOG_LEVEL` | No | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `SESSION_TTL` | No | `30m` | How long a session stays active (Go duration string) |
| `RESULT_TTL` | No | `5m` | How long result files are available after completion |
| `SCAN_UPLOAD_MAX_BYTES` | No | `20971520` | Max upload size per scan page (bytes) |
| `SCAN_MAX_PAGES` | No | `50` | Max pages per scan session |

## Action types

- **photo** — User takes a photo with their phone camera. Output formats: `jpg`, `png`, `pdf`.
- **signature** — User draws a signature on a touch-friendly pad. Output formats: `png`, `jpg`, `pdf`, `svg`.
- **scan** — User captures one or more document pages with perspective correction and multi-page assembly. Output formats: `pdf` (assembled per document) or `images` (individual pages). Supports `single` and `multi` document modes.

## Go client library

The `pkg/handoff` package provides a Go client with a fluent builder API, WebSocket event streaming with polling fallback, and automatic retries.

```bash
go get github.com/mxcd/handoff
```

Import:

```go
import "github.com/mxcd/handoff/pkg/handoff"
```

### Create a client

```go
client := handoff.NewClient("https://handoff.example.com", "my-api-key")
```

### Photo capture

```go
session, err := client.NewSession().
    WithAction(handoff.ActionTypePhoto).
    WithOutputFormat(handoff.OutputFormatJPG).
    WithIntro("Please take a photo of your ID card").
    WithSessionTTL("10m").
    WithResultTTL("5m").
    Invoke(ctx)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

// Generate a QR code for the user to scan (returns a data:image/png;base64,... URL)
qrDataURL, err := session.GenerateQR()

// Block until the user completes the action
items, err := session.WaitForResult(ctx)
if err != nil {
    log.Fatal(err)
}

// Download the photo
data, contentType, err := client.DownloadFile(ctx, items[0].DownloadID)
```

### Signature capture

```go
session, err := client.NewSession().
    WithAction(handoff.ActionTypeSignature).
    WithOutputFormat(handoff.OutputFormatSVG).
    Invoke(ctx)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

items, err := session.WaitForResult(ctx)
```

### Document scanning

```go
session, err := client.NewSession().
    WithAction(handoff.ActionTypeScan).
    WithScanOutputFormat(handoff.ScanOutputFormatPDF).
    WithDocumentMode(handoff.ScanDocumentModeMulti).
    Invoke(ctx)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

scanResult, err := session.WaitForScanResult(ctx)
if err != nil {
    log.Fatal(err)
}

// Each document has either a PDF URL or individual page image URLs
for _, doc := range scanResult.Documents {
    if doc.PDFURL != "" {
        data, _, err := client.DownloadFile(ctx, extractDownloadID(doc.PDFURL))
        // ... save PDF
    }
}
```

### Event streaming

Instead of blocking on `WaitForResult`, you can listen for real-time status updates:

```go
session.OnEvent(func(evt handoff.Event) {
    switch evt.Status {
    case handoff.SessionStatusOpened:
        fmt.Println("User opened the link")
    case handoff.SessionStatusActionStarted:
        fmt.Println("User started taking the photo")
    case handoff.SessionStatusCompleted:
        fmt.Println("Done!")
        for _, item := range evt.Result {
            fmt.Printf("  File: %s (%s)\n", item.Filename, item.ContentType)
        }
    }
})
```

The client connects via WebSocket for instant updates. If the WebSocket connection fails after 3 reconnection attempts, it falls back to polling every 2 seconds.

### Retrieving a session

```go
info, err := client.GetSession(ctx, "session-uuid")
if err != nil {
    if errors.Is(err, handoff.ErrNotFound) {
        // session doesn't exist
    }
    if errors.Is(err, handoff.ErrSessionExpired) {
        // session has expired
    }
}
fmt.Printf("Status: %s\n", info.Status)
```

### Error handling

The client returns sentinel errors that work with `errors.Is`:

```go
var apiErr *handoff.APIError
if errors.As(err, &apiErr) {
    fmt.Printf("HTTP %d: %s\n", apiErr.StatusCode, apiErr.Message)
}

errors.Is(err, handoff.ErrSessionExpired)  // 410 Gone
errors.Is(err, handoff.ErrNotFound)        // 404 Not Found
errors.Is(err, handoff.ErrUnauthorized)    // 401 Unauthorized
errors.Is(err, handoff.ErrConflict)        // 409 Conflict
```

HTTP requests retry up to 3 times on 5xx or network errors with exponential backoff.

## REST API

All API endpoints are under `/api/v1` and require an `X-API-Key` header.

### Create a session

```
POST /api/v1/sessions
```

```json
{
  "action_type": "photo",
  "output_format": "jpg",
  "intro_text": "Take a photo of your ID",
  "session_ttl": "30m",
  "result_ttl": "5m"
}
```

For scan sessions, `output_format` accepts `pdf` or `images`, and `document_mode` can be `single` (default) or `multi`.

Returns the full session object with `id`, `url`, and `status`.

### Get session status

```
GET /api/v1/sessions/:id
```

### Poll for results

```
GET /api/v1/sessions/:id/result
```

Returns `202 Accepted` while pending, `200 OK` with result data when completed, or `410 Gone` if expired.

### Download a file

```
GET /api/v1/downloads/:download_id
```

Returns the raw file with the appropriate `Content-Type` header.

### WebSocket

```
GET /api/v1/sessions/:id/ws
```

Authenticate with `X-API-Key` header or `api_key` query parameter. Receives JSON messages:

```json
{"type": "status_update", "session_id": "...", "status": "opened", "timestamp": "..."}
```

```json
{"type": "completed", "session_id": "...", "status": "completed", "data": [...], "timestamp": "..."}
```

### Health and version

```
GET /api/v1/health     →  {"status": "ok"}
GET /api/v1/version    →  {"version": "v1.0.0", "commit": "abc1234"}
```

These endpoints do not require authentication.

## Session lifecycle

Sessions progress through these states:

```
pending → opened → action_started → completed
                                  ↘ expired
```

- **pending** — Created, waiting for the user to open the URL
- **opened** — User opened the URL on their phone
- **action_started** — User began the action (camera opened, signature pad active, etc.)
- **completed** — Result submitted and available for download
- **expired** — Session TTL exceeded, no longer usable

After completion, result files remain available for the duration of `RESULT_TTL`. After that, downloads return 404.

## Development

```bash
# Run tests
go test -race ./...

# Lint
go vet ./...

# Hot-reload dev server (requires air)
just air

# Build with version info
go build -ldflags "-X github.com/mxcd/handoff/internal/util.Version=v1.0.0 \
  -X github.com/mxcd/handoff/internal/util.Commit=$(git rev-parse --short HEAD)" \
  -o server ./cmd/server
```

## License

See [LICENSE](LICENSE) for details.
