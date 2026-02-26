package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ActionType represents the type of action a session requires from the phone user.
type ActionType string

const (
	ActionTypePhoto     ActionType = "photo"
	ActionTypeSignature ActionType = "signature"
)

// ValidateActionType returns the typed ActionType value or an error for unknown types.
func ValidateActionType(s string) (ActionType, error) {
	switch ActionType(s) {
	case ActionTypePhoto:
		return ActionTypePhoto, nil
	case ActionTypeSignature:
		return ActionTypeSignature, nil
	default:
		return "", fmt.Errorf("unknown action type %q: must be 'photo' or 'signature'", s)
	}
}

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	// SessionStatusPending — session created, URL not yet opened by phone user.
	SessionStatusPending SessionStatus = "pending"
	// SessionStatusOpened — phone user opened the session URL.
	SessionStatusOpened SessionStatus = "opened"
	// SessionStatusActionStarted — phone user started the action (e.g., began capturing photo).
	SessionStatusActionStarted SessionStatus = "action_started"
	// SessionStatusCompleted — action finished, result is available.
	SessionStatusCompleted SessionStatus = "completed"
	// SessionStatusExpired — session TTL passed; session is no longer usable.
	SessionStatusExpired SessionStatus = "expired"
)

// OutputFormat represents the file format for the action result.
type OutputFormat string

const (
	OutputFormatJPG OutputFormat = "jpg"
	OutputFormatPNG OutputFormat = "png"
	OutputFormatPDF OutputFormat = "pdf"
	OutputFormatSVG OutputFormat = "svg"
)

// ValidateOutputFormat validates that format is a valid output format for the given action type.
// For photo: accepts "jpg", "png", "pdf".
// For signature: accepts "svg", "png", "pdf".
func ValidateOutputFormat(actionType ActionType, format string) (OutputFormat, error) {
	switch actionType {
	case ActionTypePhoto:
		switch OutputFormat(format) {
		case OutputFormatJPG, OutputFormatPNG, OutputFormatPDF:
			return OutputFormat(format), nil
		default:
			return "", fmt.Errorf("invalid output format %q for action type 'photo': must be 'jpg', 'png', or 'pdf'", format)
		}
	case ActionTypeSignature:
		switch OutputFormat(format) {
		case OutputFormatSVG, OutputFormatPNG, OutputFormatPDF:
			return OutputFormat(format), nil
		default:
			return "", fmt.Errorf("invalid output format %q for action type 'signature': must be 'svg', 'png', or 'pdf'", format)
		}
	default:
		return "", fmt.Errorf("unknown action type %q", actionType)
	}
}

// ResultItem represents a single result file from a completed session action.
type ResultItem struct {
	// DownloadID is a UUID used to reference this file via the download endpoint.
	DownloadID string `json:"download_id"`
	// ContentType is the MIME type of the file (e.g., "image/jpeg").
	ContentType string `json:"content_type"`
	// Filename is the suggested file name for the result file.
	Filename string `json:"filename"`
}

// Session represents a handoff session created by a backend application.
type Session struct {
	// ID is a UUIDv4 uniquely identifying this session.
	ID string `json:"id"`
	// ActionType is the action the phone user must complete.
	ActionType ActionType `json:"action_type"`
	// Status is the current lifecycle state of the session.
	Status SessionStatus `json:"status"`
	// IntroText is optional Markdown displayed to the phone user on the session page.
	IntroText string `json:"intro_text,omitempty"`
	// OutputFormat specifies the file format for the result.
	OutputFormat OutputFormat `json:"output_format"`
	// SessionTTL is how long the session remains active before expiring.
	SessionTTL time.Duration `json:"session_ttl"`
	// ResultTTL is how long the result files are available for download after completion.
	ResultTTL time.Duration `json:"result_ttl"`
	// URL is the phone-friendly URL the end user opens to complete the action.
	URL string `json:"url"`
	// CreatedAt is the time the session was created.
	CreatedAt time.Time `json:"created_at"`
	// CompletedAt is set when the session reaches the "completed" status.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Result holds the list of result files once the session is completed.
	Result []ResultItem `json:"result,omitempty"`
	// Opened is an internal flag used to track one-time-use session URL access.
	Opened bool `json:"-"`
}

// NewSessionID returns a new UUIDv4 string for use as a session ID.
func NewSessionID() string {
	return uuid.New().String()
}
