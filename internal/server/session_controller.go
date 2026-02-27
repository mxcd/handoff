package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/go-config/config"
	"github.com/mxcd/handoff/internal/model"
	"github.com/rs/zerolog/log"
)

// createSessionRequest is the JSON body for POST /api/v1/sessions.
type createSessionRequest struct {
	ActionType   string `json:"action_type" binding:"required"`
	IntroText    string `json:"intro_text"`
	OutputFormat string `json:"output_format"` // required for photo/signature; optional for scan (defaults to "pdf")
	DocumentMode string `json:"document_mode"` // scan only: "single" (default) or "multi"
	SessionTTL   string `json:"session_ttl"`   // optional, e.g. "30m", "1h"
	ResultTTL    string `json:"result_ttl"`    // optional, e.g. "5m", "10m"
}

// createSessionHandler returns a gin.HandlerFunc that creates a new session.
// POST /api/v1/sessions
func (s *Server) createSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createSessionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		actionType, err := model.ValidateActionType(req.ActionType)
		if err != nil {
			jsonError(c, http.StatusBadRequest, err.Error())
			return
		}

		// Parse SessionTTL — use request value if provided, else fall back to config default.
		var sessionTTL time.Duration
		if req.SessionTTL != "" {
			sessionTTL, err = time.ParseDuration(req.SessionTTL)
			if err != nil {
				jsonError(c, http.StatusBadRequest, "invalid session_ttl: "+err.Error())
				return
			}
		} else {
			sessionTTL, err = time.ParseDuration(config.Get().String("SESSION_TTL"))
			if err != nil {
				log.Error().Err(err).Msg("session_controller: failed to parse SESSION_TTL config")
				jsonError(c, http.StatusInternalServerError, "internal configuration error")
				return
			}
		}

		// Parse ResultTTL — use request value if provided, else fall back to config default.
		var resultTTL time.Duration
		if req.ResultTTL != "" {
			resultTTL, err = time.ParseDuration(req.ResultTTL)
			if err != nil {
				jsonError(c, http.StatusBadRequest, "invalid result_ttl: "+err.Error())
				return
			}
		} else {
			resultTTL, err = time.ParseDuration(config.Get().String("RESULT_TTL"))
			if err != nil {
				log.Error().Err(err).Msg("session_controller: failed to parse RESULT_TTL config")
				jsonError(c, http.StatusInternalServerError, "internal configuration error")
				return
			}
		}

		sessionID := model.NewSessionID()
		sessionURL := config.Get().String("BASE_URL") + "/s/" + sessionID

		var session model.Session

		if actionType == model.ActionTypeScan {
			// Scan sessions use their own document_mode and scan output format fields.

			docModeStr := req.DocumentMode
			if docModeStr == "" {
				docModeStr = "single" // default
			}
			docMode, err := model.ValidateScanDocumentMode(docModeStr)
			if err != nil {
				jsonError(c, http.StatusBadRequest, err.Error())
				return
			}

			scanFmtStr := req.OutputFormat
			if scanFmtStr == "" {
				scanFmtStr = "pdf" // default
			}
			scanFmt, err := model.ValidateScanOutputFormat(scanFmtStr)
			if err != nil {
				jsonError(c, http.StatusBadRequest, err.Error())
				return
			}

			session = model.Session{
				ID:               sessionID,
				ActionType:       actionType,
				Status:           model.SessionStatusPending,
				IntroText:        req.IntroText,
				ScanDocumentMode: docMode,
				ScanOutputFormat: scanFmt,
				SessionTTL:       sessionTTL,
				ResultTTL:        resultTTL,
				URL:              sessionURL,
				CreatedAt:        time.Now(),
			}
		} else {
			// Photo and signature sessions require a valid output_format.
			if req.OutputFormat == "" {
				jsonError(c, http.StatusBadRequest, "output_format is required")
				return
			}
			outputFormat, err := model.ValidateOutputFormat(actionType, req.OutputFormat)
			if err != nil {
				jsonError(c, http.StatusBadRequest, err.Error())
				return
			}

			session = model.Session{
				ID:           sessionID,
				ActionType:   actionType,
				Status:       model.SessionStatusPending,
				IntroText:    req.IntroText,
				OutputFormat: outputFormat,
				SessionTTL:   sessionTTL,
				ResultTTL:    resultTTL,
				URL:          sessionURL,
				CreatedAt:    time.Now(),
			}
		}

		if err := s.Store.CreateSession(&session); err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("session_controller: failed to create session")
			jsonError(c, http.StatusInternalServerError, "failed to create session")
			return
		}

		log.Info().Str("session_id", sessionID).Str("action_type", string(actionType)).Msg("session_controller: session created")
		c.JSON(http.StatusCreated, session)
	}
}

// getSessionHandler returns a gin.HandlerFunc that retrieves a session by ID.
// GET /api/v1/sessions/:id
func (s *Server) getSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("session_controller: failed to retrieve session")
			jsonError(c, http.StatusInternalServerError, "failed to retrieve session")
			return
		}

		if session == nil {
			jsonError(c, http.StatusNotFound, "session not found")
			return
		}

		// Return minimal tombstone payload for expired sessions.
		if session.Status == model.SessionStatusExpired {
			c.JSON(http.StatusOK, gin.H{
				"id":     session.ID,
				"status": "expired",
			})
			return
		}

		c.JSON(http.StatusOK, session)
	}
}
