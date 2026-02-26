// Package handoff provides a Go client library for interacting with a Handoff server.
// Go applications can create sessions, receive events, and retrieve results without
// writing raw HTTP calls.
package handoff

import "time"

// ActionType represents the type of action a session requests from the user.
type ActionType string

const (
	// ActionTypePhoto requests the user to take a photo.
	ActionTypePhoto ActionType = "photo"
	// ActionTypeSignature requests the user to provide a signature.
	ActionTypeSignature ActionType = "signature"
)

// OutputFormat represents the desired output format for a session result.
type OutputFormat string

const (
	// OutputFormatJPG requests JPEG output.
	OutputFormatJPG OutputFormat = "jpg"
	// OutputFormatPNG requests PNG output.
	OutputFormatPNG OutputFormat = "png"
	// OutputFormatPDF requests PDF output.
	OutputFormatPDF OutputFormat = "pdf"
	// OutputFormatSVG requests SVG output.
	OutputFormatSVG OutputFormat = "svg"
)

// SessionStatus represents the current status of a session.
type SessionStatus string

const (
	// SessionStatusPending means the session has been created but not yet opened by the user.
	SessionStatusPending SessionStatus = "pending"
	// SessionStatusOpened means the user has opened the session URL.
	SessionStatusOpened SessionStatus = "opened"
	// SessionStatusActionStarted means the user has started the action (e.g., taking a photo).
	SessionStatusActionStarted SessionStatus = "action_started"
	// SessionStatusCompleted means the user has completed the action and submitted the result.
	SessionStatusCompleted SessionStatus = "completed"
	// SessionStatusExpired means the session has expired.
	SessionStatusExpired SessionStatus = "expired"
)

// ResultItem represents a single result file from a completed session.
type ResultItem struct {
	// DownloadID is the identifier used to download the file.
	DownloadID string `json:"download_id"`
	// ContentType is the MIME type of the file.
	ContentType string `json:"content_type"`
	// Filename is the suggested filename for the file.
	Filename string `json:"filename"`
}

// Event represents a state change event received from the server.
type Event struct {
	// Type is the event type: "status_update" or "completed".
	Type string
	// SessionID is the ID of the session this event belongs to.
	SessionID string
	// Status is the current status of the session.
	Status SessionStatus
	// Result contains the result items when Type is "completed".
	Result []ResultItem
	// Timestamp is when the event occurred.
	Timestamp time.Time
}

// CreateSessionRequest is used to create a new session.
type CreateSessionRequest struct {
	// ActionType is the type of action to request from the user (required).
	ActionType ActionType `json:"action_type"`
	// IntroText is optional introductory text shown to the user.
	IntroText string `json:"intro_text,omitempty"`
	// OutputFormat is the desired output format for the result (required).
	OutputFormat OutputFormat `json:"output_format"`
	// SessionTTL is the session time-to-live as a duration string, e.g., "30m".
	SessionTTL string `json:"session_ttl,omitempty"`
	// ResultTTL is the result time-to-live as a duration string, e.g., "5m".
	ResultTTL string `json:"result_ttl,omitempty"`
}

// sessionResponse is the internal representation of the server's session JSON response.
type sessionResponse struct {
	ID           string        `json:"id"`
	ActionType   ActionType    `json:"action_type"`
	Status       SessionStatus `json:"status"`
	IntroText    string        `json:"intro_text,omitempty"`
	OutputFormat OutputFormat  `json:"output_format"`
	SessionTTL   int64         `json:"session_ttl"`
	ResultTTL    int64         `json:"result_ttl"`
	URL          string        `json:"url"`
	CreatedAt    time.Time     `json:"created_at"`
	CompletedAt  *time.Time    `json:"completed_at,omitempty"`
	Result       []ResultItem  `json:"result,omitempty"`
}

// resultPollResponse is the response from GET /api/v1/sessions/:id/result.
type resultPollResponse struct {
	Status      string       `json:"status"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	Items       []ResultItem `json:"items"`
}
