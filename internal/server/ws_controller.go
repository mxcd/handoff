package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mxcd/go-config/config"
	handoffws "github.com/mxcd/handoff/internal/ws"
	"github.com/mxcd/handoff/internal/model"
	"github.com/rs/zerolog/log"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins
}

// wsHandler returns the WebSocket upgrade handler for per-session real-time notifications.
// GET /api/v1/sessions/:id/ws
//
// API key authentication is performed before the WebSocket upgrade.
// The key may be supplied via the X-API-Key header or the api_key query parameter.
func (s *Server) wsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		// Validate API key before upgrade — supports header and query param.
		key := c.GetHeader("X-API-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key == "" {
			jsonError(c, http.StatusUnauthorized, "missing API key")
			return
		}
		validKeys := config.Get().StringArray("API_KEYS")
		valid := false
		for _, k := range validKeys {
			if key == k {
				valid = true
				break
			}
		}
		if !valid {
			jsonError(c, http.StatusUnauthorized, "invalid API key")
			return
		}

		// Verify session exists and is not expired.
		session, err := s.Store.GetSession(id)
		if err != nil {
			log.Error().Err(err).Str("session_id", id).Msg("ws: failed to get session")
			jsonError(c, http.StatusInternalServerError, "internal error")
			return
		}
		if session == nil {
			jsonError(c, http.StatusNotFound, "session not found")
			return
		}
		if session.Status == model.SessionStatusExpired {
			jsonError(c, http.StatusGone, "session expired")
			return
		}

		// Upgrade the HTTP connection to WebSocket.
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			// Upgrader already wrote an error response.
			log.Debug().Err(err).Str("session_id", id).Msg("ws: upgrade failed")
			return
		}

		s.Hub.Subscribe(id, conn)
		log.Info().Str("session_id", id).Msg("ws: client connected")

		// Send the current status immediately so the client knows where things stand.
		initialMsg := handoffws.WSMessage{
			Type:      "status_update",
			SessionID: id,
			Status:    string(session.Status),
			Timestamp: time.Now(),
		}
		if writeErr := conn.WriteJSON(initialMsg); writeErr != nil {
			log.Debug().Err(writeErr).Str("session_id", id).Msg("ws: failed to send initial status")
			s.Hub.Unsubscribe(id, conn)
			conn.Close()
			return
		}

		// Read loop — block until the client disconnects.
		defer func() {
			s.Hub.Unsubscribe(id, conn)
			conn.Close()
			log.Info().Str("session_id", id).Msg("ws: client disconnected")
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break // client disconnected or error
			}
		}
	}
}
