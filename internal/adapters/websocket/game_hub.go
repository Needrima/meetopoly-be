package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	gamesvc "meetopoly-be/internal/services/game"
)

type gameClient struct {
	conn   *websocket.Conn
	gameID string
	userID string
	send   chan []byte
}

// GameHub fans game events to connected WebSocket clients.
type GameHub struct {
	mu     sync.RWMutex
	rooms  map[string]map[*gameClient]struct{}
	games  gamesvc.Service
	authOK func(r *http.Request) (userID string, err error)
}

// NewGameHub builds a WS hub bound to the game service.
func NewGameHub(games gamesvc.Service, authOK func(r *http.Request) (string, error)) *GameHub {
	h := &GameHub{
		rooms:  make(map[string]map[*gameClient]struct{}),
		games:  games,
		authOK: authOK,
	}
	games.SetBroadcaster(h)
	return h
}

// Broadcast implements gamesvc.Broadcaster.
func (h *GameHub) Broadcast(gameID string, ev gamesvc.Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.Error("game ws marshal event", "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[gameID] {
		select {
		case c.send <- payload:
		default:
			slog.Warn("game ws client send buffer full", "gameId", gameID, "userId", c.userID)
		}
	}
}

func (h *GameHub) add(c *gameClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.gameID]
	if room == nil {
		room = make(map[*gameClient]struct{})
		h.rooms[c.gameID] = room
	}
	room[c] = struct{}{}
}

func (h *GameHub) remove(c *gameClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[c.gameID]; ok {
		delete(room, c)
		if len(room) == 0 {
			delete(h.rooms, c.gameID)
		}
	}
}

type gameClientMessage struct {
	Type string `json:"type"`
}

// HandleGame upgrades to WebSocket for an in-progress game.
// Auth: Authorization Bearer or ?token=. Caller must be a seated player.
func (h *GameHub) HandleGame(w http.ResponseWriter, r *http.Request) {
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
	if !playerInGame(view, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("game ws upgrade", "err", err)
		return
	}

	c := &gameClient{
		conn:   conn,
		gameID: gameID,
		userID: userID,
		send:   make(chan []byte, 16),
	}
	h.add(c)
	// Reconnect within hold cancels auto-resign (Phase 7.5).
	h.games.CancelDisconnectHold(gameID, userID)

	ev := gamesvc.Event{Type: "state", Game: view}
	if b, err := json.Marshal(ev); err == nil {
		select {
		case c.send <- b:
		default:
		}
	}

	go c.writePump()
	c.readPump(h)
}

func playerInGame(view *gamesvc.View, userID string) bool {
	if view == nil {
		return false
	}
	for _, p := range view.Players {
		if p.UserID == userID {
			return true
		}
	}
	return false
}

func (c *gameClient) writePump() {
	defer func() { _ = c.conn.Close() }()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *gameClient) readPump(h *GameHub) {
	defer func() {
		h.remove(c)
		close(c.send)
		_ = c.conn.Close()
		// Phase 7.5: last game WS for this user → silent disconnect hold → Resign.
		if !h.userHasConnection(c.gameID, c.userID) {
			if err := h.games.Disconnect(context.Background(), c.gameID, c.userID); err != nil {
				slog.Warn("game disconnect hold",
					"gameId", c.gameID,
					"userId", c.userID,
					"err", err,
				)
			}
		}
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg gameClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type == "ping" {
			pong, _ := json.Marshal(gamesvc.Event{Type: "pong"})
			select {
			case c.send <- pong:
			default:
			}
		}
	}
}

func (h *GameHub) userHasConnection(gameID, userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[gameID] {
		if c.userID == userID {
			return true
		}
	}
	return false
}
