package handoff

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

// Session represents a live Handoff session. It provides the URL for the user,
// a QR code helper, event subscription, and result retrieval.
type Session struct {
	// ID is the unique session identifier.
	ID string
	// URL is the URL the user should open on their phone.
	URL string
	// Status is the current status of the session.
	Status SessionStatus
	// ActionType is the type of action requested.
	ActionType ActionType
	// OutputFormat is the requested output format.
	OutputFormat OutputFormat

	client     *Client
	done       chan struct{}
	resultCh   chan []ResultItem
	mu         sync.Mutex
	callbacks  []func(Event)
	wsConn     *websocket.Conn
	closed     bool
	scanResult *ScanResult
}

// wsMessage is the incoming WebSocket message shape from the server.
type wsMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// newSession creates a Session from a server response without starting any goroutines.
func newSession(client *Client, sr *sessionResponse) *Session {
	return &Session{
		ID:           sr.ID,
		URL:          sr.URL,
		Status:       sr.Status,
		ActionType:   sr.ActionType,
		OutputFormat: sr.OutputFormat,
		client:       client,
		done:         make(chan struct{}),
		resultCh:     make(chan []ResultItem, 1),
	}
}

// GenerateQR generates a QR code PNG for the session URL and returns it as a
// base64-encoded data URL (data:image/png;base64,...).
func (s *Session) GenerateQR() (string, error) {
	png, err := qrcode.Encode(s.URL, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("handoff: failed to generate QR code: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(png)
	return "data:image/png;base64," + encoded, nil
}

// OnEvent registers a callback that is called for every future event received.
// Thread-safe — may be called from multiple goroutines. Callbacks registered
// after the session completes will not receive past events.
func (s *Session) OnEvent(callback func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// WaitForResult blocks until the session is completed or the context is cancelled.
// Returns the result items on completion.
func (s *Session) WaitForResult(ctx context.Context) ([]ResultItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case items, ok := <-s.resultCh:
		if !ok {
			return nil, fmt.Errorf("handoff: session closed before completion")
		}
		return items, nil
	}
}

// WaitForScanResult blocks until the session is completed or the context is cancelled.
// Returns the scan result on completion. Use this for scan sessions instead of WaitForResult.
func (s *Session) WaitForScanResult(ctx context.Context) (*ScanResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case _, ok := <-s.resultCh:
		if !ok {
			return nil, fmt.Errorf("handoff: session closed before completion")
		}
		s.mu.Lock()
		sr := s.scanResult
		s.mu.Unlock()
		if sr == nil {
			return nil, fmt.Errorf("handoff: no scan result available")
		}
		return sr, nil
	}
}

// Close stops the WebSocket connection and any background goroutines.
// It is idempotent — calling Close multiple times is safe.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	close(s.done)

	if s.wsConn != nil {
		_ = s.wsConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		return s.wsConn.Close()
	}
	return nil
}

// dispatchEvent calls all registered callbacks with the given event.
func (s *Session) dispatchEvent(evt Event) {
	s.mu.Lock()
	cbs := make([]func(Event), len(s.callbacks))
	copy(cbs, s.callbacks)
	s.mu.Unlock()

	for _, cb := range cbs {
		cb(evt)
	}
}

// isClosed returns true if the session has been closed.
func (s *Session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// wsURL converts the HTTP base URL to a WebSocket URL and appends the session path.
func (s *Session) wsURL() string {
	base := s.client.baseURL
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	return fmt.Sprintf("%s/api/v1/sessions/%s/ws?api_key=%s", base, s.ID, s.client.apiKey)
}

// runWebSocket is the main WebSocket goroutine. It attempts to connect and read messages,
// reconnecting up to 3 times with backoff on failure. Falls back to polling after that.
func (s *Session) runWebSocket() {
	const maxReconnects = 3
	reconnectDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt < maxReconnects; attempt++ {
		if s.isClosed() {
			return
		}

		if attempt > 0 {
			delay := reconnectDelays[attempt-1]
			select {
			case <-s.done:
				return
			case <-time.After(delay):
			}
		}

		conn, _, err := websocket.DefaultDialer.Dial(s.wsURL(), nil)
		if err != nil {
			continue
		}

		s.mu.Lock()
		s.wsConn = conn
		s.mu.Unlock()

		completed := s.readWebSocketMessages(conn)
		conn.Close()

		s.mu.Lock()
		s.wsConn = nil
		s.mu.Unlock()

		if completed || s.isClosed() {
			return
		}
	}

	// All reconnect attempts exhausted — fall back to polling
	s.runPolling()
}

// readWebSocketMessages reads messages from the WebSocket connection until it closes.
// Returns true if the session completed, false if the connection was lost.
func (s *Session) readWebSocketMessages(conn *websocket.Conn) bool {
	for {
		select {
		case <-s.done:
			return false
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return false
		}

		var msg wsMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		evt := Event{
			Type:      msg.Type,
			SessionID: msg.SessionID,
			Status:    SessionStatus(msg.Status),
			Timestamp: msg.Timestamp,
		}

		if msg.Type == "completed" && len(msg.Data) > 0 {
			// Try standard result items first (photo/signature sessions).
			var items []ResultItem
			if err := json.Unmarshal(msg.Data, &items); err == nil && len(items) > 0 {
				evt.Result = items
			} else {
				// Try scan result (scan sessions).
				var scanResult ScanResult
				if err := json.Unmarshal(msg.Data, &scanResult); err == nil && len(scanResult.Documents) > 0 {
					s.mu.Lock()
					s.scanResult = &scanResult
					s.mu.Unlock()
					evt.ScanResult = &scanResult
				}
			}
			s.dispatchEvent(evt)
			select {
			case s.resultCh <- evt.Result:
			default:
			}
			return true
		}

		s.dispatchEvent(evt)
	}
}

// runPolling polls the result endpoint every 2 seconds as a fallback when WebSocket fails.
func (s *Session) runPolling() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			items, done, err := s.pollResult()
			if err != nil {
				continue
			}
			if done {
				evt := Event{
					Type:      "completed",
					SessionID: s.ID,
					Status:    SessionStatusCompleted,
					Result:    items,
					Timestamp: time.Now().UTC(),
				}
				s.mu.Lock()
				evt.ScanResult = s.scanResult
				s.mu.Unlock()
				s.dispatchEvent(evt)
				select {
				case s.resultCh <- items:
				default:
				}
				return
			}
		}
	}
}

// pollResult queries the result endpoint once and reports whether the session is complete.
func (s *Session) pollResult() ([]ResultItem, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.client.doRequest(ctx, http.MethodGet, "/api/v1/sessions/"+s.ID+"/result", nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	// 202 Accepted means still pending
	if resp.StatusCode == http.StatusAccepted {
		return nil, false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("handoff: failed to read poll response: %w", err)
	}

	var pollResp resultPollResponse
	if err := json.Unmarshal(body, &pollResp); err != nil {
		return nil, false, fmt.Errorf("handoff: failed to decode poll response: %w", err)
	}

	if SessionStatus(pollResp.Status) == SessionStatusCompleted {
		if pollResp.ScanResult != nil {
			s.mu.Lock()
			s.scanResult = pollResp.ScanResult
			s.mu.Unlock()
		}
		return pollResp.Items, true, nil
	}

	return nil, false, nil
}
