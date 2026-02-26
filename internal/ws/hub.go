package ws

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const writeDeadline = 10 * time.Second

// WSMessage is the envelope sent to all WebSocket subscribers of a session.
type WSMessage struct {
	Type      string      `json:"type"`             // "status_update" or "completed"
	SessionID string      `json:"session_id"`
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`   // result metadata on completion
	Timestamp time.Time   `json:"timestamp"`
}

// Hub manages per-session WebSocket subscriber lists.
// All methods are safe for concurrent use.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]*websocket.Conn // session ID -> list of WS connections
}

// NewHub creates and returns an initialised Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string][]*websocket.Conn),
	}
}

// Subscribe adds conn to the subscriber list for the given session.
func (h *Hub) Subscribe(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[sessionID] = append(h.subscribers[sessionID], conn)
	log.Debug().
		Str("session_id", sessionID).
		Int("total_subscribers", len(h.subscribers[sessionID])).
		Msg("ws: client subscribed")
}

// Unsubscribe removes conn from the subscriber list for the given session.
// If the list becomes empty the map entry is deleted.
func (h *Hub) Unsubscribe(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unsubscribeLocked(sessionID, conn)
}

// unsubscribeLocked removes conn; caller must hold h.mu (write lock).
func (h *Hub) unsubscribeLocked(sessionID string, conn *websocket.Conn) {
	conns := h.subscribers[sessionID]
	filtered := conns[:0]
	for _, c := range conns {
		if c != conn {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		delete(h.subscribers, sessionID)
	} else {
		h.subscribers[sessionID] = filtered
	}
	log.Debug().
		Str("session_id", sessionID).
		Int("remaining_subscribers", len(filtered)).
		Msg("ws: client unsubscribed")
}

// Broadcast sends msg as JSON to all subscribers of the session.
// Connections that fail to receive the message are unsubscribed and closed.
func (h *Hub) Broadcast(sessionID string, msg WSMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.subscribers[sessionID]
	if len(conns) == 0 {
		return
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("type", msg.Type).
		Int("subscribers", len(conns)).
		Msg("ws: broadcasting message")

	var failed []*websocket.Conn
	for _, conn := range conns {
		if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
			failed = append(failed, conn)
			continue
		}
		if err := conn.WriteJSON(msg); err != nil {
			log.Debug().Err(err).Str("session_id", sessionID).Msg("ws: write failed, dropping subscriber")
			failed = append(failed, conn)
		}
	}

	for _, conn := range failed {
		h.unsubscribeLocked(sessionID, conn)
		conn.Close()
	}
}

// BroadcastStatusUpdate is a convenience method that sends a "status_update" message
// to all subscribers of the session.
func (h *Hub) BroadcastStatusUpdate(sessionID string, status string) {
	h.Broadcast(sessionID, WSMessage{
		Type:      "status_update",
		SessionID: sessionID,
		Status:    status,
		Timestamp: time.Now(),
	})
}

// BroadcastCompletion sends a "completed" message that includes full result metadata
// to all subscribers of the session.
func (h *Hub) BroadcastCompletion(sessionID string, status string, resultItems interface{}) {
	h.Broadcast(sessionID, WSMessage{
		Type:      "completed",
		SessionID: sessionID,
		Status:    status,
		Data:      resultItems,
		Timestamp: time.Now(),
	})
}

// CloseSession closes all connections for a session and removes the map entry.
func (h *Hub) CloseSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.subscribers[sessionID]
	for _, conn := range conns {
		conn.Close()
	}
	delete(h.subscribers, sessionID)
	log.Debug().Str("session_id", sessionID).Int("closed", len(conns)).Msg("ws: session closed, all subscribers disconnected")
}
