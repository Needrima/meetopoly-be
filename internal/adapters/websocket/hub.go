package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	tablesvc "meetopoly-be/internal/services/table"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type client struct {
	conn    *websocket.Conn
	tableID string
	userID  string
	send    chan []byte
}

// Hub fans table events to connected WebSocket clients.
type Hub struct {
	mu     sync.RWMutex
	rooms  map[string]map[*client]struct{}
	tables tablesvc.Service
	authOK func(r *http.Request) (userID string, err error)
}

// NewHub builds a WS hub bound to the table service.
func NewHub(tables tablesvc.Service, authOK func(r *http.Request) (string, error)) *Hub {
	h := &Hub{
		rooms:  make(map[string]map[*client]struct{}),
		tables: tables,
		authOK: authOK,
	}
	tables.SetBroadcaster(h)
	return h
}

// Broadcast implements tablesvc.Broadcaster.
func (h *Hub) Broadcast(tableID string, ev tablesvc.Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.Error("ws marshal event", "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[tableID] {
		select {
		case c.send <- payload:
		default:
			slog.Warn("ws client send buffer full", "tableId", tableID, "userId", c.userID)
		}
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.tableID]
	if room == nil {
		room = make(map[*client]struct{})
		h.rooms[c.tableID] = room
	}
	room[c] = struct{}{}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[c.tableID]; ok {
		delete(room, c)
		if len(room) == 0 {
			delete(h.rooms, c.tableID)
		}
	}
}

type clientMessage struct {
	Type  string `json:"type"`
	Ready *bool  `json:"ready"`
}

// HandleTable upgrades to WebSocket for a table lobby.
// Auth: Authorization Bearer or ?token=.
func (h *Hub) HandleTable(w http.ResponseWriter, r *http.Request) {
	tableID := chi.URLParam(r, "tableId")
	userID, err := h.authOK(r)
	if err != nil || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if tableID == "" {
		http.Error(w, "missing table id", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}

	c := &client{
		conn:    conn,
		tableID: tableID,
		userID:  userID,
		send:    make(chan []byte, 16),
	}
	h.add(c)

	if view, err := h.tables.Get(r.Context(), tableID); err == nil {
		ev := tablesvc.Event{Type: "state", Table: view}
		if b, err := json.Marshal(ev); err == nil {
			select {
			case c.send <- b:
			default:
			}
		}
	}

	go c.writePump()
	c.readPump(h)
}


func (c *client) writePump() {
	defer func() { _ = c.conn.Close() }()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *client) readPump(h *Hub) {
	defer func() {
		h.remove(c)
		close(c.send)
		_ = c.conn.Close()
		_, _ = h.tables.Disconnect(context.Background(), c.tableID, c.userID)
	}()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "ready":
			ready := msg.Ready != nil && *msg.Ready
			_, _ = h.tables.SetReady(context.Background(), c.tableID, c.userID, ready)
		case "leave":
			_, _ = h.tables.Leave(context.Background(), c.tableID, c.userID)
			return
		case "ping":
			pong, _ := json.Marshal(tablesvc.Event{Type: "pong"})
			select {
			case c.send <- pong:
			default:
			}
		}
	}
}
