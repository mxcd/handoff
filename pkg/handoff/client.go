package handoff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is the main entry point for interacting with a Handoff server.
// Create one with NewClient and use it to create sessions.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Client for the given Handoff server URL and API key.
// baseURL should be the root URL of the server, e.g., "https://handoff.example.com".
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an HTTP request with retry logic.
// Retries up to 3 times with exponential backoff on 5xx responses or network errors.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	delays := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}

	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		// If not the first attempt, wait with backoff
		if attempt > 0 {
			if attempt-1 < len(delays) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delays[attempt-1]):
				}
			}
		}

		// Create a fresh request (body may have been read)
		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, fmt.Errorf("handoff: failed to create request: %w", err)
		}
		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check for network/timeout errors — these are retryable
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				lastErr = err
				continue
			}
			// Other network errors are also retryable
			var opErr *net.OpError
			if errors.As(err, &opErr) {
				lastErr = err
				continue
			}
			return nil, err
		}

		// 5xx responses are retryable (except on last attempt)
		if resp.StatusCode >= 500 && attempt < 3 {
			resp.Body.Close()
			lastErr = fmt.Errorf("handoff: server error %d", resp.StatusCode)
			continue
		}

		// Non-2xx responses after retries exhausted → APIError
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, newAPIError(resp.StatusCode, bodyBytes)
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("handoff: request failed after retries: %w", lastErr)
	}
	return nil, fmt.Errorf("handoff: request failed after retries")
}

// SessionInfo contains information about a session retrieved from the server.
type SessionInfo struct {
	// ID is the unique session identifier.
	ID string
	// ActionType is the type of action requested.
	ActionType ActionType
	// Status is the current session status.
	Status SessionStatus
	// IntroText is the optional introductory text shown to the user.
	IntroText string
	// OutputFormat is the requested output format.
	OutputFormat OutputFormat
	// URL is the URL for the user to open on their phone.
	URL string
	// CreatedAt is when the session was created.
	CreatedAt time.Time
	// CompletedAt is when the session was completed (nil if not completed).
	CompletedAt *time.Time
	// Result contains the result items if the session is completed.
	Result []ResultItem
}

// GetSession retrieves the current state of a session by ID.
func (c *Client) GetSession(ctx context.Context, id string) (*SessionInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/sessions/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("handoff: failed to decode session response: %w", err)
	}

	return sessionResponseToInfo(&sr), nil
}

// DownloadFile downloads a result file by its download ID.
// Returns the file data, content type, and any error.
func (c *Client) DownloadFile(ctx context.Context, downloadID string) ([]byte, string, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/downloads/"+downloadID, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("handoff: failed to read file data: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return data, contentType, nil
}

// NewSession returns a new SessionBuilder for creating a session.
func (c *Client) NewSession() *SessionBuilder {
	return &SessionBuilder{client: c}
}

// SessionBuilder provides a fluent API for constructing and invoking sessions.
type SessionBuilder struct {
	client       *Client
	actionType   ActionType
	introText    string
	outputFormat OutputFormat
	sessionTTL   string
	resultTTL    string
}

// WithAction sets the action type for the session. This is required.
func (b *SessionBuilder) WithAction(actionType ActionType) *SessionBuilder {
	b.actionType = actionType
	return b
}

// WithIntro sets the introductory text shown to the user.
func (b *SessionBuilder) WithIntro(text string) *SessionBuilder {
	b.introText = text
	return b
}

// WithOutputFormat sets the desired output format. This is required.
func (b *SessionBuilder) WithOutputFormat(format OutputFormat) *SessionBuilder {
	b.outputFormat = format
	return b
}

// WithSessionTTL sets the session time-to-live as a duration string, e.g., "30m".
func (b *SessionBuilder) WithSessionTTL(ttl string) *SessionBuilder {
	b.sessionTTL = ttl
	return b
}

// WithResultTTL sets the result time-to-live as a duration string, e.g., "5m".
func (b *SessionBuilder) WithResultTTL(ttl string) *SessionBuilder {
	b.resultTTL = ttl
	return b
}

// Invoke validates the builder, creates the session on the server, and returns a
// connected Session object ready to receive events.
func (b *SessionBuilder) Invoke(ctx context.Context) (*Session, error) {
	if b.actionType == "" {
		return nil, fmt.Errorf("handoff: action type is required (use WithAction)")
	}
	if b.outputFormat == "" {
		return nil, fmt.Errorf("handoff: output format is required (use WithOutputFormat)")
	}

	reqBody := CreateSessionRequest{
		ActionType:   b.actionType,
		IntroText:    b.introText,
		OutputFormat: b.outputFormat,
		SessionTTL:   b.sessionTTL,
		ResultTTL:    b.resultTTL,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("handoff: failed to marshal request: %w", err)
	}

	resp, err := b.client.doRequest(ctx, http.MethodPost, "/api/v1/sessions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("handoff: failed to decode session response: %w", err)
	}

	session := newSession(b.client, &sr)
	go session.runWebSocket()
	return session, nil
}

// sessionResponseToInfo converts an internal sessionResponse to a public SessionInfo.
func sessionResponseToInfo(sr *sessionResponse) *SessionInfo {
	return &SessionInfo{
		ID:           sr.ID,
		ActionType:   sr.ActionType,
		Status:       sr.Status,
		IntroText:    sr.IntroText,
		OutputFormat: sr.OutputFormat,
		URL:          sr.URL,
		CreatedAt:    sr.CreatedAt,
		CompletedAt:  sr.CompletedAt,
		Result:       sr.Result,
	}
}
