package table

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("table not found")
	ErrFull         = errors.New("table is full")
	ErrNotSeated    = errors.New("not seated at this table")
	ErrNeedPlayers  = errors.New("need at least 2 players to ready")
	ErrWrongStatus  = errors.New("table is not accepting lobby actions")
	ErrInvalidWorld = errors.New("invalid world id")
)

// SeatView is the public seat shape for HTTP/WS.
type SeatView struct {
	SeatIndex  int     `json:"seatIndex"`
	UserID     *string `json:"userId"`
	Username   *string `json:"username"`
	PinColor   *string `json:"pinColor"`
	Ready      bool    `json:"ready"`
	Holding    bool    `json:"holding"`
	HoldEndsAt *string `json:"holdEndsAt"`
}

// View is the public table snapshot.
type View struct {
	ID      string     `json:"id"`
	WorldID string     `json:"worldId"`
	Status  string     `json:"status"`
	Seats   []SeatView `json:"seats"`
	GameID  *string    `json:"gameId"`
}

// Event is pushed to WebSocket subscribers.
type Event struct {
	Type  string `json:"type"` // state | started | error
	Table *View  `json:"table,omitempty"`
	Error string `json:"error,omitempty"`
}

// Broadcaster fans table events to WS clients.
type Broadcaster interface {
	Broadcast(tableID string, ev Event)
}

// GameStarter creates an M1 game when the lobby is ready to start.
type GameStarter interface {
	StartFromTable(ctx context.Context, tableID, worldID string, seats []SeatView) (gameID string, err error)
}

// Config tunes lobby behaviour.
type Config struct {
	DisconnectHold time.Duration
}

// Service is the table/matchmaking application port.
type Service interface {
	Join(ctx context.Context, userID, username, worldID string) (*View, error)
	Get(ctx context.Context, tableID string) (*View, error)
	SetReady(ctx context.Context, tableID, userID string, ready bool) (*View, error)
	Leave(ctx context.Context, tableID, userID string) (*View, error)
	Disconnect(ctx context.Context, tableID, userID string) (*View, error)
	SetBroadcaster(b Broadcaster)
	SetGameStarter(g GameStarter)
}
