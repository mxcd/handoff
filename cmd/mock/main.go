// Package main implements a mock consumer server that demonstrates the full Handoff
// e2e flow via a browser-based dashboard. It uses the pkg/handoff client library
// as its only integration point with the Handoff server.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	_ "embed"

	"github.com/mxcd/handoff/pkg/handoff"
)

//go:embed templates/dashboard.html
var dashboardHTML string

// sessionEntry holds an active session and its context cancel function.
type sessionEntry struct {
	session *handoff.Session
	cancel  context.CancelFunc
}

// createSessionRequest is the JSON body for POST /api/create-session.
type createSessionRequest struct {
	ActionType       string `json:"action_type"`
	OutputFormat     string `json:"output_format"`
	IntroText        string `json:"intro_text"`
	DocumentMode     string `json:"document_mode,omitempty"`
	ScanOutputFormat string `json:"scan_output_format,omitempty"`
}

// createSessionResponse is the JSON response for POST /api/create-session.
type createSessionResponse struct {
	SessionID  string `json:"session_id"`
	URL        string `json:"url"`
	QRDataURL  string `json:"qr_data_url"`
	Status     string `json:"status"`
	ActionType string `json:"action_type"`
}

// ssePreviewEvent is a SSE event payload sent to the browser on completion.
type ssePreviewEvent struct {
	Type        string           `json:"type"`
	Status      string           `json:"status"`
	Result      []handoff.ResultItem `json:"result,omitempty"`
	PreviewData []previewItem    `json:"preview_data,omitempty"`
}

// previewItem holds base64-encoded file data for browser preview.
type previewItem struct {
	DownloadID  string `json:"download_id"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
	DataURL     string `json:"data_url"`
}

// dashboardTemplateData is passed to the dashboard HTML template.
type dashboardTemplateData struct {
	HandoffURL string
	MockPort   string
}

// server holds the mock consumer state.
type server struct {
	handoffURL string
	apiKey     string
	mockPort   string
	client     *handoff.Client
	sessions   sync.Map // sessionID -> *sessionEntry
}

func main() {
	// Parse configuration from env vars with flag fallbacks.
	handoffURL := os.Getenv("HANDOFF_URL")
	if handoffURL == "" {
		handoffURL = "http://localhost:8080"
	}
	apiKey := os.Getenv("HANDOFF_API_KEY")
	if apiKey == "" {
		apiKey = "test-key"
	}
	mockPort := os.Getenv("MOCK_PORT")
	if mockPort == "" {
		mockPort = "9090"
	}

	flag.StringVar(&handoffURL, "handoff-url", handoffURL, "Handoff server base URL")
	flag.StringVar(&apiKey, "api-key", apiKey, "Handoff server API key")
	flag.StringVar(&mockPort, "port", mockPort, "Mock dashboard port")
	flag.Parse()

	s := &server{
		handoffURL: strings.TrimRight(handoffURL, "/"),
		apiKey:     apiKey,
		mockPort:   mockPort,
		client:     handoff.NewClient(handoffURL, apiKey),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/create-session", s.handleCreateSession)
	mux.HandleFunc("/api/session/", s.handleSessionRoutes)

	httpServer := &http.Server{
		Addr:    ":" + mockPort,
		Handler: mux,
	}

	dashboardURL := fmt.Sprintf("http://localhost:%s", mockPort)
	log.Printf("Handoff mock dashboard starting on %s", dashboardURL)
	log.Printf("Connected to Handoff server: %s", handoffURL)

	// Handle graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		log.Println("Shutting down mock consumer...")
		// Close all active sessions.
		s.sessions.Range(func(key, value any) bool {
			if entry, ok := value.(*sessionEntry); ok {
				entry.cancel()
				if err := entry.session.Close(); err != nil {
					log.Printf("Error closing session %v: %v", key, err)
				}
			}
			return true
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	// Open browser after a short delay to let the server start.
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(dashboardURL)
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		log.Printf("Cannot auto-open browser on %s. Visit: %s", runtime.GOOS, url)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v. Visit: %s", err, url)
	}
}

// handleDashboard renders and serves the main dashboard HTML page.
func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		log.Printf("Template parse error: %v", err)
		return
	}

	data := dashboardTemplateData{
		HandoffURL: s.handoffURL,
		MockPort:   s.mockPort,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execute error: %v", err)
	}
}

// handleCreateSession handles POST /api/create-session.
func (s *server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate action type.
	var actionType handoff.ActionType
	switch req.ActionType {
	case "photo":
		actionType = handoff.ActionTypePhoto
	case "signature":
		actionType = handoff.ActionTypeSignature
	case "scan":
		actionType = handoff.ActionTypeScan
	default:
		jsonError(w, http.StatusBadRequest, "invalid action_type: must be 'photo', 'signature', or 'scan'")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	builder := s.client.NewSession().WithAction(actionType)

	if actionType == handoff.ActionTypeScan {
		// Scan sessions use document_mode and scan_output_format instead of output_format.
		var docMode handoff.ScanDocumentMode
		switch req.DocumentMode {
		case "single":
			docMode = handoff.ScanDocumentModeSingle
		case "multi":
			docMode = handoff.ScanDocumentModeMulti
		default:
			docMode = handoff.ScanDocumentModeSingle
		}
		builder = builder.WithDocumentMode(docMode)

		var scanFmt handoff.ScanOutputFormat
		switch req.ScanOutputFormat {
		case "images":
			scanFmt = handoff.ScanOutputFormatImages
		default:
			scanFmt = handoff.ScanOutputFormatPDF
		}
		builder = builder.WithScanOutputFormat(scanFmt)
	} else {
		// Photo/signature sessions use output_format.
		var outputFormat handoff.OutputFormat
		switch req.OutputFormat {
		case "jpg":
			outputFormat = handoff.OutputFormatJPG
		case "png":
			outputFormat = handoff.OutputFormatPNG
		case "pdf":
			outputFormat = handoff.OutputFormatPDF
		case "svg":
			outputFormat = handoff.OutputFormatSVG
		default:
			jsonError(w, http.StatusBadRequest, "invalid output_format: must be jpg, png, pdf, or svg")
			return
		}
		builder = builder.WithOutputFormat(outputFormat)
	}

	if req.IntroText != "" {
		builder = builder.WithIntro(req.IntroText)
	}

	session, err := builder.Invoke(ctx)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		jsonError(w, http.StatusBadGateway, fmt.Sprintf("failed to create session: %v", err))
		return
	}

	qrDataURL, err := session.GenerateQR()
	if err != nil {
		session.Close()
		log.Printf("Failed to generate QR: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to generate QR code")
		return
	}

	// Store the session for the SSE endpoint.
	sessionCtx, sessionCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.sessions.Store(session.ID, &sessionEntry{
		session: session,
		cancel:  sessionCancel,
	})

	// Clean up session when done.
	go func() {
		<-sessionCtx.Done()
		s.sessions.Delete(session.ID)
	}()

	resp := createSessionResponse{
		SessionID:  session.ID,
		URL:        session.URL,
		QRDataURL:  qrDataURL,
		Status:     string(session.Status),
		ActionType: string(session.ActionType),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSessionRoutes dispatches /api/session/:id/... routes.
func (s *server) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	// Path format: /api/session/{id}/events or /api/session/{id}/download/{download_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/session/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	sessionID := parts[0]
	action := parts[1]

	switch action {
	case "events":
		s.handleSessionEvents(w, r, sessionID)
	case "download":
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		s.handleSessionDownload(w, r, parts[2])
	default:
		http.NotFound(w, r)
	}
}

// handleSessionEvents handles GET /api/session/:id/events as Server-Sent Events.
func (s *server) handleSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	value, ok := s.sessions.Load(sessionID)
	if !ok {
		jsonError(w, http.StatusNotFound, "session not found")
		return
	}
	entry, ok := value.(*sessionEntry)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// eventCh receives events from the session callback.
	eventCh := make(chan handoff.Event, 10)
	entry.session.OnEvent(func(event handoff.Event) {
		select {
		case eventCh <- event:
		default:
			// Drop if channel is full (shouldn't happen in practice).
		}
	})

	sendSSE := func(data any) {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
	}

	// Send initial status event so the browser knows the session started.
	sendSSE(ssePreviewEvent{
		Type:   "status_update",
		Status: string(entry.session.Status),
	})

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-eventCh:
			if !ok {
				return
			}

			if evt.Type == "completed" {
				// Download all result files for preview.
				var previews []previewItem
				downloadCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

				if evt.ScanResult != nil && len(evt.ScanResult.Documents) > 0 {
					// Scan session: extract download URLs from ScanResult.
					for di, doc := range evt.ScanResult.Documents {
						if doc.PDFURL != "" {
							dlID := extractDownloadID(doc.PDFURL)
							filename := fmt.Sprintf("document_%d.pdf", di+1)
							data, ct, err := s.client.DownloadFile(downloadCtx, dlID)
							if err != nil {
								log.Printf("Failed to download scan PDF %s: %v", dlID, err)
								previews = append(previews, previewItem{
									DownloadID: dlID, ContentType: "application/pdf", Filename: filename,
								})
								continue
							}
							encoded := base64.StdEncoding.EncodeToString(data)
							previews = append(previews, previewItem{
								DownloadID: dlID, ContentType: ct, Filename: filename,
								DataURL: fmt.Sprintf("data:%s;base64,%s", ct, encoded),
							})
						}
						for pi, page := range doc.Pages {
							dlID := extractDownloadID(page.URL)
							filename := fmt.Sprintf("document_%d_page_%d.jpg", di+1, pi+1)
							data, ct, err := s.client.DownloadFile(downloadCtx, dlID)
							if err != nil {
								log.Printf("Failed to download scan page %s: %v", dlID, err)
								previews = append(previews, previewItem{
									DownloadID: dlID, ContentType: page.ContentType, Filename: filename,
								})
								continue
							}
							encoded := base64.StdEncoding.EncodeToString(data)
							previews = append(previews, previewItem{
								DownloadID: dlID, ContentType: ct, Filename: filename,
								DataURL: fmt.Sprintf("data:%s;base64,%s", ct, encoded),
							})
						}
					}
				} else {
					// Photo/signature session: use ResultItem list.
					for _, item := range evt.Result {
						data, contentType, err := s.client.DownloadFile(downloadCtx, item.DownloadID)
						if err != nil {
							log.Printf("Failed to download result %s: %v", item.DownloadID, err)
							previews = append(previews, previewItem{
								DownloadID:  item.DownloadID,
								ContentType: item.ContentType,
								Filename:    item.Filename,
							})
							continue
						}
						encoded := base64.StdEncoding.EncodeToString(data)
						dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, encoded)
						previews = append(previews, previewItem{
							DownloadID:  item.DownloadID,
							ContentType: contentType,
							Filename:    item.Filename,
							DataURL:     dataURL,
						})
					}
				}
				cancel()

				sendSSE(ssePreviewEvent{
					Type:        "completed",
					Status:      string(evt.Status),
					Result:      evt.Result,
					PreviewData: previews,
				})

				// Clean up session after completion.
				entry.cancel()
				s.sessions.Delete(sessionID)
				return
			}

			// Status update event.
			sendSSE(ssePreviewEvent{
				Type:   evt.Type,
				Status: string(evt.Status),
			})
		}
	}
}

// handleSessionDownload handles GET /api/session/:id/download/:download_id.
func (s *server) handleSessionDownload(w http.ResponseWriter, r *http.Request, downloadID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	data, contentType, err := s.client.DownloadFile(ctx, downloadID)
	if err != nil {
		log.Printf("Failed to download file %s: %v", downloadID, err)
		jsonError(w, http.StatusBadGateway, "failed to download file")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

// extractDownloadID extracts the download ID from a URL like "/api/v1/downloads/{id}".
func extractDownloadID(url string) string {
	const prefix = "/api/v1/downloads/"
	if idx := strings.LastIndex(url, prefix); idx >= 0 {
		return url[idx+len(prefix):]
	}
	// If it doesn't match the expected pattern, return as-is (might already be an ID).
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		return url[idx+1:]
	}
	return url
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
