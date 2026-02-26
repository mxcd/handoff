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
			web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "Something went wrong. Please try again.",
			})
			return
		}

		// Session never existed
		if session == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusNotFound)
			web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "This session was not found.",
			})
			return
		}

		// Expired session
		if session.Status == model.SessionStatusExpired {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusGone)
			web.RenderPage(c.Writer, "expired.html", nil)
			return
		}

		// Already completed
		if session.Status == model.SessionStatusCompleted {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusOK)
			web.RenderPage(c.Writer, "success.html", nil)
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
			web.RenderPage(c.Writer, "intro.html", map[string]interface{}{
				"IntroText": session.IntroText,
				"ActionURL": actionURL,
			})
			return
		}

		// No intro — go directly to the action page
		s.renderActionPage(c, session)
	}
}

// renderActionPage renders the correct action-specific template based on session.ActionType.
func (s *Server) renderActionPage(c *gin.Context, session *model.Session) {
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
	default:
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusInternalServerError)
		web.RenderPage(c.Writer, "error.html", map[string]interface{}{
			"Message": "Unknown action type.",
		})
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
			web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "Something went wrong. Please try again.",
			})
			return
		}

		if session == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusNotFound)
			web.RenderPage(c.Writer, "error.html", map[string]interface{}{
				"Message": "This session was not found.",
			})
			return
		}

		if session.Status == model.SessionStatusExpired {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusGone)
			web.RenderPage(c.Writer, "expired.html", nil)
			return
		}

		if session.Status == model.SessionStatusCompleted {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Status(http.StatusOK)
			web.RenderPage(c.Writer, "success.html", nil)
			return
		}

		s.renderActionPage(c, session)
	}
}
