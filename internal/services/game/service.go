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
	ErrNotFound          = errors.New("game not found")
	ErrAlreadyExists     = errors.New("game already exists for table")
	ErrNeedPlayers       = errors.New("need at least 2 players to start")
	ErrNotYourTurn       = errors.New("not your turn")
	ErrNotPlayer         = errors.New("not a player in this game")
	ErrInactive          = errors.New("game is not active")
	ErrMustEndTurn       = errors.New("must end turn before rolling again")
	ErrMustRoll          = errors.New("must roll before ending turn")
	ErrAlreadyOut        = errors.New("already resigned")
	ErrNotBuyable        = errors.New("space is not buyable")
	ErrCannotAfford      = errors.New("insufficient MeetCoin")
	ErrAlreadyOwned      = errors.New("space already owned")
	ErrMustSettle        = errors.New("must settle rent or tax before continuing")
	ErrMustBuy           = errors.New("must buy unowned property before ending turn")
	ErrInvalidPinColor   = errors.New("invalid pin color")
	ErrInvalidHubID      = errors.New("invalid hub id")
	ErrNotBuildable      = errors.New("space is not buildable")
	ErrNotOwner          = errors.New("you do not own this deed")
	ErrNoMonopoly        = errors.New("need full color group to build")
	ErrUnevenBuild       = errors.New("must build evenly across the color group")
	ErrMaxBuilt          = errors.New("hotel already built")
	ErrMortgagedSet      = errors.New("cannot build while a deed in the set is mortgaged")
	ErrInvalidBoardIndex = errors.New("invalid board index")
	ErrUnevenSell        = errors.New("must sell evenly across the color group")
	ErrNothingToSell     = errors.New("no building to sell on this deed")
	ErrAlreadyMortgaged  = errors.New("deed is already mortgaged")
	ErrNotMortgaged      = errors.New("deed is not mortgaged")
	ErrMustSellBuildings = errors.New("sell all buildings on the color group before mortgaging")
	ErrCannotMortgage    = errors.New("space cannot be mortgaged")
	ErrInJail            = errors.New("player is in jail")
	ErrNotInJail         = errors.New("player is not in jail")
	ErrNoJailCard        = errors.New("no get out of jail free card")
	ErrMustLeaveJail     = errors.New("must pay jail fine or use get out of jail free card")
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
	// Country — ISO 3166-1 alpha-2 from the user profile (Phase 9.0a); not stored on the game doc.
	Country string `json:"country,omitempty"`
	// HubID set while inside a hub (Phase 8.2); omitted when on the board.
	HubID string `json:"hubId,omitempty"`
	// HubRevision — bumps on leave/resign; clients send it on enter-hub to ignore stale enters (8.4).
	HubRevision int64 `json:"hubRevision"`
	// InJail — sent to Jail (Phase 12.0); false when Just Visiting on the Jail tile.
	InJail bool `json:"inJail"`
	// JailTurns — failed exit attempts (Phase 12.1); 0 on entry.
	JailTurns int `json:"jailTurns"`
	// GetOutOfJailFree — GOOJF cards held (Phase 12.2+).
	GetOutOfJailFree int `json:"getOutOfJailFree"`
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
	BoardIndex    int    `json:"boardIndex"`
	OwnerUserID   string `json:"ownerUserId"`
	OwnerUsername string `json:"ownerUsername"`
	Houses        int    `json:"houses"`    // 0–5; 5 = hotel (Phase 11.0)
	Mortgaged     bool   `json:"mortgaged"` // Phase 11.3; no rent while true
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

// LastCardView is the public last Chance/Chest draw (Phase 12.2).
type LastCardView struct {
	Deck      string `json:"deck"`
	CardID    string `json:"cardId"`
	Title     string `json:"title"`
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	CashDelta int    `json:"cashDelta,omitempty"`
}

// View is the public game snapshot.
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
	TurnPhase       string       `json:"turnPhase"`
	DoublesStreak   int          `json:"doublesStreak"`
	CanRoll         bool         `json:"canRoll"`
	CanEndTurn      bool         `json:"canEndTurn"`
	CanBuy          bool         `json:"canBuy"`
	// CanPayJailFine — current player may pay 100 MeetCoin to leave Jail (Phase 12.1).
	CanPayJailFine bool `json:"canPayJailFine"`
	// CanUseJailCard — current player holds a GOOJF card and may use it (Phase 12.1).
	CanUseJailCard bool                `json:"canUseJailCard"`
	BuyOffer       *BuyOfferView       `json:"buyOffer"`
	Deeds          []DeedView          `json:"deeds"`
	LastRoll       *LastRollView       `json:"lastRoll"`
	LastPayment    *LastPaymentView    `json:"lastPayment,omitempty"`
	PendingPayment *PendingPaymentView `json:"pendingPayment,omitempty"`
	LastCard       *LastCardView       `json:"lastCard,omitempty"`
	WinnerUserID   string              `json:"winnerUserId,omitempty"`
	WinnerUsername string              `json:"winnerUsername,omitempty"`
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
	// Build buys one house/hotel step on an owned city (Phase 11.1).
	Build(ctx context.Context, gameID, userID string, boardIndex int) (*View, error)
	// SellBuilding sells one house/hotel step at half houseCost (Phase 11.2).
	SellBuilding(ctx context.Context, gameID, userID string, boardIndex int) (*View, error)
	// Mortgage mortgages an owned deed for half list price (Phase 11.3).
	Mortgage(ctx context.Context, gameID, userID string, boardIndex int) (*View, error)
	// Redeem unmortgages a deed for mortgage value + 10% (Phase 11.3).
	Redeem(ctx context.Context, gameID, userID string, boardIndex int) (*View, error)
	// PayJailFine pays JailFine MeetCoin to leave Jail (Phase 12.1); then Roll to move.
	PayJailFine(ctx context.Context, gameID, userID string) (*View, error)
	// UseJailCard spends one Get Out of Jail Free card (Phase 12.1); then Roll to move.
	UseJailCard(ctx context.Context, gameID, userID string) (*View, error)
	SetPinColor(ctx context.Context, gameID, userID, pinColor string) (*View, error)
	// EnterHub marks the seated player as inside hubId (Phase 8.2); fans out via game WS.
	// clientRevision: when non-nil, ignore the enter if it is older than the player's HubRevision (8.4).
	EnterHub(ctx context.Context, gameID, userID, hubID string, clientRevision *int64) (*View, error)
	// LeaveHub clears hubId so board peers see them back on the board.
	LeaveHub(ctx context.Context, gameID, userID string) (*View, error)
	// Disconnect starts a silent hold; expire → Resign (Phase 7.5). Presence must not call this.
	Disconnect(ctx context.Context, gameID, userID string) error
	// CancelDisconnectHold clears a pending auto-resign when the game WS reconnects.
	CancelDisconnectHold(gameID, userID string)
	SetBroadcaster(b Broadcaster)
	// SetCountryLookup enriches PlayerView.Country from user profiles (Phase 9.0a).
	SetCountryLookup(l CountryLookup)
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
	countries  CountryLookup
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

func (s *service) SetCountryLookup(l CountryLookup) {
	// Boot-only; read lock-free from viewOf (may already hold s.mu).
	s.countries = l
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
		ChanceDeck:    newShuffledChanceDeck(),
		ChestDeck:     newShuffledChestDeck(),
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
	if g.Players[playerIdx].InJail {
		return s.rollFromJailLocked(ctx, g, gameID, userID, playerIdx)
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
	streakForRoll := g.DoublesStreak

	if isDoubles && g.DoublesStreak >= 3 {
		thirdDoubles = true
		spaces := s.loadSpaces(ctx, g.WorldID)
		to = sendPlayerToJail(g, playerIdx, spaces)
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
		DoublesStreak: streakForRoll,
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

// rollFromJailLocked — doubles get out free and move; else jailTurns++; on 3rd fail pay fine + move when affordable.
func (s *service) rollFromJailLocked(
	ctx context.Context, g *gamerepo.Game, gameID, userID string, playerIdx int,
) (*View, error) {
	if g.Players[playerIdx].JailTurns >= gamerepo.MaxJailAttempts {
		return nil, ErrMustLeaveJail
	}

	die1 := rollDie()
	die2 := rollDie()
	total := die1 + die2
	isDoubles := die1 == die2
	from := g.Players[playerIdx].BoardIndex
	g.DoublesStreak = 0

	if isDoubles {
		leaveJail(&g.Players[playerIdx])
		to, passedGo, passAmt := applyBoardMove(g, playerIdx, from, total)
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
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
			IsDoubles:     true,
			DoublesStreak: 0,
			ThirdDoubles:  false,
		}
		s.resolveLandingLocked(ctx, g, playerIdx, total)
	} else {
		g.Players[playerIdx].JailTurns++
		attempt := g.Players[playerIdx].JailTurns
		if attempt >= gamerepo.MaxJailAttempts {
			if g.Players[playerIdx].Cash < gamerepo.JailFine {
				// Stay in jail until PayJailFine / UseJailCard; dice shown, no move.
				g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
				g.LastRoll = &gamerepo.LastRoll{
					UserID:        userID,
					Username:      g.Players[playerIdx].Username,
					Die1:          die1,
					Die2:          die2,
					Total:         total,
					FromIndex:     from,
					ToIndex:       from,
					PassedGo:      false,
					PassGoAmount:  0,
					IsDoubles:     false,
					DoublesStreak: 0,
					ThirdDoubles:  false,
				}
			} else {
				if err := payJailFineFull(g, playerIdx); err != nil {
					return nil, err
				}
				leaveJail(&g.Players[playerIdx])
				to, passedGo, passAmt := applyBoardMove(g, playerIdx, from, total)
				g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
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
					IsDoubles:     false,
					DoublesStreak: 0,
					ThirdDoubles:  false,
				}
				s.resolveLandingLocked(ctx, g, playerIdx, total)
			}
		} else {
			g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
			g.LastRoll = &gamerepo.LastRoll{
				UserID:        userID,
				Username:      g.Players[playerIdx].Username,
				Die1:          die1,
				Die2:          die2,
				Total:         total,
				FromIndex:     from,
				ToIndex:       from,
				PassedGo:      false,
				PassGoAmount:  0,
				IsDoubles:     false,
				DoublesStreak: 0,
				ThirdDoubles:  false,
			}
		}
	}

	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// PayJailFine leaves Jail for JailFine MeetCoin (Phase 12.1). Caller then Rolls to move.
func (s *service) PayJailFine(ctx context.Context, gameID, userID string) (*View, error) {
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
	if !g.Players[playerIdx].InJail {
		return nil, ErrNotInJail
	}
	if hasPendingPayment(g) {
		return nil, ErrMustSettle
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingRoll && g.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
		return nil, ErrNotInJail
	}
	if err := payJailFineFull(g, playerIdx); err != nil {
		return nil, err
	}
	leaveJail(&g.Players[playerIdx])
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	g.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// UseJailCard spends one GOOJF to leave Jail (Phase 12.1). Caller then Rolls to move.
func (s *service) UseJailCard(ctx context.Context, gameID, userID string) (*View, error) {
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
	if !g.Players[playerIdx].InJail {
		return nil, ErrNotInJail
	}
	if hasPendingPayment(g) {
		return nil, ErrMustSettle
	}
	cardID, ok := popJailFreeCard(&g.Players[playerIdx])
	if !ok {
		return nil, ErrNoJailCard
	}
	returnJailCardToDeck(g, cardID)
	leaveJail(&g.Players[playerIdx])
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
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
	if offer := openBuyOffer(g, s.loadSpaces(ctx, g.WorldID)); offer != nil {
		return nil, ErrMustBuy
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
	g.Players[playerIdx].HubID = ""
	g.Players[playerIdx].HubRevision++
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
		Houses:      0,
		Mortgaged:   false,
	})
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// Build purchases one house/hotel step on an owned property (Phase 11.1).
func (s *service) Build(ctx context.Context, gameID, userID string, boardIndex int) (*View, error) {
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
	sp := spaceAt(spaces, boardIndex)
	if sp == nil {
		return nil, ErrInvalidBoardIndex
	}
	if sp.Kind != "property" || sp.ColorGroup == "" || sp.HouseCost <= 0 {
		return nil, ErrNotBuildable
	}

	deedIdx := -1
	for i, d := range g.Deeds {
		if d.BoardIndex == boardIndex {
			deedIdx = i
			break
		}
	}
	if deedIdx < 0 || g.Deeds[deedIdx].OwnerUserID != userID {
		return nil, ErrNotOwner
	}
	if g.Deeds[deedIdx].Houses >= 5 {
		return nil, ErrMaxBuilt
	}
	if !ownsFullColorGroup(spaces, g.Deeds, userID, sp.ColorGroup) {
		return nil, ErrNoMonopoly
	}
	if colorGroupHasMortgage(spaces, g.Deeds, sp.ColorGroup) {
		return nil, ErrMortgagedSet
	}
	minH := minHousesInColorGroup(spaces, g.Deeds, userID, sp.ColorGroup)
	curH := g.Deeds[deedIdx].Houses
	if curH < 0 {
		curH = 0
	}
	if curH != minH {
		return nil, ErrUnevenBuild
	}
	if g.Players[playerIdx].Cash < sp.HouseCost {
		return nil, ErrCannotAfford
	}

	g.Players[playerIdx].Cash -= sp.HouseCost
	g.Deeds[deedIdx].Houses = curH + 1
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// SellBuilding sells one house/hotel step back to the bank at half houseCost (Phase 11.2).
// Allowed during pendingPayment so the player can raise funds; refund applies toward the debt.
func (s *service) SellBuilding(ctx context.Context, gameID, userID string, boardIndex int) (*View, error) {
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
	// Intentionally NOT blocked by pendingPayment — sell raises funds (option A).

	spaces := s.loadSpaces(ctx, g.WorldID)
	sp := spaceAt(spaces, boardIndex)
	if sp == nil {
		return nil, ErrInvalidBoardIndex
	}
	if sp.Kind != "property" || sp.ColorGroup == "" || sp.HouseCost <= 0 {
		return nil, ErrNotBuildable
	}

	deedIdx := -1
	for i, d := range g.Deeds {
		if d.BoardIndex == boardIndex {
			deedIdx = i
			break
		}
	}
	if deedIdx < 0 || g.Deeds[deedIdx].OwnerUserID != userID {
		return nil, ErrNotOwner
	}
	curH := g.Deeds[deedIdx].Houses
	if curH < 0 {
		curH = 0
	}
	if curH <= 0 {
		return nil, ErrNothingToSell
	}
	if !ownsFullColorGroup(spaces, g.Deeds, userID, sp.ColorGroup) {
		return nil, ErrNoMonopoly
	}
	maxH := maxHousesInColorGroup(spaces, g.Deeds, userID, sp.ColorGroup)
	if curH != maxH {
		return nil, ErrUnevenSell
	}

	refund := sp.HouseCost / 2
	g.Players[playerIdx].Cash += refund
	g.Deeds[deedIdx].Houses = curH - 1
	trySettlePendingLocked(g, playerIdx)
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// mortgageValue is half the list price (classic bank loan).
func mortgageValue(listPrice int) int {
	if listPrice <= 0 {
		return 0
	}
	return listPrice / 2
}

// redeemCost is mortgage value + 10% interest (classic).
func redeemCost(listPrice int) int {
	mv := mortgageValue(listPrice)
	return mv + mv/10
}

// Mortgage mortgages an owned buyable deed for half list price (Phase 11.3).
// Allowed during pendingPayment; payout applies toward outstanding debt.
func (s *service) Mortgage(ctx context.Context, gameID, userID string, boardIndex int) (*View, error) {
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

	spaces := s.loadSpaces(ctx, g.WorldID)
	sp := spaceAt(spaces, boardIndex)
	if sp == nil {
		return nil, ErrInvalidBoardIndex
	}
	if !isBuyableKind(sp.Kind) || sp.Price <= 0 {
		return nil, ErrCannotMortgage
	}

	deedIdx := -1
	for i, d := range g.Deeds {
		if d.BoardIndex == boardIndex {
			deedIdx = i
			break
		}
	}
	if deedIdx < 0 || g.Deeds[deedIdx].OwnerUserID != userID {
		return nil, ErrNotOwner
	}
	if g.Deeds[deedIdx].Mortgaged {
		return nil, ErrAlreadyMortgaged
	}
	if sp.Kind == "property" && sp.ColorGroup != "" {
		if colorGroupHasBuildings(spaces, g.Deeds, sp.ColorGroup) {
			return nil, ErrMustSellBuildings
		}
	} else if g.Deeds[deedIdx].Houses > 0 {
		return nil, ErrMustSellBuildings
	}

	payout := mortgageValue(sp.Price)
	g.Deeds[deedIdx].Mortgaged = true
	g.Players[playerIdx].Cash += payout
	trySettlePendingLocked(g, playerIdx)
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// Redeem unmortgages an owned deed for mortgage value + 10% (Phase 11.3).
func (s *service) Redeem(ctx context.Context, gameID, userID string, boardIndex int) (*View, error) {
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
	sp := spaceAt(spaces, boardIndex)
	if sp == nil {
		return nil, ErrInvalidBoardIndex
	}
	if !isBuyableKind(sp.Kind) || sp.Price <= 0 {
		return nil, ErrCannotMortgage
	}

	deedIdx := -1
	for i, d := range g.Deeds {
		if d.BoardIndex == boardIndex {
			deedIdx = i
			break
		}
	}
	if deedIdx < 0 || g.Deeds[deedIdx].OwnerUserID != userID {
		return nil, ErrNotOwner
	}
	if !g.Deeds[deedIdx].Mortgaged {
		return nil, ErrNotMortgaged
	}

	cost := redeemCost(sp.Price)
	if g.Players[playerIdx].Cash < cost {
		return nil, ErrCannotAfford
	}

	g.Players[playerIdx].Cash -= cost
	g.Deeds[deedIdx].Mortgaged = false
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

// EnterHub records that a seated active player is inside a location hub (Phase 8.2).
// When clientRevision is set and older than HubRevision, the enter is ignored (Phase 8.4 stale guard).
func (s *service) EnterHub(ctx context.Context, gameID, userID, hubID string, clientRevision *int64) (*View, error) {
	normalized, ok := normalizeHubID(hubID)
	if !ok {
		return nil, ErrInvalidHubID
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
	if g.Status != gamerepo.StatusActive {
		return nil, ErrInactive
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
	if g.Players[playerIdx].Resigned {
		return nil, ErrAlreadyOut
	}
	if clientRevision != nil && *clientRevision < g.Players[playerIdx].HubRevision {
		// Stale enter that lost a race with LeaveHub — keep current hubId.
		return s.viewOf(ctx, g), nil
	}
	if g.Players[playerIdx].HubID == normalized {
		return s.viewOf(ctx, g), nil
	}

	g.Players[playerIdx].HubID = normalized
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// LeaveHub clears the player's hubId (Phase 8.2) and bumps HubRevision (Phase 8.4).
func (s *service) LeaveHub(ctx context.Context, gameID, userID string) (*View, error) {
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

	// Always bump revision so in-flight EnterHub with an older revision cannot restick hubId.
	g.Players[playerIdx].HubRevision++
	changed := g.Players[playerIdx].HubID != ""
	g.Players[playerIdx].HubID = ""
	if !changed {
		// Still persist revision bump even when hubId was already empty.
		g.UpdatedAt = time.Now().UTC()
		if err := s.repo.Update(ctx, g); err != nil {
			return nil, err
		}
		return s.viewOf(ctx, g), nil
	}

	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func normalizeHubID(raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", false
	}
	if !strings.HasPrefix(id, "hub:") {
		id = "hub:" + id
	}
	// hub:{world}:{slug} — at least one colon after the prefix.
	rest := strings.TrimPrefix(id, "hub:")
	if rest == "" || !strings.Contains(rest, ":") {
		return "", false
	}
	return id, true
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
	v := toView(g, s.loadSpaces(ctx, g.WorldID))
	s.enrichCountries(ctx, v)
	return v
}

func (s *service) enrichCountries(ctx context.Context, v *View) {
	if v == nil || s.countries == nil {
		return
	}
	lookup := s.countries
	for i := range v.Players {
		if c := strings.ToUpper(strings.TrimSpace(lookup.CountryForUser(ctx, v.Players[i].UserID))); c != "" {
			v.Players[i].Country = c
		}
	}
}

// openBuyOffer is set when the current player just landed on unowned buyable land.
// Until Phase 13 auction, EndTurn is blocked while this is non-nil.
func openBuyOffer(g *gamerepo.Game, spaces []Space) *BuyOfferView {
	if g == nil || g.Status != gamerepo.StatusActive {
		return nil
	}
	idx := currentPlayerIndex(g)
	if idx < 0 || g.Players[idx].Resigned {
		return nil
	}
	p := g.Players[idx]
	sp := spaceAt(spaces, p.BoardIndex)
	if sp == nil || !isBuyableKind(sp.Kind) || sp.Price <= 0 || ownerOf(g.Deeds, sp.BoardIndex) != "" {
		return nil
	}
	if g.LastRoll == nil ||
		g.LastRoll.UserID != p.UserID ||
		g.LastRoll.ToIndex != sp.BoardIndex ||
		g.LastRoll.ThirdDoubles {
		return nil
	}
	return &BuyOfferView{
		BoardIndex: sp.BoardIndex,
		Slug:       sp.Slug,
		Name:       sp.Name,
		Kind:       sp.Kind,
		Price:      sp.Price,
	}
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
		goojf := len(p.GetOutOfJailFreeCards)
		if goojf == 0 {
			goojf = p.GetOutOfJailFree
		}
		players[i] = PlayerView{
			UserID:           p.UserID,
			Username:         p.Username,
			SeatIndex:        p.SeatIndex,
			TurnOrder:        p.TurnOrder,
			Cash:             p.Cash,
			BoardIndex:       p.BoardIndex,
			PinColor:         p.PinColor,
			Resigned:         p.Resigned,
			TimeRemainingMs:  liveRemainingMs(g, p, now),
			HubID:            p.HubID,
			HubRevision:      p.HubRevision,
			InJail:           p.InJail,
			JailTurns:        p.JailTurns,
			GetOutOfJailFree: goojf,
		}
		if !p.Resigned && p.TurnOrder == g.CurrentTurn {
			currentUserID = p.UserID
			currentUsername = p.Username
		}
	}
	deeds := make([]DeedView, 0, len(g.Deeds))
	for _, d := range g.Deeds {
		houses := d.Houses
		if houses < 0 {
			houses = 0
		}
		if houses > 5 {
			houses = 5
		}
		deeds = append(deeds, DeedView{
			BoardIndex:    d.BoardIndex,
			OwnerUserID:   d.OwnerUserID,
			OwnerUsername: nameByID[d.OwnerUserID],
			Houses:        houses,
			Mortgaged:     d.Mortgaged,
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
	if active && !pending {
		buyOffer = openBuyOffer(g, spaces)
		canBuy = buyOffer != nil
	}

	currentInJail := false
	currentJailTurns := 0
	currentCash := 0
	currentCards := 0
	if idx := currentPlayerIndex(g); idx >= 0 {
		currentInJail = g.Players[idx].InJail
		currentJailTurns = g.Players[idx].JailTurns
		currentCash = g.Players[idx].Cash
		currentCards = len(g.Players[idx].GetOutOfJailFreeCards)
		if currentCards == 0 {
			currentCards = g.Players[idx].GetOutOfJailFree
		}
	}

	canRoll := active && phase == gamerepo.TurnPhaseAwaitingRoll && !pending
	if canRoll && currentInJail && currentJailTurns >= gamerepo.MaxJailAttempts {
		// Must pay fine or use card after 3 failed doubles attempts.
		canRoll = false
	}
	canPayJail := active && currentInJail && !pending && currentCash >= gamerepo.JailFine &&
		(phase == gamerepo.TurnPhaseAwaitingRoll || phase == gamerepo.TurnPhaseAwaitingEnd)
	canUseCard := active && currentInJail && !pending && currentCards > 0 &&
		(phase == gamerepo.TurnPhaseAwaitingRoll || phase == gamerepo.TurnPhaseAwaitingEnd)

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
	var lastCard *LastCardView
	if g.LastCard != nil {
		lastCard = &LastCardView{
			Deck:      g.LastCard.Deck,
			CardID:    g.LastCard.CardID,
			Title:     g.LastCard.Title,
			UserID:    g.LastCard.UserID,
			Username:  g.LastCard.Username,
			CashDelta: g.LastCard.CashDelta,
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
		CanRoll:         canRoll,
		CanEndTurn:      active && phase == gamerepo.TurnPhaseAwaitingEnd && !pending && buyOffer == nil,
		CanBuy:          canBuy,
		CanPayJailFine:  canPayJail,
		CanUseJailCard:  canUseCard,
		BuyOffer:        buyOffer,
		Deeds:           deeds,
		LastRoll:        last,
		LastPayment:     lastPay,
		PendingPayment:  pend,
		LastCard:        lastCard,
		WinnerUserID:    g.WinnerUserID,
		WinnerUsername:  g.WinnerUsername,
		TurnStartedAt:   started,
	}
}

func hasPendingPayment(g *gamerepo.Game) bool {
	return g.PendingPayment != nil && g.PendingPayment.Amount > 0
}

// trySettlePendingLocked applies available cash toward an outstanding rent/tax remainder.
func trySettlePendingLocked(g *gamerepo.Game, payerIdx int) {
	if !hasPendingPayment(g) || payerIdx < 0 || payerIdx >= len(g.Players) {
		return
	}
	remaining := g.PendingPayment.Amount
	pay := remaining
	if g.Players[payerIdx].Cash < remaining {
		pay = g.Players[payerIdx].Cash
	}
	if pay <= 0 {
		return
	}

	g.Players[payerIdx].Cash -= pay
	toUserID := g.PendingPayment.ToUserID
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

	paidInFull := pay >= remaining
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         g.PendingPayment.Kind,
		FromUserID:   g.Players[payerIdx].UserID,
		FromUsername: g.Players[payerIdx].Username,
		ToUserID:     toUserID,
		ToUsername:   toUsername,
		Amount:       pay,
		BoardIndex:   g.PendingPayment.BoardIndex,
		SpaceName:    g.PendingPayment.SpaceName,
		PaidInFull:   paidInFull,
	}
	if paidInFull {
		g.PendingPayment = nil
	} else {
		g.PendingPayment.Amount = remaining - pay
	}
}

// resolveLandingLocked auto-collects rent/tax after a move (Phase 6.5).
// Phase 12.0: land on go_to_jail → Jail; land on jail = Just Visiting.
// Phase 12.2–12.3: land on chance / community_chest → draw + apply effects (lock A).
func (s *service) resolveLandingLocked(ctx context.Context, g *gamerepo.Game, payerIdx, diceTotal int) {
	s.resolveLandingWithOpts(ctx, g, payerIdx, diceTotal, landingOpts{})
}

func (s *service) resolveLandingWithOpts(
	ctx context.Context, g *gamerepo.Game, payerIdx, diceTotal int, opts landingOpts,
) {
	g.PendingPayment = nil
	spaces := s.loadSpaces(ctx, g.WorldID)
	boardIndex := g.Players[payerIdx].BoardIndex
	sp := spaceAt(spaces, boardIndex)
	if sp != nil && sp.SpecialType == "go_to_jail" {
		jail := sendPlayerToJail(g, payerIdx, spaces)
		if g.LastRoll != nil {
			g.LastRoll.ToIndex = jail
		}
		return
	}
	if sp != nil && (sp.SpecialType == DeckChance || sp.SpecialType == DeckChest) {
		id := drawCardLocked(g, payerIdx, sp.SpecialType, spaces)
		s.applyCardEffectLocked(ctx, g, payerIdx, id)
		return
	}

	payerID := g.Players[payerIdx].UserID
	amount, toUserID, kind, spaceName := rentDueForLanding(
		spaces, g.Deeds, boardIndex, payerID, diceTotal,
	)
	if opts.utilityDiceTotal > 0 && sp != nil && sp.Kind == "utility" {
		ownerID := ownerOf(g.Deeds, boardIndex)
		if ownerID != "" && ownerID != payerID && !deedMortgaged(g.Deeds, boardIndex) {
			amount = 10 * opts.utilityDiceTotal
			kind = "rent"
			toUserID = ownerID
			spaceName = sp.Name
		}
	}
	if opts.rentMultiplier > 1 && amount > 0 && kind == "rent" {
		amount *= opts.rentMultiplier
	}
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
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	}
}

// jailBoardIndex returns the Jail / Just Visiting space, defaulting to classic index 10.
func jailBoardIndex(spaces []Space) int {
	for _, sp := range spaces {
		if sp.SpecialType == "jail" {
			return sp.BoardIndex
		}
	}
	return gamerepo.JailBoardIndex
}

// sendPlayerToJail moves the player to Jail, flags inJail, ends the turn, clears doubles streak.
// Does not award Pass GO for the teleport (Phase 12.0).
func sendPlayerToJail(g *gamerepo.Game, playerIdx int, spaces []Space) int {
	jail := jailBoardIndex(spaces)
	if playerIdx < 0 || playerIdx >= len(g.Players) {
		return jail
	}
	g.Players[playerIdx].BoardIndex = jail
	g.Players[playerIdx].InJail = true
	g.Players[playerIdx].JailTurns = 0
	g.DoublesStreak = 0
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.PendingPayment = nil
	return jail
}

func leaveJail(p *gamerepo.Player) {
	if p == nil {
		return
	}
	p.InJail = false
	p.JailTurns = 0
}

func payJailFineFull(g *gamerepo.Game, playerIdx int) error {
	if playerIdx < 0 || playerIdx >= len(g.Players) {
		return ErrNotPlayer
	}
	if g.Players[playerIdx].Cash < gamerepo.JailFine {
		return ErrCannotAfford
	}
	g.Players[playerIdx].Cash -= gamerepo.JailFine
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         "jail_fine",
		FromUserID:   g.Players[playerIdx].UserID,
		FromUsername: g.Players[playerIdx].Username,
		ToUserID:     "",
		ToUsername:   "Bank",
		Amount:       gamerepo.JailFine,
		BoardIndex:   g.Players[playerIdx].BoardIndex,
		SpaceName:    "Jail",
		PaidInFull:   true,
	}
	return nil
}

// applyBoardMove walks total spaces from fromIndex; awards Pass GO. Returns to, passedGo, passAmt.
func applyBoardMove(g *gamerepo.Game, playerIdx, fromIndex, total int) (to int, passedGo bool, passAmt int) {
	to = (fromIndex + total) % gamerepo.BoardSpaceCount
	passedGo = fromIndex+total >= gamerepo.BoardSpaceCount
	if passedGo {
		passAmt = g.PassGoBonus
		if passAmt <= 0 {
			passAmt = gamerepo.PassGoBonus
		}
		g.Players[playerIdx].Cash += passAmt
	}
	g.Players[playerIdx].BoardIndex = to
	return to, passedGo, passAmt
}
