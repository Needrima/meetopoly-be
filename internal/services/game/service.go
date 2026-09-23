package game

import (
	"context"
	"crypto/rand"
	"errors"
	"sort"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	ErrNotFound      = errors.New("game not found")
	ErrAlreadyExists = errors.New("game already exists for table")
	ErrNeedPlayers   = errors.New("need at least 2 players to start")
	ErrNotYourTurn   = errors.New("not your turn")
	ErrNotPlayer     = errors.New("not a player in this game")
	ErrInactive      = errors.New("game is not active")
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

// LastRollView is the public last-dice snapshot (Phase 6.1).
type LastRollView struct {
	UserID       string `json:"userId"`
	Username     string `json:"username"`
	Die1         int    `json:"die1"`
	Die2         int    `json:"die2"`
	Total        int    `json:"total"`
	FromIndex    int    `json:"fromIndex"`
	ToIndex      int    `json:"toIndex"`
	PassedGo     bool   `json:"passedGo"`
	PassGoAmount int    `json:"passGoAmount"`
}

// View is the public game snapshot.
type View struct {
	ID              string        `json:"id"`
	TableID         string        `json:"tableId"`
	WorldID         string        `json:"worldId"`
	Status          string        `json:"status"`
	Players         []PlayerView  `json:"players"`
	CurrentTurn     int           `json:"currentTurn"`
	CurrentUserID   string        `json:"currentUserId"`
	CurrentUsername string        `json:"currentUsername"`
	PassGoBonus     int           `json:"passGoBonus"`
	Currency        string        `json:"currency"`
	LastRoll        *LastRollView `json:"lastRoll"`
}

// Service is the game application port.
type Service interface {
	CreateFromSeats(ctx context.Context, tableID, worldID string, seats []SeatInput) (*View, error)
	Get(ctx context.Context, gameID string) (*View, error)
	GetByTableID(ctx context.Context, tableID string) (*View, error)
	Roll(ctx context.Context, gameID, userID string) (*View, error)
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
	mu   sync.Mutex
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

func (s *service) Roll(ctx context.Context, gameID, userID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if g.Status != gamerepo.StatusActive {
		return nil, ErrInactive
	}

	playerIdx := -1
	for i := range g.Players {
		if g.Players[i].UserID == userID {
			playerIdx = i
			break
		}
	}
	if playerIdx < 0 {
		return nil, ErrNotPlayer
	}
	if g.Players[playerIdx].TurnOrder != g.CurrentTurn {
		return nil, ErrNotYourTurn
	}

	die1 := rollDie()
	die2 := rollDie()
	total := die1 + die2
	from := g.Players[playerIdx].BoardIndex
	to := (from + total) % gamerepo.BoardSpaceCount
	passedGo := from+total >= gamerepo.BoardSpaceCount
	passAmt := 0
	if passedGo {
		passAmt = g.PassGoBonus
		if passAmt <= 0 {
			passAmt = gamerepo.PassGoBonus
		}
		g.Players[playerIdx].Cash += passAmt
	}
	g.Players[playerIdx].BoardIndex = to
	g.LastRoll = &gamerepo.LastRoll{
		UserID:       userID,
		Username:     g.Players[playerIdx].Username,
		Die1:         die1,
		Die2:         die2,
		Total:        total,
		FromIndex:    from,
		ToIndex:      to,
		PassedGo:     passedGo,
		PassGoAmount: passAmt,
	}

	// Phase 6.1: doubles do not grant an extra roll; auto-advance (End arrives in 6.4).
	n := len(g.Players)
	if n > 0 {
		g.CurrentTurn = (g.CurrentTurn + 1) % n
	}
	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	return toView(g), nil
}

func rollDie() int {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1
	}
	return int(b[0]%6) + 1
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
	var last *LastRollView
	if g.LastRoll != nil {
		last = &LastRollView{
			UserID:       g.LastRoll.UserID,
			Username:     g.LastRoll.Username,
			Die1:         g.LastRoll.Die1,
			Die2:         g.LastRoll.Die2,
			Total:        g.LastRoll.Total,
			FromIndex:    g.LastRoll.FromIndex,
			ToIndex:      g.LastRoll.ToIndex,
			PassedGo:     g.LastRoll.PassedGo,
			PassGoAmount: g.LastRoll.PassGoAmount,
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
		LastRoll:        last,
	}
}
