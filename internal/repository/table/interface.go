package table

import (
	"context"
	"time"
)

const (
	StatusLobby    = "lobby"
	StatusStarting = "starting"
	StatusInGame   = "in_game"

	MaxSeats = 6
	MinSeats = 2
)

// Seat is one of six table slots.
type Seat struct {
	SeatIndex  int        `bson:"seatIndex" json:"seatIndex"`
	UserID     string     `bson:"userId,omitempty" json:"userId,omitempty"`
	Username   string     `bson:"username,omitempty" json:"username,omitempty"`
	// PinColor is set on join — unique among occupied seats (lobby → game).
	PinColor   string     `bson:"pinColor,omitempty" json:"pinColor,omitempty"`
	Ready      bool       `bson:"ready" json:"ready"`
	Holding    bool       `bson:"holding" json:"holding"`
	HoldEndsAt *time.Time `bson:"holdEndsAt,omitempty" json:"holdEndsAt,omitempty"`
}

// Table is a matchmaking lobby / session for one World.
type Table struct {
	ID         string    `bson:"_id" json:"id"`
	WorldID    string    `bson:"worldId" json:"worldId"`
	Status     string    `bson:"status" json:"status"`
	Private    bool      `bson:"private" json:"private"`
	InviteCode string    `bson:"inviteCode,omitempty" json:"inviteCode,omitempty"`
	Seats      []Seat    `bson:"seats" json:"seats"`
	GameID     string    `bson:"gameId,omitempty" json:"gameId,omitempty"`
	CreatedAt  time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Repository persists tables.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Insert(ctx context.Context, t *Table) error
	Update(ctx context.Context, t *Table) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*Table, error)
	// FindOpenLobby returns a public lobby with free seats created after createdAfter
	// (Phase 20.6 — excludes TTL-expired lobbies when createdAfter is set).
	FindOpenLobby(ctx context.Context, worldID string, createdAfter time.Time) (*Table, error)
	FindLobbyByUser(ctx context.Context, userID string) (*Table, error)
	// FindByInviteCode looks up a private lobby by normalized invite code (Phase 20).
	FindByInviteCode(ctx context.Context, inviteCode string) (*Table, error)
	// ListUnstartedCreatedBefore returns lobby tables with no game created before `before` (Phase 20.6).
	ListUnstartedCreatedBefore(ctx context.Context, before time.Time) ([]*Table, error)
}
