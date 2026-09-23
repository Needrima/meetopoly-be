package game

import (
	"context"
	"time"
)

const (
	StatusActive = "active"

	StartingCash   = 2000
	PassGoBonus    = 200
	GoBoardIndex   = 0
	BoardSpaceCount = 40
)

// Player is one seated participant in an M1 game.
type Player struct {
	UserID     string `bson:"userId" json:"userId"`
	Username   string `bson:"username" json:"username"`
	SeatIndex  int    `bson:"seatIndex" json:"seatIndex"`
	TurnOrder  int    `bson:"turnOrder" json:"turnOrder"`
	Cash       int    `bson:"cash" json:"cash"`
	BoardIndex int    `bson:"boardIndex" json:"boardIndex"`
	PinColor   string `bson:"pinColor" json:"pinColor"`
}

// LastRoll is the most recent dice result (Phase 6.1+).
type LastRoll struct {
	UserID       string `bson:"userId" json:"userId"`
	Username     string `bson:"username" json:"username"`
	Die1         int    `bson:"die1" json:"die1"`
	Die2         int    `bson:"die2" json:"die2"`
	Total        int    `bson:"total" json:"total"`
	FromIndex    int    `bson:"fromIndex" json:"fromIndex"`
	ToIndex      int    `bson:"toIndex" json:"toIndex"`
	PassedGo     bool   `bson:"passedGo" json:"passedGo"`
	PassGoAmount int    `bson:"passGoAmount,omitempty" json:"passGoAmount,omitempty"`
}

// Game is the authoritative M1 session (Phase 6+).
type Game struct {
	ID          string    `bson:"_id" json:"id"`
	TableID     string    `bson:"tableId" json:"tableId"`
	WorldID     string    `bson:"worldId" json:"worldId"`
	Status      string    `bson:"status" json:"status"`
	Players     []Player  `bson:"players" json:"players"`
	CurrentTurn int       `bson:"currentTurn" json:"currentTurn"` // turnOrder of active player
	PassGoBonus int       `bson:"passGoBonus" json:"passGoBonus"`
	LastRoll    *LastRoll `bson:"lastRoll,omitempty" json:"lastRoll,omitempty"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Repository persists games.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Insert(ctx context.Context, g *Game) error
	Update(ctx context.Context, g *Game) error
	FindByID(ctx context.Context, id string) (*Game, error)
	FindByTableID(ctx context.Context, tableID string) (*Game, error)
}
