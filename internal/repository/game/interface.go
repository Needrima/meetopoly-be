package game

import (
	"context"
	"time"
)

const (
	StatusActive   = "active"
	StatusFinished = "finished"

	StartingCash    = 2000
	PassGoBonus     = 200
	GoBoardIndex    = 0
	JailBoardIndex  = 10 // classic Jail / Just Visiting; worlds seed specialType "jail"
	BoardSpaceCount = 40
	// JailFine — MeetCoin to leave Jail by paying (Phase 12.1).
	JailFine = 100
	// MaxJailAttempts — failed doubles tries before fine is required (Phase 12.1).
	MaxJailAttempts = 3

	// AuctionBidTurn — per-bidder clock while an auction is active (Phase 13.0 / 13.1).
	AuctionBidTurn = 60 * time.Second
	// AuctionMinBid — floor when high bid is 0 (Phase 13.0).
	AuctionMinBid = 1

	// TurnClockDuration — Phase 13.2 per-turn clock (fresh each time you become current).
	TurnClockDuration = 3 * time.Minute
	// TurnTimeoutResignAfter — auto-resign after this many turn-clock expiries (Phase 13.2).
	TurnTimeoutResignAfter = 2
	// TradeReplyTimeout — target must accept/decline within this window (Phase 13.2).
	TradeReplyTimeout = 60 * time.Second

	// Deprecated alias — use TurnClockDuration (Phase 13.2 replaced 45m banks).
	TimeBankDuration = TurnClockDuration

	// TurnPhaseAwaitingRoll — current player may roll (start of turn or after doubles).
	TurnPhaseAwaitingRoll = "awaiting_roll"
	// TurnPhaseAwaitingEnd — current player must End (non-doubles roll, or third doubles).
	TurnPhaseAwaitingEnd = "awaiting_end"
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
	// HubID is set while the player is inside a location hub (Phase 8.2); empty = on board.
	HubID string `bson:"hubId,omitempty" json:"hubId,omitempty"`
	// HubRevision increments on LeaveHub (and resign). EnterHub with an older revision is ignored (Phase 8.4).
	HubRevision int64 `bson:"hubRevision,omitempty" json:"hubRevision,omitempty"`
	// Resigned — left mid-game or time-bank eliminated; skipped for turns. Assets frozen until Phase 14.
	Resigned bool `bson:"resigned,omitempty" json:"resigned,omitempty"`
	// TimeRemainingMs — personal turn clock remainder when last paused (Phase 13.2: 3m fresh per turn).
	TimeRemainingMs int64 `bson:"timeRemainingMs" json:"timeRemainingMs"`
	// TurnTimeouts — how many times this player's turn clock hit 0 (Phase 13.2); 2 → auto-resign.
	TurnTimeouts int `bson:"turnTimeouts,omitempty" json:"turnTimeouts,omitempty"`
	// InJail — true when sent to Jail (Go to Jail / third doubles / card). Land on Jail without this = Just Visiting (Phase 12.0).
	InJail bool `bson:"inJail,omitempty" json:"inJail,omitempty"`
	// JailTurns — failed exit attempts while in Jail (Phase 12.1); 0 on entry.
	JailTurns int `bson:"jailTurns,omitempty" json:"jailTurns,omitempty"`
	// GetOutOfJailFree — Chance/Chest GOOJF cards held (Phase 12.2+); count of GetOutOfJailFreeCards.
	GetOutOfJailFree int `bson:"getOutOfJailFree,omitempty" json:"getOutOfJailFree,omitempty"`
	// GetOutOfJailFreeCards — card ids held (chance_get_out_of_jail / chest_get_out_of_jail).
	GetOutOfJailFreeCards []string `bson:"getOutOfJailFreeCards,omitempty" json:"getOutOfJailFreeCards,omitempty"`
}

// Deed is ownership of a buyable board space (Phase 6.4+).
// Houses: 0–5 where 5 = hotel (Phase 11.0). Mortgaged stays false until 11.3.
type Deed struct {
	BoardIndex  int    `bson:"boardIndex" json:"boardIndex"`
	OwnerUserID string `bson:"ownerUserId" json:"ownerUserId"`
	Houses      int    `bson:"houses" json:"houses"`
	Mortgaged   bool   `bson:"mortgaged" json:"mortgaged"`
}

// LastPayment is the most recent rent/tax transfer (Phase 6.5) for client toasts.
type LastPayment struct {
	Kind         string `bson:"kind" json:"kind"` // rent | tax
	FromUserID   string `bson:"fromUserId" json:"fromUserId"`
	FromUsername string `bson:"fromUsername" json:"fromUsername"`
	ToUserID     string `bson:"toUserId,omitempty" json:"toUserId,omitempty"` // empty = Bank
	ToUsername   string `bson:"toUsername,omitempty" json:"toUsername,omitempty"`
	Amount       int    `bson:"amount" json:"amount"`
	BoardIndex   int    `bson:"boardIndex" json:"boardIndex"`
	SpaceName    string `bson:"spaceName" json:"spaceName"`
	PaidInFull   bool   `bson:"paidInFull" json:"paidInFull"`
}

// PendingPayment — unpaid remainder after a land; blocks Roll/End until resign / Phase 14.
type PendingPayment struct {
	Kind       string `bson:"kind" json:"kind"` // rent | tax
	Amount     int    `bson:"amount" json:"amount"`
	ToUserID   string `bson:"toUserId,omitempty" json:"toUserId,omitempty"`
	BoardIndex int    `bson:"boardIndex" json:"boardIndex"`
	SpaceName  string `bson:"spaceName" json:"spaceName"`
}

// LastRoll is the most recent dice result (Phase 6.1+).
type LastRoll struct {
	UserID        string `bson:"userId" json:"userId"`
	Username      string `bson:"username" json:"username"`
	Die1          int    `bson:"die1" json:"die1"`
	Die2          int    `bson:"die2" json:"die2"`
	Total         int    `bson:"total" json:"total"`
	FromIndex     int    `bson:"fromIndex" json:"fromIndex"`
	ToIndex       int    `bson:"toIndex" json:"toIndex"`
	PassedGo      bool   `bson:"passedGo" json:"passedGo"`
	PassGoAmount  int    `bson:"passGoAmount,omitempty" json:"passGoAmount,omitempty"`
	IsDoubles     bool   `bson:"isDoubles" json:"isDoubles"`
	DoublesStreak int    `bson:"doublesStreak" json:"doublesStreak"`
	// ThirdDoubles — rolled doubles three times; sent to Jail (Phase 12.0).
	ThirdDoubles bool `bson:"thirdDoubles,omitempty" json:"thirdDoubles,omitempty"`
}

// LastCard is the most recently drawn Chance / Community Chest card (Phase 12.2).
type LastCard struct {
	Deck     string `bson:"deck" json:"deck"` // chance | community_chest
	CardID   string `bson:"cardId" json:"cardId"`
	Title    string `bson:"title" json:"title"`
	UserID   string `bson:"userId" json:"userId"`
	Username string `bson:"username" json:"username"`
	// CashDelta — signed MeetCoin for the drawer's net from this card (0 = none).
	CashDelta int `bson:"cashDelta,omitempty" json:"cashDelta,omitempty"`
}

// AuctionEvent is one bid/fold line in the auction feed (Phase 13.0).
type AuctionEvent struct {
	Kind     string `bson:"kind" json:"kind"` // bid | fold | auto_bid | auto_fold
	UserID   string `bson:"userId" json:"userId"`
	Username string `bson:"username" json:"username"`
	Amount   int    `bson:"amount,omitempty" json:"amount,omitempty"`
}

// Auction is an active bank auction for one unowned space (Phase 13.0).
type Auction struct {
	BoardIndex            int            `bson:"boardIndex" json:"boardIndex"`
	Slug                  string         `bson:"slug" json:"slug"`
	Name                  string         `bson:"name" json:"name"`
	Kind                  string         `bson:"kind" json:"kind"`
	ListPrice             int            `bson:"listPrice" json:"listPrice"`
	HighBid               int            `bson:"highBid" json:"highBid"`
	HighBidderUserID      string         `bson:"highBidderUserId,omitempty" json:"highBidderUserId,omitempty"`
	CurrentBidderUserID   string         `bson:"currentBidderUserId" json:"currentBidderUserId"`
	BidderTurnStartedAt   time.Time      `bson:"bidderTurnStartedAt" json:"bidderTurnStartedAt"`
	FoldedUserIDs         []string       `bson:"foldedUserIds,omitempty" json:"foldedUserIds,omitempty"`
	History               []AuctionEvent `bson:"history,omitempty" json:"history,omitempty"`
	StartedByUserID       string         `bson:"startedByUserId" json:"startedByUserId"`
}

// LastAuction is the most recent auction settle/void (Phase 13.0) for client toasts.
type LastAuction struct {
	BoardIndex     int    `bson:"boardIndex" json:"boardIndex"`
	SpaceName      string `bson:"spaceName" json:"spaceName"`
	WinnerUserID   string `bson:"winnerUserId,omitempty" json:"winnerUserId,omitempty"`
	WinnerUsername string `bson:"winnerUsername,omitempty" json:"winnerUsername,omitempty"`
	Amount         int    `bson:"amount,omitempty" json:"amount,omitempty"`
	Void           bool   `bson:"void,omitempty" json:"void,omitempty"`
}

// TradeSide is one side of a trade offer (Phase 13.2).
type TradeSide struct {
	Cash              int   `bson:"cash,omitempty" json:"cash,omitempty"`
	BoardIndexes      []int `bson:"boardIndexes,omitempty" json:"boardIndexes,omitempty"`
	GetOutOfJailFree  int   `bson:"getOutOfJailFree,omitempty" json:"getOutOfJailFree,omitempty"`
}

// TradeOffer is the single open player-to-player trade (Phase 13.2).
type TradeOffer struct {
	FromUserID    string    `bson:"fromUserId" json:"fromUserId"`
	ToUserID      string    `bson:"toUserId" json:"toUserId"`
	Give          TradeSide `bson:"give" json:"give"` // from → to
	Take          TradeSide `bson:"take" json:"take"` // to → from
	ReplyDeadline time.Time `bson:"replyDeadline" json:"replyDeadline"`
}

// LastForfeit is set on turn-clock strike / auto-resign / resign for client toasts (Phase 13.2).
type LastForfeit struct {
	UserID   string `bson:"userId" json:"userId"`
	Username string `bson:"username" json:"username"`
	// Reason — turn_strike (1st clock expiry) | turn_timeout (2nd → kicked) | resign.
	Reason string `bson:"reason" json:"reason"`
	// Strikes — turn-timeout count after this event (1 or 2); 0 for manual resign.
	Strikes int `bson:"strikes,omitempty" json:"strikes,omitempty"`
}

// Game is the authoritative M1 session (Phase 6+).
type Game struct {
	ID          string   `bson:"_id" json:"id"`
	TableID     string   `bson:"tableId" json:"tableId"`
	WorldID     string   `bson:"worldId" json:"worldId"`
	Status      string   `bson:"status" json:"status"`
	Players     []Player `bson:"players" json:"players"`
	CurrentTurn int      `bson:"currentTurn" json:"currentTurn"` // turnOrder of active player
	PassGoBonus int      `bson:"passGoBonus" json:"passGoBonus"`
	// Deeds — owned buyable spaces (Phase 6.4). Unowned spaces are absent.
	Deeds []Deed `bson:"deeds,omitempty" json:"deeds,omitempty"`
	// LastPayment — most recent auto rent/tax (Phase 6.5).
	LastPayment *LastPayment `bson:"lastPayment,omitempty" json:"lastPayment,omitempty"`
	// PendingPayment — unpaid remainder; blocks turn actions until settled (Phase 6.5 / 14).
	PendingPayment *PendingPayment `bson:"pendingPayment,omitempty" json:"pendingPayment,omitempty"`
	// TurnPhase — awaiting_roll | awaiting_end (Phase 6.2).
	TurnPhase string `bson:"turnPhase" json:"turnPhase"`
	// DoublesStreak — consecutive doubles this turn (0–3).
	DoublesStreak int       `bson:"doublesStreak" json:"doublesStreak"`
	LastRoll      *LastRoll `bson:"lastRoll,omitempty" json:"lastRoll,omitempty"`
	// ChanceDeck — remaining Chance cards top-first (Phase 12.2). Not exposed on public View.
	ChanceDeck []string `bson:"chanceDeck,omitempty" json:"chanceDeck,omitempty"`
	// ChestDeck — remaining Community Chest cards top-first (Phase 12.2).
	ChestDeck []string `bson:"chestDeck,omitempty" json:"chestDeck,omitempty"`
	// LastCard — most recent Chance/Chest draw (Phase 12.2); effects in 12.3.
	LastCard *LastCard `bson:"lastCard,omitempty" json:"lastCard,omitempty"`
	// Auction — active bank auction (Phase 13.0); nil when none.
	Auction *Auction `bson:"auction,omitempty" json:"auction,omitempty"`
	// LastAuction — most recent auction result for toasts (Phase 13.0).
	LastAuction *LastAuction `bson:"lastAuction,omitempty" json:"lastAuction,omitempty"`
	// SuppressBuyOffer — after auction settle/void this turn; cleared on EndTurn (Phase 13.0).
	SuppressBuyOffer bool `bson:"suppressBuyOffer,omitempty" json:"suppressBuyOffer,omitempty"`
	// Trade — single open trade offer (Phase 13.2); nil when none.
	Trade *TradeOffer `bson:"trade,omitempty" json:"trade,omitempty"`
	// LastForfeit — most recent auto/manual forfeit for toasts (Phase 13.2).
	LastForfeit *LastForfeit `bson:"lastForfeit,omitempty" json:"lastForfeit,omitempty"`
	// WinnerUserID — set when StatusFinished (last player standing).
	WinnerUserID   string `bson:"winnerUserId,omitempty" json:"winnerUserId,omitempty"`
	WinnerUsername string `bson:"winnerUsername,omitempty" json:"winnerUsername,omitempty"`
	// TurnStartedAt — when the current player's bank started draining (Phase 6.3b).
	TurnStartedAt time.Time `bson:"turnStartedAt,omitempty" json:"turnStartedAt,omitempty"`
	CreatedAt     time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time `bson:"updatedAt" json:"updatedAt"`
}

// Repository persists games.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Insert(ctx context.Context, g *Game) error
	Update(ctx context.Context, g *Game) error
	FindByID(ctx context.Context, id string) (*Game, error)
	FindByTableID(ctx context.Context, tableID string) (*Game, error)
}
