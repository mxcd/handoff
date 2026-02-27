package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mxcd/handoff/internal/model"
	"github.com/mxcd/handoff/internal/web"
	"github.com/rs/zerolog/log"
)

// sessionPageHandler returns the handler for GET /s/:id
// This is the main phone-user entry point. It looks up the session and renders
// the appropriate HTML page based on the session's current state.
func (s *Server) sessionPageHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("session_page: store error")
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusInternalServerError)
			if err := web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "Something went wrong. Please try again.",
			}); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: error template render error")
			}
			return
		}

		// Session never existed
		if session == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusNotFound)
			if err := web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "This session was not found.",
			}); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: error template render error")
			}
			return
		}

		// Expired session
		if session.Status == model.SessionStatusExpired {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusGone)
			if err := web.RenderPage(c.Writer, "expired.html", nil); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: expired template render error")
			}
			return
		}

		// Already completed
		if session.Status == model.SessionStatusCompleted {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusOK)
			if err := web.RenderPage(c.Writer, "success.html", nil); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: success template render error")
			}
			return
		}

		// Mark session as opened if this is the first visit
		if session.Status == model.SessionStatusPending {
			if err := s.Store.MarkSessionOpened(id); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: failed to mark opened")
				// Non-fatal — continue rendering the page
			}
			// Broadcast status update via WebSocket
			s.Hub.BroadcastStatusUpdate(id, string(model.SessionStatusOpened))
		}

		// Determine what to show: intro page or action page
		// If intro text is configured, show intro page with Continue button
		if session.IntroText != "" {
			actionURL := fmt.Sprintf("/s/%s/action", id)
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusOK)
			if err := web.RenderPage(c.Writer, "intro.html", map[string]interface{}{
				"IntroText": session.IntroText,
				"ActionURL": actionURL,
			}); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_page: intro template render error")
			}
			return
		}

		// No intro — go directly to the action page
		s.renderActionPage(c, session)
	}
}

// renderActionPage renders the correct action-specific template based on session.ActionType.
// It also advances the session status to action_started if it is currently opened.
func (s *Server) renderActionPage(c *gin.Context, session *model.Session) {
	// Advance to action_started once the user reaches the action UI
	if session.Status == model.SessionStatusOpened || session.Status == model.SessionStatusPending {
		session.Status = model.SessionStatusActionStarted
		if err := s.Store.UpdateSession(session); err != nil {
			log.Error().Err(err).Str("session_id", session.ID).Msg("session_page: failed to update action_started")
		}
		s.Hub.BroadcastStatusUpdate(session.ID, string(model.SessionStatusActionStarted))
	}

	data := map[string]interface{}{
		"SessionID":    session.ID,
		"ActionType":   string(session.ActionType),
		"OutputFormat": string(session.OutputFormat),
		"SubmitURL":    fmt.Sprintf("/s/%s/result", session.ID),
	}

	var templateName string
	switch session.ActionType {
	case model.ActionTypePhoto:
		templateName = "action_photo.html"
	case model.ActionTypeSignature:
		templateName = "action_signature.html"
	case model.ActionTypeScan:
		data["ScanDocumentMode"] = string(session.ScanDocumentMode)
		data["ScanUploadURL"] = fmt.Sprintf("/s/%s/scan/upload", session.ID)
		data["ScanFinalizeURL"] = fmt.Sprintf("/s/%s/scan/finalize", session.ID)
		templateName = "action_scan.html"
	default:
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusInternalServerError)
		if err := web.RenderPage(c.Writer, "error.html", map[string]interface{}{
			"Message": "Unknown action type.",
		}); err != nil {
			log.Error().Err(err).Str("session_id", session.ID).Msg("session_page: error template render error")
		}
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	if err := web.RenderPage(c.Writer, templateName, data); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("session_page: template render error")
	}
}

// sessionActionHandler returns the handler for GET /s/:id/action
// This route is used when the intro page has a Continue button that navigates here.
func (s *Server) sessionActionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("session_action: store error")
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusInternalServerError)
			if err := web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "Something went wrong. Please try again.",
			}); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_action: error template render error")
			}
			return
		}

		if session == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusNotFound)
			if err := web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "This session was not found.",
			}); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_action: error template render error")
			}
			return
		}

		if session.Status == model.SessionStatusExpired {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusGone)
			if err := web.RenderPage(c.Writer, "expired.html", nil); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_action: expired template render error")
			}
			return
		}

		if session.Status == model.SessionStatusCompleted {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusOK)
			if err := web.RenderPage(c.Writer, "success.html", nil); err != nil {
				log.Error().Err(err).Str("session_id", id).Msg("session_action: success template render error")
			}
			return
		}

		s.renderActionPage(c, session)
	}
}
