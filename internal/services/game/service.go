package game

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	ErrNotFound      = errors.New("game not found")
	ErrAlreadyExists = errors.New("game already exists for table")
	ErrNeedPlayers   = errors.New("need at least 2 players to start")
)

// SeatInput is a seated lobby player used to bootstrap a game.
type SeatInput struct {
	UserID    string
	Username  string
	SeatIndex int
}

// PlayerView is the public player shape.
type PlayerView struct {
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	SeatIndex  int    `json:"seatIndex"`
	TurnOrder  int    `json:"turnOrder"`
	Cash       int    `json:"cash"`
	BoardIndex int    `json:"boardIndex"`
	PinColor   string `json:"pinColor"`
}

// View is the public game snapshot (Phase 6.0).
type View struct {
	ID              string       `json:"id"`
	TableID         string       `json:"tableId"`
	WorldID         string       `json:"worldId"`
	Status          string       `json:"status"`
	Players         []PlayerView `json:"players"`
	CurrentTurn     int          `json:"currentTurn"`
	CurrentUserID   string       `json:"currentUserId"`
	CurrentUsername string       `json:"currentUsername"`
	PassGoBonus     int          `json:"passGoBonus"`
	Currency        string       `json:"currency"`
}

// Service is the game application port.
type Service interface {
	CreateFromSeats(ctx context.Context, tableID, worldID string, seats []SeatInput) (*View, error)
	Get(ctx context.Context, gameID string) (*View, error)
	GetByTableID(ctx context.Context, tableID string) (*View, error)
}

// Classic Monopoly token-ish palette for pins (hex).
var pinPalette = []string{
	"#ED1B24", // red
	"#0072BB", // dark blue
	"#1FB25A", // green
	"#F7941D", // orange
	"#D93A96", // pink
	"#FEF200", // yellow
}

type service struct {
	repo gamerepo.Repository
}

// New builds a game Service.
func New(repo gamerepo.Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateFromSeats(ctx context.Context, tableID, worldID string, seats []SeatInput) (*View, error) {
	if existing, err := s.repo.FindByTableID(ctx, tableID); err == nil {
		return toView(existing), nil
	} else if !errors.Is(err, gamerepo.ErrNotFound) {
		return nil, err
	}

	occupied := make([]SeatInput, 0, len(seats))
	for _, seat := range seats {
		if seat.UserID == "" {
			continue
		}
		occupied = append(occupied, seat)
	}
	if len(occupied) < 2 {
		return nil, ErrNeedPlayers
	}
	sort.Slice(occupied, func(i, j int) bool {
		return occupied[i].SeatIndex < occupied[j].SeatIndex
	})

	now := time.Now().UTC()
	players := make([]gamerepo.Player, 0, len(occupied))
	for i, seat := range occupied {
		name := seat.Username
		if name == "" {
			name = "Player"
		}
		players = append(players, gamerepo.Player{
			UserID:     seat.UserID,
			Username:   name,
			SeatIndex:  seat.SeatIndex,
			TurnOrder:  i,
			Cash:       gamerepo.StartingCash,
			BoardIndex: gamerepo.GoBoardIndex,
			PinColor:   pinPalette[i%len(pinPalette)],
		})
	}

	g := &gamerepo.Game{
		ID:          primitive.NewObjectID().Hex(),
		TableID:     tableID,
		WorldID:     worldID,
		Status:      gamerepo.StatusActive,
		Players:     players,
		CurrentTurn: 0,
		PassGoBonus: gamerepo.PassGoBonus,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Insert(ctx, g); err != nil {
		// Race: another starter won — return existing.
		if existing, findErr := s.repo.FindByTableID(ctx, tableID); findErr == nil {
			return toView(existing), nil
		}
		return nil, err
	}
	return toView(g), nil
}

func (s *service) Get(ctx context.Context, gameID string) (*View, error) {
	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toView(g), nil
}

func (s *service) GetByTableID(ctx context.Context, tableID string) (*View, error) {
	g, err := s.repo.FindByTableID(ctx, tableID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toView(g), nil
}

func toView(g *gamerepo.Game) *View {
	players := make([]PlayerView, len(g.Players))
	currentUserID := ""
	currentUsername := ""
	for i, p := range g.Players {
		players[i] = PlayerView{
			UserID:     p.UserID,
			Username:   p.Username,
			SeatIndex:  p.SeatIndex,
			TurnOrder:  p.TurnOrder,
			Cash:       p.Cash,
			BoardIndex: p.BoardIndex,
			PinColor:   p.PinColor,
		}
		if p.TurnOrder == g.CurrentTurn {
			currentUserID = p.UserID
			currentUsername = p.Username
		}
	}
	return &View{
		ID:              g.ID,
		TableID:         g.TableID,
		WorldID:         g.WorldID,
		Status:          g.Status,
		Players:         players,
		CurrentTurn:     g.CurrentTurn,
		CurrentUserID:   currentUserID,
		CurrentUsername: currentUsername,
		PassGoBonus:     g.PassGoBonus,
		Currency:        "MeetCoin",
	}
}
