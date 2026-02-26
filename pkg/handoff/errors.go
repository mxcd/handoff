package handoff

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Sentinel errors for common API error conditions.
var (
	// ErrSessionExpired is returned when the server responds with 410 Gone.
	ErrSessionExpired = errors.New("handoff: session expired")
	// ErrNotFound is returned when the server responds with 404 Not Found.
	ErrNotFound = errors.New("handoff: session not found")
	// ErrUnauthorized is returned when the server responds with 401 Unauthorized.
	ErrUnauthorized = errors.New("handoff: unauthorized")
	// ErrConflict is returned when the server responds with 409 Conflict.
	ErrConflict = errors.New("handoff: conflict")
)

// APIError represents an error returned by the Handoff API.
// It implements the error interface and supports errors.Is() via Unwrap().
type APIError struct {
	// StatusCode is the HTTP status code returned by the server.
	StatusCode int
	// Message is the error message from the server.
	Message string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("handoff: API error %d: %s", e.StatusCode, e.Message)
}

// Unwrap returns the matching sentinel error for the status code,
// enabling errors.Is() checks against ErrSessionExpired, ErrNotFound, etc.
func (e *APIError) Unwrap() error {
	switch e.StatusCode {
	case 410:
		return ErrSessionExpired
	case 404:
		return ErrNotFound
	case 401:
		return ErrUnauthorized
	case 409:
		return ErrConflict
	default:
		return nil
	}
}

// serverErrorResponse is used to parse the server's {"error": "..."} JSON body.
type serverErrorResponse struct {
	Error string `json:"error"`
}

// newAPIError parses the server error response body and returns a wrapped *APIError.
func newAPIError(statusCode int, body []byte) error {
	var resp serverErrorResponse
	msg := string(body)
	if err := json.Unmarshal(body, &resp); err == nil && resp.Error != "" {
		msg = resp.Error
	}
	return &APIError{
		StatusCode: statusCode,
		Message:    msg,
	}
}
