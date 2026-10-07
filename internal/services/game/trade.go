package game

import (
	"context"
	"errors"
	"time"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	ErrNoTrade             = errors.New("no open trade offer")
	ErrTradeActive         = errors.New("a trade offer is already open")
	ErrInvalidTrade        = errors.New("invalid trade offer")
	ErrNotTradeTarget      = errors.New("not the trade target")
	ErrTradeNeedsDeed      = errors.New("cash-for-cash trades are not allowed")
	ErrTradeHasBuildings   = errors.New("cannot trade properties with houses or hotels")
	ErrTradeMortgageChoice = errors.New("mortgageAction required: redeem_all or leave_all")
)

// TradeSideView is one side of a public trade offer (Phase 13.2).
type TradeSideView struct {
	Cash             int   `json:"cash"`
	BoardIndexes     []int `json:"boardIndexes"`
	GetOutOfJailFree int   `json:"getOutOfJailFree"`
}

// TradeView is the public open trade snapshot (Phase 13.2).
type TradeView struct {
	FromUserID    string        `json:"fromUserId"`
	FromUsername  string        `json:"fromUsername"`
	ToUserID      string        `json:"toUserId"`
	ToUsername    string        `json:"toUsername"`
	Give          TradeSideView `json:"give"`
	Take          TradeSideView `json:"take"`
	ReplyDeadline string        `json:"replyDeadline"`
}

// LastTradeView for accept/decline toasts (Phase 13.3).
type LastTradeView struct {
	FromUserID   string `json:"fromUserId"`
	FromUsername string `json:"fromUsername"`
	ToUserID     string `json:"toUserId"`
	ToUsername   string `json:"toUsername"`
	Outcome      string `json:"outcome"` // accepted | declined
	SettledAt    string `json:"settledAt"`
}

// LastForfeitView for client toasts (Phase 13.2).
type LastForfeitView struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Reason   string `json:"reason"`
	Strikes  int    `json:"strikes,omitempty"`
}

// TradeSideInput is the propose body side (Phase 13.2).
type TradeSideInput struct {
	Cash             int
	BoardIndexes     []int
	GetOutOfJailFree int
}

const (
	MortgageRedeemAll = "redeem_all"
	MortgageLeaveAll  = "leave_all"
)

func tradeSideViewOf(s gamerepo.TradeSide) TradeSideView {
	idxs := append([]int(nil), s.BoardIndexes...)
	if idxs == nil {
		idxs = []int{}
	}
	return TradeSideView{
		Cash:             s.Cash,
		BoardIndexes:     idxs,
		GetOutOfJailFree: s.GetOutOfJailFree,
	}
}

func tradeViewOf(g *gamerepo.Game) *TradeView {
	if g == nil || g.Trade == nil {
		return nil
	}
	t := g.Trade
	fromName, toName := "", ""
	if p := playerByID(g, t.FromUserID); p != nil {
		fromName = p.Username
	}
	if p := playerByID(g, t.ToUserID); p != nil {
		toName = p.Username
	}
	return &TradeView{
		FromUserID:    t.FromUserID,
		FromUsername:  fromName,
		ToUserID:      t.ToUserID,
		ToUsername:    toName,
		Give:          tradeSideViewOf(t.Give),
		Take:          tradeSideViewOf(t.Take),
		ReplyDeadline: t.ReplyDeadline.UTC().Format(time.RFC3339Nano),
	}
}

func lastTradeViewOf(g *gamerepo.Game) *LastTradeView {
	if g == nil || g.LastTrade == nil {
		return nil
	}
	lt := g.LastTrade
	return &LastTradeView{
		FromUserID:   lt.FromUserID,
		FromUsername: lt.FromUsername,
		ToUserID:     lt.ToUserID,
		ToUsername:   lt.ToUsername,
		Outcome:      lt.Outcome,
		SettledAt:    lt.SettledAt.UTC().Format(time.RFC3339Nano),
	}
}

const (
	TradeOutcomeAccepted = "accepted"
	TradeOutcomeDeclined = "declined"
)

// recordTradeOutcomeLocked snapshots the open trade into LastTrade then clears it.
func (s *service) recordTradeOutcomeLocked(g *gamerepo.Game, outcome string) {
	if g.Trade == nil {
		return
	}
	t := g.Trade
	g.LastTrade = &gamerepo.LastTrade{
		FromUserID:   t.FromUserID,
		FromUsername: playerUsername(g, t.FromUserID),
		ToUserID:     t.ToUserID,
		ToUsername:   playerUsername(g, t.ToUserID),
		Outcome:      outcome,
		SettledAt:    time.Now().UTC(),
	}
	s.clearTradeLocked(g)
}

func lastForfeitViewOf(g *gamerepo.Game) *LastForfeitView {
	if g == nil || g.LastForfeit == nil {
		return nil
	}
	return &LastForfeitView{
		UserID:   g.LastForfeit.UserID,
		Username: g.LastForfeit.Username,
		Reason:   g.LastForfeit.Reason,
		Strikes:  g.LastForfeit.Strikes,
	}
}

func tradeSideNonEmpty(s TradeSideInput) bool {
	return s.Cash > 0 || len(s.BoardIndexes) > 0 || s.GetOutOfJailFree > 0
}

func tradeSideHasDeedOrCard(s TradeSideInput) bool {
	return len(s.BoardIndexes) > 0 || s.GetOutOfJailFree > 0
}

func normalizeTradeSide(s TradeSideInput) (gamerepo.TradeSide, error) {
	if s.Cash < 0 || s.GetOutOfJailFree < 0 {
		return gamerepo.TradeSide{}, ErrInvalidTrade
	}
	seen := make(map[int]struct{}, len(s.BoardIndexes))
	out := make([]int, 0, len(s.BoardIndexes))
	for _, idx := range s.BoardIndexes {
		if idx < 0 || idx >= gamerepo.BoardSpaceCount {
			return gamerepo.TradeSide{}, ErrInvalidTrade
		}
		if _, ok := seen[idx]; ok {
			return gamerepo.TradeSide{}, ErrInvalidTrade
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
	}
	return gamerepo.TradeSide{
		Cash:             s.Cash,
		BoardIndexes:     out,
		GetOutOfJailFree: s.GetOutOfJailFree,
	}, nil
}

func deedOwnedBy(g *gamerepo.Game, boardIndex int, userID string) *gamerepo.Deed {
	for i := range g.Deeds {
		if g.Deeds[i].BoardIndex == boardIndex && g.Deeds[i].OwnerUserID == userID {
			return &g.Deeds[i]
		}
	}
	return nil
}

func validateTradeSideOwnership(g *gamerepo.Game, userID string, side gamerepo.TradeSide) error {
	idx := playerIndexByID(g, userID)
	if idx < 0 || g.Players[idx].Resigned {
		return ErrNotPlayer
	}
	if side.Cash > g.Players[idx].Cash {
		return ErrCannotAfford
	}
	if side.GetOutOfJailFree > g.Players[idx].GetOutOfJailFree {
		return ErrInvalidTrade
	}
	for _, bi := range side.BoardIndexes {
		d := deedOwnedBy(g, bi, userID)
		if d == nil {
			return ErrInvalidTrade
		}
		if d.Houses > 0 {
			return ErrTradeHasBuildings
		}
	}
	return nil
}

func (s *service) clearTradeLocked(g *gamerepo.Game) {
	g.Trade = nil
	s.cancelTradeTimerLocked(g.ID)
}

func (s *service) armTradeTimerLocked(g *gamerepo.Game) {
	s.cancelTradeTimerLocked(g.ID)
	if g.Trade == nil {
		return
	}
	delay := max(time.Until(g.Trade.ReplyDeadline), 0)
	gameID := g.ID
	s.tradeTimers[gameID] = time.AfterFunc(delay, func() {
		_ = s.onTradeReplyTimeout(context.Background(), gameID)
	})
}

func (s *service) cancelTradeTimerLocked(gameID string) {
	if t, ok := s.tradeTimers[gameID]; ok {
		t.Stop()
		delete(s.tradeTimers, gameID)
	}
}

func (s *service) onTradeReplyTimeout(ctx context.Context, gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		return err
	}
	if g.Trade == nil {
		return nil
	}
	if time.Now().Before(g.Trade.ReplyDeadline.Add(-50 * time.Millisecond)) {
		return nil
	}
	s.recordTradeOutcomeLocked(g, TradeOutcomeDeclined)
	s.startCurrentBankLocked(g) // resume offerer clock
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return err
	}
	s.broadcast(gameID, Event{Type: "state", Game: s.viewOf(ctx, g)})
	return nil
}

// ProposeTrade opens a trade from the current player to toUserID (Phase 13.2).
func (s *service) ProposeTrade(ctx context.Context, gameID, userID, toUserID string, give, take TradeSideInput) (*View, error) {
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
	if g.Auction != nil {
		return nil, ErrAuctionActive
	}
	if g.Trade != nil {
		return nil, ErrTradeActive
	}
	if currentPlayerInDebt(g) {
		return nil, ErrMustSettle
	}
	if _, err := requireCurrentPlayer(g, userID); err != nil {
		return nil, err
	}
	if toUserID == "" || toUserID == userID {
		return nil, ErrInvalidTrade
	}
	toIdx := playerIndexByID(g, toUserID)
	if toIdx < 0 || g.Players[toIdx].Resigned {
		return nil, ErrNotPlayer
	}
	if !tradeSideNonEmpty(give) || !tradeSideNonEmpty(take) {
		return nil, ErrInvalidTrade
	}
	if !tradeSideHasDeedOrCard(give) && !tradeSideHasDeedOrCard(take) {
		return nil, ErrTradeNeedsDeed
	}
	giveSide, err := normalizeTradeSide(give)
	if err != nil {
		return nil, err
	}
	takeSide, err := normalizeTradeSide(take)
	if err != nil {
		return nil, err
	}
	// Disjoint board indexes across sides.
	overlap := make(map[int]struct{}, len(giveSide.BoardIndexes))
	for _, bi := range giveSide.BoardIndexes {
		overlap[bi] = struct{}{}
	}
	for _, bi := range takeSide.BoardIndexes {
		if _, ok := overlap[bi]; ok {
			return nil, ErrInvalidTrade
		}
	}
	if err := validateTradeSideOwnership(g, userID, giveSide); err != nil {
		return nil, err
	}
	if err := validateTradeSideOwnership(g, toUserID, takeSide); err != nil {
		return nil, err
	}

	s.pauseCurrentBankLocked(g)
	s.cancelBankTimerLocked(g.ID)
	now := time.Now().UTC()
	g.LastTrade = nil
	g.Trade = &gamerepo.TradeOffer{
		FromUserID:    userID,
		ToUserID:      toUserID,
		Give:          giveSide,
		Take:          takeSide,
		ReplyDeadline: now.Add(gamerepo.TradeReplyTimeout),
	}
	s.armTradeTimerLocked(g)
	g.UpdatedAt = now
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// DeclineTrade declines (or proposer cannot — only target) the open offer (Phase 13.2).
func (s *service) DeclineTrade(ctx context.Context, gameID, userID string) (*View, error) {
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
	if g.Trade == nil {
		return nil, ErrNoTrade
	}
	if g.Trade.ToUserID != userID {
		return nil, ErrNotTradeTarget
	}
	s.recordTradeOutcomeLocked(g, TradeOutcomeDeclined)
	s.startCurrentBankLocked(g)
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// AcceptTrade completes the open offer (Phase 13.2).
// mortgageAction: redeem_all | leave_all — required when the acceptor receives any mortgaged deed.
func (s *service) AcceptTrade(ctx context.Context, gameID, userID, mortgageAction string) (*View, error) {
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
	if g.Auction != nil {
		return nil, ErrAuctionActive
	}
	if g.DebtPay != nil {
		return nil, ErrMustSettle
	}
	if g.Trade == nil {
		return nil, ErrNoTrade
	}
	t := g.Trade
	if t.ToUserID != userID {
		return nil, ErrNotTradeTarget
	}

	// Re-validate ownership/cash still hold.
	if err := validateTradeSideOwnership(g, t.FromUserID, t.Give); err != nil {
		return nil, err
	}
	if err := validateTradeSideOwnership(g, t.ToUserID, t.Take); err != nil {
		return nil, err
	}

	receivedMortgaged := false
	for _, bi := range t.Give.BoardIndexes {
		if d := deedOwnedBy(g, bi, t.FromUserID); d != nil && d.Mortgaged {
			receivedMortgaged = true
			break
		}
	}
	if !receivedMortgaged {
		for _, bi := range t.Take.BoardIndexes {
			if d := deedOwnedBy(g, bi, t.ToUserID); d != nil && d.Mortgaged {
				receivedMortgaged = true
				break
			}
		}
	}
	if receivedMortgaged {
		if mortgageAction != MortgageRedeemAll && mortgageAction != MortgageLeaveAll {
			return nil, ErrTradeMortgageChoice
		}
	}

	spaces := s.loadSpaces(ctx, g.WorldID)
	toRedeem := 0
	fromRedeem := 0
	if receivedMortgaged && mortgageAction == MortgageRedeemAll {
		for _, bi := range t.Give.BoardIndexes {
			d := deedOwnedBy(g, bi, t.FromUserID)
			if d == nil || !d.Mortgaged {
				continue
			}
			if sp := spaceByIndex(spaces, bi); sp != nil {
				toRedeem += redeemCost(sp.Price)
			}
		}
		for _, bi := range t.Take.BoardIndexes {
			d := deedOwnedBy(g, bi, t.ToUserID)
			if d == nil || !d.Mortgaged {
				continue
			}
			if sp := spaceByIndex(spaces, bi); sp != nil {
				fromRedeem += redeemCost(sp.Price)
			}
		}
	}

	fromIdx := playerIndexByID(g, t.FromUserID)
	toIdx := playerIndexByID(g, t.ToUserID)
	if fromIdx < 0 || toIdx < 0 {
		return nil, ErrNotPlayer
	}

	fromCash := g.Players[fromIdx].Cash - t.Give.Cash + t.Take.Cash - fromRedeem
	toCash := g.Players[toIdx].Cash + t.Give.Cash - t.Take.Cash - toRedeem
	if fromCash < 0 || toCash < 0 {
		return nil, ErrCannotAfford
	}
	g.Players[fromIdx].Cash = fromCash
	g.Players[toIdx].Cash = toCash

	transferGOOJF(g, fromIdx, toIdx, t.Give.GetOutOfJailFree)
	transferGOOJF(g, toIdx, fromIdx, t.Take.GetOutOfJailFree)

	for _, bi := range t.Give.BoardIndexes {
		transferDeedOwner(g, bi, t.ToUserID)
		if mortgageAction == MortgageRedeemAll {
			if d := deedOwnedBy(g, bi, t.ToUserID); d != nil {
				d.Mortgaged = false
			}
		}
	}
	for _, bi := range t.Take.BoardIndexes {
		transferDeedOwner(g, bi, t.FromUserID)
		if mortgageAction == MortgageRedeemAll {
			if d := deedOwnedBy(g, bi, t.FromUserID); d != nil {
				d.Mortgaged = false
			}
		}
	}

	s.recordTradeOutcomeLocked(g, TradeOutcomeAccepted)
	s.startCurrentBankLocked(g)
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func transferDeedOwner(g *gamerepo.Game, boardIndex int, newOwner string) {
	for i := range g.Deeds {
		if g.Deeds[i].BoardIndex == boardIndex {
			g.Deeds[i].OwnerUserID = newOwner
			return
		}
	}
}

func transferGOOJF(g *gamerepo.Game, fromIdx, toIdx, n int) {
	if n <= 0 || fromIdx < 0 || toIdx < 0 {
		return
	}
	from := &g.Players[fromIdx]
	to := &g.Players[toIdx]
	moved := 0
	remaining := make([]string, 0, len(from.GetOutOfJailFreeCards))
	for _, id := range from.GetOutOfJailFreeCards {
		if moved < n {
			to.GetOutOfJailFreeCards = append(to.GetOutOfJailFreeCards, id)
			moved++
			continue
		}
		remaining = append(remaining, id)
	}
	from.GetOutOfJailFreeCards = remaining
	from.GetOutOfJailFree = len(from.GetOutOfJailFreeCards)
	to.GetOutOfJailFree = len(to.GetOutOfJailFreeCards)
}

func spaceByIndex(spaces []Space, boardIndex int) *Space {
	for i := range spaces {
		if spaces[i].BoardIndex == boardIndex {
			return &spaces[i]
		}
	}
	return nil
}

func playerByID(g *gamerepo.Game, userID string) *gamerepo.Player {
	idx := playerIndexByID(g, userID)
	if idx < 0 {
		return nil
	}
	return &g.Players[idx]
}
