package game

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"sort"
	"strings"
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
	ErrNotBuyable    = errors.New("space is not buyable")
	ErrCannotAfford  = errors.New("insufficient MeetCoin")
	ErrAlreadyOwned  = errors.New("space already owned")
	ErrMustSettle       = errors.New("must settle rent or tax before continuing")
	ErrInvalidPinColor  = errors.New("invalid pin color")
)

// SeatInput is a seated lobby player used to bootstrap a game.
type SeatInput struct {
	UserID    string
	Username  string
	SeatIndex int
	PinColor  string
}

// PlayerView is the public player shape.
type PlayerView struct {
	UserID          string `json:"userId"`
	Username        string `json:"username"`
	SeatIndex       int    `json:"seatIndex"`
	TurnOrder       int    `json:"turnOrder"`
	Cash            int    `json:"cash"`
	BoardIndex      int    `json:"boardIndex"`
	PinColor        string `json:"pinColor"`
	Resigned        bool   `json:"resigned"`
	TimeRemainingMs int64  `json:"timeRemainingMs"`
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

// DeedView is public ownership of a board space.
type DeedView struct {
	BoardIndex     int    `json:"boardIndex"`
	OwnerUserID    string `json:"ownerUserId"`
	OwnerUsername  string `json:"ownerUsername"`
}

// BuyOfferView is shown when the current player may buy the space they occupy.
type BuyOfferView struct {
	BoardIndex int    `json:"boardIndex"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Price      int    `json:"price"`
}

// LastPaymentView is the most recent rent/tax transfer (Phase 6.5).
type LastPaymentView struct {
	Kind         string `json:"kind"`
	FromUserID   string `json:"fromUserId"`
	FromUsername string `json:"fromUsername"`
	ToUserID     string `json:"toUserId,omitempty"`
	ToUsername   string `json:"toUsername,omitempty"`
	Amount       int    `json:"amount"`
	BoardIndex   int    `json:"boardIndex"`
	SpaceName    string `json:"spaceName"`
	PaidInFull   bool   `json:"paidInFull"`
}

// PendingPaymentView — unpaid remainder blocking Roll/End.
type PendingPaymentView struct {
	Kind       string `json:"kind"`
	Amount     int    `json:"amount"`
	ToUserID   string `json:"toUserId,omitempty"`
	ToUsername string `json:"toUsername,omitempty"`
	BoardIndex int    `json:"boardIndex"`
	SpaceName  string `json:"spaceName"`
}

// View is the public game snapshot.
type View struct {
	ID              string              `json:"id"`
	TableID         string              `json:"tableId"`
	WorldID         string              `json:"worldId"`
	Status          string              `json:"status"`
	Players         []PlayerView        `json:"players"`
	CurrentTurn     int                 `json:"currentTurn"`
	CurrentUserID   string              `json:"currentUserId"`
	CurrentUsername string              `json:"currentUsername"`
	PassGoBonus     int                 `json:"passGoBonus"`
	Currency        string              `json:"currency"`
	TurnPhase       string              `json:"turnPhase"`
	DoublesStreak   int                 `json:"doublesStreak"`
	CanRoll         bool                `json:"canRoll"`
	CanEndTurn      bool                `json:"canEndTurn"`
	CanBuy          bool                `json:"canBuy"`
	BuyOffer        *BuyOfferView       `json:"buyOffer"`
	Deeds           []DeedView          `json:"deeds"`
	LastRoll        *LastRollView       `json:"lastRoll"`
	LastPayment     *LastPaymentView    `json:"lastPayment,omitempty"`
	PendingPayment  *PendingPaymentView `json:"pendingPayment,omitempty"`
	WinnerUserID    string              `json:"winnerUserId,omitempty"`
	WinnerUsername  string              `json:"winnerUsername,omitempty"`
	// TurnStartedAt — RFC3339 UTC; current player's bank drains from this instant.
	TurnStartedAt string `json:"turnStartedAt,omitempty"`
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
	Buy(ctx context.Context, gameID, userID string) (*View, error)
	SetPinColor(ctx context.Context, gameID, userID, pinColor string) (*View, error)
	// Disconnect starts a silent hold; expire → Resign (Phase 7.5). Presence must not call this.
	Disconnect(ctx context.Context, gameID, userID string) error
	// CancelDisconnectHold clears a pending auto-resign when the game WS reconnects.
	CancelDisconnectHold(gameID, userID string)
	SetBroadcaster(b Broadcaster)
}

// Config tunes game service timers (Phase 7.5 disconnect hold).
type Config struct {
	// DisconnectHold is how long after game WS drop before auto-resign. Default 3m.
	DisconnectHold time.Duration
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
	repo       gamerepo.Repository
	spaces     SpaceCatalog
	cfg        Config
	mu         sync.Mutex
	bcast      Broadcaster
	bankTimers map[string]*time.Timer
	holds      map[string]*time.Timer // gameID\0userID → disconnect hold
}

// New builds a game Service. spaces may be nil in unit tests that never buy.
func New(repo gamerepo.Repository, spaces SpaceCatalog, cfg Config) Service {
	if cfg.DisconnectHold <= 0 {
		cfg.DisconnectHold = 3 * time.Minute
	}
	return &service{
		repo:       repo,
		spaces:     spaces,
		cfg:        cfg,
		bankTimers: make(map[string]*time.Timer),
		holds:      make(map[string]*time.Timer),
	}
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
		return s.viewOf(ctx, existing), nil
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
	bankMs := gamerepo.TimeBankDuration.Milliseconds()
	players := make([]gamerepo.Player, 0, len(occupied))
	for i, seat := range occupied {
		name := seat.Username
		if name == "" {
			name = "Player"
		}
		color := strings.TrimSpace(seat.PinColor)
		if color == "" {
			color = pinPalette[i%len(pinPalette)]
		}
		players = append(players, gamerepo.Player{
			UserID:          seat.UserID,
			Username:        capitalizePlayerName(name),
			SeatIndex:       seat.SeatIndex,
			TurnOrder:       i,
			Cash:            gamerepo.StartingCash,
			BoardIndex:      gamerepo.GoBoardIndex,
			PinColor:        color,
			TimeRemainingMs: bankMs,
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
		TurnStartedAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.armBankTimerLocked(g)
	if err := s.repo.Insert(ctx, g); err != nil {
		s.cancelBankTimerLocked(g.ID)
		if existing, findErr := s.repo.FindByTableID(ctx, tableID); findErr == nil {
			return s.viewOf(ctx, existing), nil
		}
		return nil, err
	}
	return s.viewOf(ctx, g), nil
}

func (s *service) Get(ctx context.Context, gameID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if changed, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	} else if changed {
		return s.viewOf(ctx, g), nil
	}
	s.ensureBanksLocked(g)
	s.armBankTimerLocked(g)
	return s.viewOf(ctx, g), nil
}

func (s *service) GetByTableID(ctx context.Context, tableID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByTableID(ctx, tableID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if changed, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	} else if changed {
		return s.viewOf(ctx, g), nil
	}
	s.ensureBanksLocked(g)
	s.armBankTimerLocked(g)
	return s.viewOf(ctx, g), nil
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
	if _, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	}
	if g.Status != gamerepo.StatusActive {
		return s.viewOf(ctx, g), nil
	}

	playerIdx, err := requireCurrentPlayer(g, userID)
	if err != nil {
		return nil, err
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
		return nil, ErrMustEndTurn
	}
	if hasPendingPayment(g) {
		return nil, ErrMustSettle
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

	if !thirdDoubles {
		s.resolveLandingLocked(ctx, g, playerIdx, total)
	}

	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
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
	if _, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	}
	if g.Status != gamerepo.StatusActive {
		return s.viewOf(ctx, g), nil
	}

	if _, err := requireCurrentPlayer(g, userID); err != nil {
		return nil, err
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
		return nil, ErrMustRoll
	}
	if hasPendingPayment(g) {
		return nil, ErrMustSettle
	}

	s.pauseCurrentBankLocked(g)
	curIdx := currentPlayerIndex(g)
	if curIdx >= 0 && g.Players[curIdx].TimeRemainingMs <= 0 {
		g.Players[curIdx].Resigned = true
		g.Players[curIdx].TimeRemainingMs = 0
		if finishIfOneActive(g) {
			s.clearBankClockLocked(g)
		} else {
			advanceToNextActive(g)
			g.DoublesStreak = 0
			g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
			s.startCurrentBankLocked(g)
		}
	} else {
		advanceToNextActive(g)
		g.DoublesStreak = 0
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		s.startCurrentBankLocked(g)
	}
	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// Resign marks the caller as out (Phase 6.2c). Leaving the board mid-game is resigning.
func (s *service) Resign(ctx context.Context, gameID, userID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resignLocked(ctx, gameID, userID)
}

// Disconnect starts a silent auto-resign hold (Phase 7.5). No broadcast while held.
func (s *service) Disconnect(ctx context.Context, gameID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil
		}
		return err
	}
	if g.Status != gamerepo.StatusActive {
		return nil
	}
	var found, alreadyOut bool
	for i := range g.Players {
		if g.Players[i].UserID == userID {
			found = true
			alreadyOut = g.Players[i].Resigned
			break
		}
	}
	if !found || alreadyOut {
		return nil
	}

	s.clearDisconnectHoldLocked(gameID, userID)
	key := disconnectHoldKey(gameID, userID)
	hold := s.cfg.DisconnectHold
	s.holds[key] = time.AfterFunc(hold, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.holds, key)
		if _, err := s.resignLocked(context.Background(), gameID, userID); err != nil {
			if !errors.Is(err, ErrAlreadyOut) && !errors.Is(err, ErrInactive) && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrNotPlayer) {
				slog.Warn("game disconnect hold resign",
					"gameId", gameID,
					"userId", userID,
					"err", err,
				)
			}
		}
	})
	slog.Info("game disconnect hold started",
		"gameId", gameID,
		"userId", userID,
		"hold", hold.String(),
	)
	return nil
}

// CancelDisconnectHold clears a pending auto-resign (game WS reconnected).
func (s *service) CancelDisconnectHold(gameID, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearDisconnectHoldLocked(gameID, userID)
}

func disconnectHoldKey(gameID, userID string) string {
	return gameID + "\x00" + userID
}

func (s *service) clearDisconnectHoldLocked(gameID, userID string) {
	key := disconnectHoldKey(gameID, userID)
	if t, ok := s.holds[key]; ok {
		t.Stop()
		delete(s.holds, key)
	}
}

func (s *service) resignLocked(ctx context.Context, gameID, userID string) (*View, error) {
	s.clearDisconnectHoldLocked(gameID, userID)

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
	if _, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	}
	if g.Status != gamerepo.StatusActive {
		return s.viewOf(ctx, g), nil
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
	if g.Players[playerIdx].Resigned {
		return nil, ErrAlreadyOut
	}

	wasCurrent := g.Players[playerIdx].TurnOrder == g.CurrentTurn
	if wasCurrent {
		s.pauseCurrentBankLocked(g)
	}
	g.Players[playerIdx].Resigned = true
	if wasCurrent {
		g.PendingPayment = nil
	}

	if finishIfOneActive(g) {
		s.clearBankClockLocked(g)
	} else if wasCurrent {
		advanceToNextActive(g)
		g.DoublesStreak = 0
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		s.startCurrentBankLocked(g)
	}

	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// Buy purchases the unowned buyable space the current player occupies (Phase 6.4).
func (s *service) Buy(ctx context.Context, gameID, userID string) (*View, error) {
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
	if _, err := s.syncTimeBankLocked(ctx, g); err != nil {
		return nil, err
	}
	if g.Status != gamerepo.StatusActive {
		return s.viewOf(ctx, g), nil
	}

	playerIdx, err := requireCurrentPlayer(g, userID)
	if err != nil {
		return nil, err
	}
	if hasPendingPayment(g) {
		return nil, ErrMustSettle
	}

	spaces := s.loadSpaces(ctx, g.WorldID)
	sp := spaceAt(spaces, g.Players[playerIdx].BoardIndex)
	if sp == nil || !isBuyableKind(sp.Kind) || sp.Price <= 0 {
		return nil, ErrNotBuyable
	}
	if ownerOf(g.Deeds, sp.BoardIndex) != "" {
		return nil, ErrAlreadyOwned
	}
	if g.Players[playerIdx].Cash < sp.Price {
		return nil, ErrCannotAfford
	}

	g.Players[playerIdx].Cash -= sp.Price
	g.Deeds = append(g.Deeds, gamerepo.Deed{
		BoardIndex:  sp.BoardIndex,
		OwnerUserID: userID,
	})
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// SetPinColor syncs a player's board pin / ownership chip to their avatar accent.
func (s *service) SetPinColor(ctx context.Context, gameID, userID, pinColor string) (*View, error) {
	normalized, ok := normalizePinColor(pinColor)
	if !ok {
		return nil, ErrInvalidPinColor
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		if errors.Is(err, gamerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	playerIdx := -1
	for i, p := range g.Players {
		if p.UserID == userID {
			playerIdx = i
			break
		}
	}
	if playerIdx < 0 {
		return nil, ErrNotPlayer
	}
	if strings.EqualFold(g.Players[playerIdx].PinColor, normalized) {
		return s.viewOf(ctx, g), nil
	}
	for i, p := range g.Players {
		if i == playerIdx || p.UserID == "" {
			continue
		}
		if strings.EqualFold(p.PinColor, normalized) {
			return nil, ErrInvalidPinColor
		}
	}

	g.Players[playerIdx].PinColor = normalized
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func normalizePinColor(raw string) (string, bool) {
	c := strings.TrimSpace(raw)
	if len(c) != 7 || c[0] != '#' {
		return "", false
	}
	for i := 1; i < 7; i++ {
		ch := c[i]
		isHex := (ch >= '0' && ch <= '9') ||
			(ch >= 'a' && ch <= 'f') ||
			(ch >= 'A' && ch <= 'F')
		if !isHex {
			return "", false
		}
	}
	return "#" + strings.ToLower(c[1:]), true
}

// capitalizePlayerName Title-Cases seat names into game docs (ademola → Ademola).
func capitalizePlayerName(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	capNext := true
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '_' {
			capNext = true
			continue
		}
		if capNext {
			if c >= 'a' && c <= 'z' {
				b[i] = c - 'a' + 'A'
			}
			capNext = false
			continue
		}
		if c >= 'A' && c <= 'Z' {
			b[i] = c - 'A' + 'a'
		}
	}
	return string(b)
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

func currentPlayerIndex(g *gamerepo.Game) int {
	for i := range g.Players {
		if g.Players[i].TurnOrder == g.CurrentTurn {
			return i
		}
	}
	return -1
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
	g.TurnStartedAt = time.Time{}
	return true
}

func normalizeTurnPhase(g *gamerepo.Game) {
	if g.TurnPhase == gamerepo.TurnPhaseAwaitingRoll || g.TurnPhase == gamerepo.TurnPhaseAwaitingEnd {
		return
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
}

func (s *service) ensureBanksLocked(g *gamerepo.Game) {
	bankMs := gamerepo.TimeBankDuration.Milliseconds()
	anyPositive := false
	for i := range g.Players {
		if g.Players[i].TimeRemainingMs > 0 {
			anyPositive = true
			break
		}
	}
	if !anyPositive {
		for i := range g.Players {
			if g.Players[i].Resigned {
				continue
			}
			g.Players[i].TimeRemainingMs = bankMs
		}
	}
	if g.Status == gamerepo.StatusActive && g.TurnStartedAt.IsZero() {
		g.TurnStartedAt = time.Now().UTC()
	}
}

// pauseCurrentBankLocked deducts elapsed time from the current player and clears turn start.
func (s *service) pauseCurrentBankLocked(g *gamerepo.Game) {
	idx := currentPlayerIndex(g)
	if idx < 0 || g.Players[idx].Resigned || g.TurnStartedAt.IsZero() {
		g.TurnStartedAt = time.Time{}
		return
	}
	elapsed := time.Since(g.TurnStartedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	left := g.Players[idx].TimeRemainingMs - elapsed.Milliseconds()
	if left < 0 {
		left = 0
	}
	g.Players[idx].TimeRemainingMs = left
	g.TurnStartedAt = time.Time{}
}

func (s *service) startCurrentBankLocked(g *gamerepo.Game) {
	if g.Status != gamerepo.StatusActive {
		s.clearBankClockLocked(g)
		return
	}
	g.TurnStartedAt = time.Now().UTC()
	s.armBankTimerLocked(g)
}

func (s *service) clearBankClockLocked(g *gamerepo.Game) {
	g.TurnStartedAt = time.Time{}
	s.cancelBankTimerLocked(g.ID)
}

// syncTimeBankLocked applies elapsed drain; eliminates current player if bank hit 0.
// Returns true when the document was mutated and persisted.
func (s *service) syncTimeBankLocked(ctx context.Context, g *gamerepo.Game) (bool, error) {
	normalizeTurnPhase(g)
	s.ensureBanksLocked(g)
	if g.Status != gamerepo.StatusActive {
		return false, nil
	}

	changed := false
	for {
		idx := currentPlayerIndex(g)
		if idx < 0 || g.Players[idx].Resigned || g.TurnStartedAt.IsZero() {
			break
		}
		elapsed := time.Since(g.TurnStartedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed.Milliseconds() < g.Players[idx].TimeRemainingMs {
			s.armBankTimerLocked(g)
			break
		}

		g.Players[idx].TimeRemainingMs = 0
		g.Players[idx].Resigned = true
		g.TurnStartedAt = time.Time{}
		changed = true

		if finishIfOneActive(g) {
			s.clearBankClockLocked(g)
			break
		}
		advanceToNextActive(g)
		g.DoublesStreak = 0
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		s.startCurrentBankLocked(g)
	}

	if !changed {
		return false, nil
	}
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return false, err
	}
	s.broadcast(g.ID, Event{Type: "state", Game: s.viewOf(ctx, g)})
	return true, nil
}

func (s *service) armBankTimerLocked(g *gamerepo.Game) {
	s.cancelBankTimerLocked(g.ID)
	if g.Status != gamerepo.StatusActive || g.TurnStartedAt.IsZero() {
		return
	}
	idx := currentPlayerIndex(g)
	if idx < 0 || g.Players[idx].Resigned {
		return
	}
	left := g.Players[idx].TimeRemainingMs - time.Since(g.TurnStartedAt).Milliseconds()
	if left < 0 {
		left = 0
	}
	delay := time.Duration(left) * time.Millisecond
	gameID := g.ID
	s.bankTimers[gameID] = time.AfterFunc(delay, func() {
		_ = s.onBankExpired(context.Background(), gameID)
	})
}

func (s *service) cancelBankTimerLocked(gameID string) {
	if t, ok := s.bankTimers[gameID]; ok {
		t.Stop()
		delete(s.bankTimers, gameID)
	}
}

func (s *service) onBankExpired(ctx context.Context, gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		return err
	}
	_, err = s.syncTimeBankLocked(ctx, g)
	return err
}

func rollDie() int {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1
	}
	return int(b[0]%6) + 1
}

func liveRemainingMs(g *gamerepo.Game, p gamerepo.Player, now time.Time) int64 {
	if p.Resigned {
		return 0
	}
	left := p.TimeRemainingMs
	if g.Status == gamerepo.StatusActive &&
		!g.TurnStartedAt.IsZero() &&
		p.TurnOrder == g.CurrentTurn {
		elapsed := now.Sub(g.TurnStartedAt).Milliseconds()
		left -= elapsed
	}
	if left < 0 {
		return 0
	}
	return left
}

func (s *service) loadSpaces(ctx context.Context, worldID string) []Space {
	if s.spaces == nil {
		return nil
	}
	spaces, err := s.spaces.ListSpaces(ctx, worldID)
	if err != nil {
		return nil
	}
	return spaces
}

func (s *service) viewOf(ctx context.Context, g *gamerepo.Game) *View {
	return toView(g, s.loadSpaces(ctx, g.WorldID))
}

func toView(g *gamerepo.Game, spaces []Space) *View {
	normalizeTurnPhase(g)
	now := time.Now().UTC()
	players := make([]PlayerView, len(g.Players))
	currentUserID := ""
	currentUsername := ""
	nameByID := make(map[string]string, len(g.Players))
	for i, p := range g.Players {
		nameByID[p.UserID] = p.Username
		players[i] = PlayerView{
			UserID:          p.UserID,
			Username:        p.Username,
			SeatIndex:       p.SeatIndex,
			TurnOrder:       p.TurnOrder,
			Cash:            p.Cash,
			BoardIndex:      p.BoardIndex,
			PinColor:        p.PinColor,
			Resigned:        p.Resigned,
			TimeRemainingMs: liveRemainingMs(g, p, now),
		}
		if !p.Resigned && p.TurnOrder == g.CurrentTurn {
			currentUserID = p.UserID
			currentUsername = p.Username
		}
	}
	deeds := make([]DeedView, 0, len(g.Deeds))
	for _, d := range g.Deeds {
		deeds = append(deeds, DeedView{
			BoardIndex:    d.BoardIndex,
			OwnerUserID:   d.OwnerUserID,
			OwnerUsername: nameByID[d.OwnerUserID],
		})
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
	started := ""
	if active && !g.TurnStartedAt.IsZero() {
		started = g.TurnStartedAt.UTC().Format(time.RFC3339Nano)
	}

	var buyOffer *BuyOfferView
	canBuy := false
	pending := hasPendingPayment(g)
	if active && currentUserID != "" && !pending {
		idx := currentPlayerIndex(g)
		if idx >= 0 && !g.Players[idx].Resigned {
			sp := spaceAt(spaces, g.Players[idx].BoardIndex)
			if sp != nil && isBuyableKind(sp.Kind) && sp.Price > 0 && ownerOf(g.Deeds, sp.BoardIndex) == "" {
				landedThisTurn := g.LastRoll != nil &&
					g.LastRoll.UserID == currentUserID &&
					g.LastRoll.ToIndex == sp.BoardIndex &&
					!g.LastRoll.ThirdDoubles
				if landedThisTurn {
					buyOffer = &BuyOfferView{
						BoardIndex: sp.BoardIndex,
						Slug:       sp.Slug,
						Name:       sp.Name,
						Kind:       sp.Kind,
						Price:      sp.Price,
					}
					canBuy = true
				}
			}
		}
	}

	var lastPay *LastPaymentView
	if g.LastPayment != nil {
		lastPay = &LastPaymentView{
			Kind:         g.LastPayment.Kind,
			FromUserID:   g.LastPayment.FromUserID,
			FromUsername: g.LastPayment.FromUsername,
			ToUserID:     g.LastPayment.ToUserID,
			ToUsername:   g.LastPayment.ToUsername,
			Amount:       g.LastPayment.Amount,
			BoardIndex:   g.LastPayment.BoardIndex,
			SpaceName:    g.LastPayment.SpaceName,
			PaidInFull:   g.LastPayment.PaidInFull,
		}
	}
	var pend *PendingPaymentView
	if g.PendingPayment != nil && g.PendingPayment.Amount > 0 {
		pend = &PendingPaymentView{
			Kind:       g.PendingPayment.Kind,
			Amount:     g.PendingPayment.Amount,
			ToUserID:   g.PendingPayment.ToUserID,
			ToUsername: nameByID[g.PendingPayment.ToUserID],
			BoardIndex: g.PendingPayment.BoardIndex,
			SpaceName:  g.PendingPayment.SpaceName,
		}
		if g.PendingPayment.ToUserID == "" {
			pend.ToUsername = "Bank"
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
		TurnPhase:       phase,
		DoublesStreak:   g.DoublesStreak,
		CanRoll:         active && phase == gamerepo.TurnPhaseAwaitingRoll && !pending,
		CanEndTurn:      active && phase == gamerepo.TurnPhaseAwaitingEnd && !pending,
		CanBuy:          canBuy,
		BuyOffer:        buyOffer,
		Deeds:           deeds,
		LastRoll:        last,
		LastPayment:     lastPay,
		PendingPayment:  pend,
		WinnerUserID:    g.WinnerUserID,
		WinnerUsername:  g.WinnerUsername,
		TurnStartedAt:   started,
	}
}

func hasPendingPayment(g *gamerepo.Game) bool {
	return g.PendingPayment != nil && g.PendingPayment.Amount > 0
}

// resolveLandingLocked auto-collects rent/tax after a move (Phase 6.5).
func (s *service) resolveLandingLocked(ctx context.Context, g *gamerepo.Game, payerIdx, diceTotal int) {
	g.PendingPayment = nil
	spaces := s.loadSpaces(ctx, g.WorldID)
	boardIndex := g.Players[payerIdx].BoardIndex
	payerID := g.Players[payerIdx].UserID
	amount, toUserID, kind, spaceName := rentDueForLanding(
		spaces, g.Deeds, boardIndex, payerID, diceTotal,
	)
	if amount <= 0 || kind == "" {
		return
	}

	pay := amount
	paidInFull := true
	if g.Players[payerIdx].Cash < amount {
		pay = g.Players[payerIdx].Cash
		paidInFull = false
	}
	g.Players[payerIdx].Cash -= pay
	toUsername := "Bank"
	if toUserID != "" {
		for i := range g.Players {
			if g.Players[i].UserID == toUserID {
				g.Players[i].Cash += pay
				toUsername = g.Players[i].Username
				break
			}
		}
	}

	g.LastPayment = &gamerepo.LastPayment{
		Kind:         kind,
		FromUserID:   payerID,
		FromUsername: g.Players[payerIdx].Username,
		ToUserID:     toUserID,
		ToUsername:   toUsername,
		Amount:       pay,
		BoardIndex:   boardIndex,
		SpaceName:    spaceName,
		PaidInFull:   paidInFull,
	}

	if !paidInFull {
		remaining := amount - pay
		g.PendingPayment = &gamerepo.PendingPayment{
			Kind:       kind,
			Amount:     remaining,
			ToUserID:   toUserID,
			BoardIndex: boardIndex,
			SpaceName:  spaceName,
		}
		// Can't continue turn (incl. doubles re-roll) until settled / resign.
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	}
}
