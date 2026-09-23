package game

import (
	"context"
	"time"
)

const (
	StatusActive = "active"

	StartingCash = 2000
	PassGoBonus  = 200 // applied in later slices when passing GO
	GoBoardIndex = 0
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

// Game is the authoritative M1 session (Phase 6+).
type Game struct {
	ID           string    `bson:"_id" json:"id"`
	TableID      string    `bson:"tableId" json:"tableId"`
	WorldID      string    `bson:"worldId" json:"worldId"`
	Status       string    `bson:"status" json:"status"`
	Players      []Player  `bson:"players" json:"players"`
	CurrentTurn  int       `bson:"currentTurn" json:"currentTurn"` // turnOrder of active player
	PassGoBonus  int       `bson:"passGoBonus" json:"passGoBonus"`
	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Repository persists games.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Insert(ctx context.Context, g *Game) error
	FindByID(ctx context.Context, id string) (*Game, error)
	FindByTableID(ctx context.Context, tableID string) (*Game, error)
}
