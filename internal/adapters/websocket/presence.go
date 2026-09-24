package websocket

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	rtcadapter "meetopoly-be/internal/adapters/webrtc"
	gamesvc "meetopoly-be/internal/services/game"
)

type presenceClient struct {
	conn     *websocket.Conn
	gameID   string
	roomID   string
	userID   string
	username string
	send     chan []byte
	hub      *PresenceHub
}

func (c *presenceClient) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	select {
	case c.send <- b:
		return nil
	default:
		return errSendBufferFull
	}
}

type sendBufferFullError struct{}

func (sendBufferFullError) Error() string { return "presence send buffer full" }

var errSendBufferFull = sendBufferFullError{}

// PresenceHub is Phase 7.0 board-presence signaling + Pion SFU join/leave.
type PresenceHub struct {
	mu     sync.RWMutex
	rooms  map[string]map[*presenceClient]struct{} // roomID → clients
	games  gamesvc.Service
	sfu    *rtcadapter.SFU
	authOK func(r *http.Request) (userID string, err error)
}

// NewPresenceHub builds the board presence signaling hub.
func NewPresenceHub(games gamesvc.Service, authOK func(r *http.Request) (string, error)) *PresenceHub {
	return &PresenceHub{
		rooms:  make(map[string]map[*presenceClient]struct{}),
		games:  games,
		sfu:    rtcadapter.NewSFU(),
		authOK: authOK,
	}
}

func (h *PresenceHub) add(c *presenceClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.roomID]
	if room == nil {
		room = make(map[*presenceClient]struct{})
		h.rooms[c.roomID] = room
	}
	// Reconnect: drop prior sockets for the same user so roster/toasts stay single-homed.
	for old := range room {
		if old.userID == c.userID && old != c {
			delete(room, old)
			go func(prev *presenceClient) {
				_ = prev.conn.Close()
			}(old)
		}
	}
	room[c] = struct{}{}
}

func (h *PresenceHub) remove(c *presenceClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[c.roomID]; ok {
		delete(room, c)
		if len(room) == 0 {
			delete(h.rooms, c.roomID)
		}
	}
}

func (h *PresenceHub) broadcast(roomID string, excludeUserID string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[roomID] {
		if excludeUserID != "" && c.userID == excludeUserID {
			continue
		}
		select {
		case c.send <- b:
		default:
			slog.Warn("presence ws send buffer full", "roomId", roomID, "userId", c.userID)
		}
	}
}

type presenceInMessage struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

// HandleBoardPresence upgrades to WebSocket for board:{gameId} presence.
// Auth: Bearer or ?token=; caller must be a seated game player.
func (h *PresenceHub) HandleBoardPresence(w http.ResponseWriter, r *http.Request) {
	gameID := chi.URLParam(r, "gameId")
	userID, err := h.authOK(r)
	if err != nil || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if gameID == "" {
		http.Error(w, "missing game id", http.StatusBadRequest)
		return
	}

	view, err := h.games.Get(r.Context(), gameID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	username, ok := playerUsername(view, userID)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("presence ws upgrade", "err", err)
		return
	}

	roomID := rtcadapter.BoardRoomID(gameID)
	c := &presenceClient{
		conn:     conn,
		gameID:   gameID,
		roomID:   roomID,
		userID:   userID,
		username: username,
		send:     make(chan []byte, 32),
		hub:      h,
	}

	alreadyPresent := false
	for _, p := range h.sfu.Roster(roomID, "") {
		if p.UserID == userID {
			alreadyPresent = true
			break
		}
	}

	if err := h.sfu.Attach(roomID, userID, username, c); err != nil {
		slog.Error("presence sfu attach", "err", err)
		_ = conn.Close()
		return
	}

	h.add(c)

	welcome := map[string]any{
		"type":       "welcome",
		"roomId":     roomID,
		"userId":     userID,
		"username":   username,
		"peers":      h.sfu.Roster(roomID, userID),
		"iceServers": h.sfu.ICEServersJSON(),
	}
	if b, err := json.Marshal(welcome); err == nil {
		select {
		case c.send <- b:
		default:
		}
	}

	if !alreadyPresent {
		h.broadcast(roomID, userID, map[string]any{
			"type":     "peer-joined",
			"userId":   userID,
			"username": username,
		})
	}

	go c.writePump()
	c.readPump()
}

func playerUsername(view *gamesvc.View, userID string) (string, bool) {
	if view == nil {
		return "", false
	}
	for _, p := range view.Players {
		if p.UserID == userID {
			name := p.Username
			if name == "" {
				name = "Player"
			}
			return name, true
		}
	}
	return "", false
}

func (c *presenceClient) writePump() {
	defer func() { _ = c.conn.Close() }()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *presenceClient) readPump() {
	defer func() {
		info, ok := c.hub.sfu.Detach(c.roomID, c.userID, c)
		c.hub.remove(c)
		close(c.send)
		_ = c.conn.Close()
		if ok {
			c.hub.broadcast(c.roomID, c.userID, map[string]any{
				"type":     "peer-left",
				"userId":   info.UserID,
				"username": info.Username,
			})
		}
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg presenceInMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "ping":
			pong, _ := json.Marshal(map[string]any{"type": "pong"})
			select {
			case c.send <- pong:
			default:
			}
		case "offer":
			if msg.SDP == "" {
				c.sendError("missing sdp")
				continue
			}
			if err := c.hub.sfu.HandleOffer(c.roomID, c.userID, msg.SDP); err != nil {
				slog.Warn("presence offer failed", "userId", c.userID, "err", err)
				c.sendError("offer failed")
			}
		case "ice":
			if len(msg.Candidate) == 0 || string(msg.Candidate) == "null" {
				continue
			}
			var cand webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &cand); err != nil {
				continue
			}
			if err := c.hub.sfu.AddICE(c.roomID, c.userID, cand); err != nil {
				slog.Debug("presence ice add", "userId", c.userID, "err", err)
			}
		}
	}
}

func (c *presenceClient) sendError(message string) {
	b, _ := json.Marshal(map[string]any{"type": "error", "message": message})
	select {
	case c.send <- b:
	default:
	}
}
