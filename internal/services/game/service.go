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
	ErrMustEndTurn   = errors.New("must end turn before rolling again")
	ErrMustRoll      = errors.New("must roll before ending turn")
	ErrAlreadyOut    = errors.New("already resigned")
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
	Resigned   bool   `json:"resigned"`
}

// LastRollView is the public last-dice snapshot.
type LastRollView struct {
	UserID        string `json:"userId"`
	Username      string `json:"username"`
	Die1          int    `json:"die1"`
	Die2          int    `json:"die2"`
	Total         int    `json:"total"`
	FromIndex     int    `json:"fromIndex"`
	ToIndex       int    `json:"toIndex"`
	PassedGo      bool   `json:"passedGo"`
	PassGoAmount  int    `json:"passGoAmount"`
	IsDoubles     bool   `json:"isDoubles"`
	DoublesStreak int    `json:"doublesStreak"`
	ThirdDoubles  bool   `json:"thirdDoubles"`
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
	TurnPhase       string        `json:"turnPhase"`
	DoublesStreak   int           `json:"doublesStreak"`
	CanRoll         bool          `json:"canRoll"`
	CanEndTurn      bool          `json:"canEndTurn"`
	LastRoll        *LastRollView `json:"lastRoll"`
	WinnerUserID    string        `json:"winnerUserId,omitempty"`
	WinnerUsername  string        `json:"winnerUsername,omitempty"`
}

// Event is pushed to WebSocket subscribers (Phase 6 realtime).
type Event struct {
	Type  string `json:"type"` // state | error | pong
	Game  *View  `json:"game,omitempty"`
	Error string `json:"error,omitempty"`
}

// Broadcaster fans game events to WS clients.
type Broadcaster interface {
	Broadcast(gameID string, ev Event)
}

// Service is the game application port.
type Service interface {
	CreateFromSeats(ctx context.Context, tableID, worldID string, seats []SeatInput) (*View, error)
	Get(ctx context.Context, gameID string) (*View, error)
	GetByTableID(ctx context.Context, tableID string) (*View, error)
	Roll(ctx context.Context, gameID, userID string) (*View, error)
	EndTurn(ctx context.Context, gameID, userID string) (*View, error)
	Resign(ctx context.Context, gameID, userID string) (*View, error)
	SetBroadcaster(b Broadcaster)
}

var pinPalette = []string{
	"#ED1B24",
	"#0072BB",
	"#1FB25A",
	"#F7941D",
	"#D93A96",
	"#FEF200",
}

type service struct {
	repo  gamerepo.Repository
	mu    sync.Mutex
	bcast Broadcaster
}

// New builds a game Service.
func New(repo gamerepo.Repository) Service {
	return &service{repo: repo}
}

func (s *service) SetBroadcaster(b Broadcaster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bcast = b
}

func (s *service) broadcast(gameID string, ev Event) {
	if s.bcast != nil {
		s.bcast.Broadcast(gameID, ev)
	}
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
		ID:            primitive.NewObjectID().Hex(),
		TableID:       tableID,
		WorldID:       worldID,
		Status:        gamerepo.StatusActive,
		Players:       players,
		CurrentTurn:   0,
		PassGoBonus:   gamerepo.PassGoBonus,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		DoublesStreak: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
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
	normalizeTurnPhase(g)
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
	normalizeTurnPhase(g)
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
	normalizeTurnPhase(g)

	playerIdx, err := requireCurrentPlayer(g, userID)
	if err != nil {
		return nil, err
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
		return nil, ErrMustEndTurn
	}

	die1 := rollDie()
	die2 := rollDie()
	total := die1 + die2
	isDoubles := die1 == die2
	from := g.Players[playerIdx].BoardIndex

	thirdDoubles := false
	to := from
	passedGo := false
	passAmt := 0

	if isDoubles {
		g.DoublesStreak++
	} else {
		g.DoublesStreak = 0
	}

	if isDoubles && g.DoublesStreak >= 3 {
		// Official: go to Jail without completing the third move (Jail = Phase 12).
		// Phase 6.2: skip movement and require End turn.
		thirdDoubles = true
		to = from
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	} else {
		to = (from + total) % gamerepo.BoardSpaceCount
		passedGo = from+total >= gamerepo.BoardSpaceCount
		if passedGo {
			passAmt = g.PassGoBonus
			if passAmt <= 0 {
				passAmt = gamerepo.PassGoBonus
			}
			g.Players[playerIdx].Cash += passAmt
		}
		g.Players[playerIdx].BoardIndex = to
		if isDoubles {
			g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		} else {
			g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
		}
	}

	g.LastRoll = &gamerepo.LastRoll{
		UserID:        userID,
		Username:      g.Players[playerIdx].Username,
		Die1:          die1,
		Die2:          die2,
		Total:         total,
		FromIndex:     from,
		ToIndex:       to,
		PassedGo:      passedGo,
		PassGoAmount:  passAmt,
		IsDoubles:     isDoubles,
		DoublesStreak: g.DoublesStreak,
		ThirdDoubles:  thirdDoubles,
	}
	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := toView(g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func (s *service) EndTurn(ctx context.Context, gameID, userID string) (*View, error) {
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
	normalizeTurnPhase(g)

	if _, err := requireCurrentPlayer(g, userID); err != nil {
		return nil, err
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
		return nil, ErrMustRoll
	}

	n := len(g.Players)
	if n > 0 {
		advanceToNextActive(g)
	}
	g.DoublesStreak = 0
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := toView(g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// Resign marks the caller as out (Phase 6.2c). Leaving the board mid-game is resigning.
// If only one active player remains, they win and the game finishes.
func (s *service) Resign(ctx context.Context, gameID, userID string) (*View, error) {
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
	normalizeTurnPhase(g)

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
	if g.Players[playerIdx].Resigned {
		return nil, ErrAlreadyOut
	}

	g.Players[playerIdx].Resigned = true
	wasCurrent := g.Players[playerIdx].TurnOrder == g.CurrentTurn

	if finishIfOneActive(g) {
		// winner set
	} else if wasCurrent {
		advanceToNextActive(g)
		g.DoublesStreak = 0
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	}

	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := toView(g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func requireCurrentPlayer(g *gamerepo.Game, userID string) (int, error) {
	playerIdx := -1
	for i := range g.Players {
		if g.Players[i].UserID == userID {
			playerIdx = i
			break
		}
	}
	if playerIdx < 0 {
		return -1, ErrNotPlayer
	}
	if g.Players[playerIdx].Resigned {
		return -1, ErrAlreadyOut
	}
	if g.Players[playerIdx].TurnOrder != g.CurrentTurn {
		return -1, ErrNotYourTurn
	}
	return playerIdx, nil
}

func playerByTurnOrder(g *gamerepo.Game, turnOrder int) *gamerepo.Player {
	for i := range g.Players {
		if g.Players[i].TurnOrder == turnOrder {
			return &g.Players[i]
		}
	}
	return nil
}

func advanceToNextActive(g *gamerepo.Game) {
	n := len(g.Players)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		g.CurrentTurn = (g.CurrentTurn + 1) % n
		p := playerByTurnOrder(g, g.CurrentTurn)
		if p != nil && !p.Resigned {
			return
		}
	}
}

// finishIfOneActive sets finished + winner when exactly one active player remains.
func finishIfOneActive(g *gamerepo.Game) bool {
	var winner *gamerepo.Player
	active := 0
	for i := range g.Players {
		if g.Players[i].Resigned {
			continue
		}
		active++
		winner = &g.Players[i]
	}
	if active != 1 || winner == nil {
		return false
	}
	g.Status = gamerepo.StatusFinished
	g.WinnerUserID = winner.UserID
	g.WinnerUsername = winner.Username
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	g.DoublesStreak = 0
	return true
}

func normalizeTurnPhase(g *gamerepo.Game) {
	if g.TurnPhase == gamerepo.TurnPhaseAwaitingRoll || g.TurnPhase == gamerepo.TurnPhaseAwaitingEnd {
		return
	}
	// Legacy 6.1 docs (auto-advance): treat as start-of-turn roll.
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
}

func rollDie() int {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1
	}
	return int(b[0]%6) + 1
}

func toView(g *gamerepo.Game) *View {
	normalizeTurnPhase(g)
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
			Resigned:   p.Resigned,
		}
		if !p.Resigned && p.TurnOrder == g.CurrentTurn {
			currentUserID = p.UserID
			currentUsername = p.Username
		}
	}
	var last *LastRollView
	if g.LastRoll != nil {
		last = &LastRollView{
			UserID:        g.LastRoll.UserID,
			Username:      g.LastRoll.Username,
			Die1:          g.LastRoll.Die1,
			Die2:          g.LastRoll.Die2,
			Total:         g.LastRoll.Total,
			FromIndex:     g.LastRoll.FromIndex,
			ToIndex:       g.LastRoll.ToIndex,
			PassedGo:      g.LastRoll.PassedGo,
			PassGoAmount:  g.LastRoll.PassGoAmount,
			IsDoubles:     g.LastRoll.IsDoubles,
			DoublesStreak: g.LastRoll.DoublesStreak,
			ThirdDoubles:  g.LastRoll.ThirdDoubles,
		}
	}
	phase := g.TurnPhase
	active := g.Status == gamerepo.StatusActive
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
		TurnPhase:       phase,
		DoublesStreak:   g.DoublesStreak,
		CanRoll:         active && phase == gamerepo.TurnPhaseAwaitingRoll,
		CanEndTurn:      active && phase == gamerepo.TurnPhaseAwaitingEnd,
		LastRoll:        last,
		WinnerUserID:    g.WinnerUserID,
		WinnerUsername:  g.WinnerUsername,
	}
}
