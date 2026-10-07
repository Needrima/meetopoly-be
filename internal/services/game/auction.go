package game

import (
	"context"
	"errors"
	"slices"
	"time"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	ErrNoBuyOffer     = errors.New("no buy offer to auction")
	ErrAuctionActive  = errors.New("auction already in progress")
	ErrNoAuction      = errors.New("no active auction")
	ErrNotAuctionTurn = errors.New("not your turn to bid")
	ErrBidTooLow      = errors.New("bid too low")
	ErrAlreadyFolded  = errors.New("already folded from auction")
	ErrMustResolveBuy = errors.New("must buy or start auction before ending turn")
)

// AuctionEventView is one line in the public auction feed.
type AuctionEventView struct {
	Kind     string `json:"kind"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Amount   int    `json:"amount,omitempty"`
}

// AuctionView is the public active auction snapshot (Phase 13.0).
type AuctionView struct {
	BoardIndex            int                `json:"boardIndex"`
	Slug                  string             `json:"slug"`
	Name                  string             `json:"name"`
	Kind                  string             `json:"kind"`
	ListPrice             int                `json:"listPrice"`
	HighBid               int                `json:"highBid"`
	HighBidderUserID      string             `json:"highBidderUserId,omitempty"`
	HighBidderUsername    string             `json:"highBidderUsername,omitempty"`
	CurrentBidderUserID   string             `json:"currentBidderUserId"`
	CurrentBidderUsername string             `json:"currentBidderUsername,omitempty"`
	MinBid                int                `json:"minBid"`
	BidDeadline           string             `json:"bidDeadline"`
	FoldedUserIDs         []string           `json:"foldedUserIds"`
	History               []AuctionEventView `json:"history"`
	StartedByUserID       string             `json:"startedByUserId"`
}

// LastAuctionView is the most recent settle/void for toasts (Phase 13.0).
type LastAuctionView struct {
	BoardIndex     int    `json:"boardIndex"`
	SpaceName      string `json:"spaceName"`
	WinnerUserID   string `json:"winnerUserId,omitempty"`
	WinnerUsername string `json:"winnerUsername,omitempty"`
	Amount         int    `json:"amount,omitempty"`
	Void           bool   `json:"void"`
}

func auctionMinNext(highBid int) int {
	if highBid <= 0 {
		return gamerepo.AuctionMinBid
	}
	return highBid + 1
}

func playerIndexByID(g *gamerepo.Game, userID string) int {
	for i := range g.Players {
		if g.Players[i].UserID == userID {
			return i
		}
	}
	return -1
}

func playerUsername(g *gamerepo.Game, userID string) string {
	if i := playerIndexByID(g, userID); i >= 0 {
		return g.Players[i].Username
	}
	return ""
}

func isFolded(a *gamerepo.Auction, userID string) bool {
	if a == nil {
		return false
	}
	return slices.Contains(a.FoldedUserIDs, userID)
}

func nonFoldedActive(g *gamerepo.Game, a *gamerepo.Auction) []string {
	out := make([]string, 0, len(g.Players))
	for _, p := range g.Players {
		if p.Resigned || isFolded(a, p.UserID) {
			continue
		}
		out = append(out, p.UserID)
	}
	return out
}

func appendAuctionEvent(a *gamerepo.Auction, kind, userID, username string, amount int) {
	a.History = append(a.History, gamerepo.AuctionEvent{
		Kind:     kind,
		UserID:   userID,
		Username: username,
		Amount:   amount,
	})
}

func revertHighBidAfterFold(g *gamerepo.Game, a *gamerepo.Auction) {
	a.HighBid = 0
	a.HighBidderUserID = ""
	for i := len(a.History) - 1; i >= 0; i-- {
		ev := a.History[i]
		if (ev.Kind == "bid" || ev.Kind == "auto_bid") && ev.Amount > 0 &&
			!isFolded(a, ev.UserID) && playerIndexByID(g, ev.UserID) >= 0 &&
			!g.Players[playerIndexByID(g, ev.UserID)].Resigned {
			a.HighBid = ev.Amount
			a.HighBidderUserID = ev.UserID
			return
		}
	}
}

// nextAuctionBidder returns the next non-folded active player after fromUserID,
// skipping the current high bidder (they sit out while holding high).
func nextAuctionBidder(g *gamerepo.Game, a *gamerepo.Auction, fromUserID string) string {
	order := make([]string, 0, len(g.Players))
	for _, p := range g.Players {
		order = append(order, p.UserID)
	}
	if len(order) == 0 {
		return ""
	}
	start := 0
	for i, id := range order {
		if id == fromUserID {
			start = i
			break
		}
	}
	for step := 1; step <= len(order); step++ {
		id := order[(start+step)%len(order)]
		idx := playerIndexByID(g, id)
		if idx < 0 || g.Players[idx].Resigned || isFolded(a, id) {
			continue
		}
		if a.HighBidderUserID != "" && id == a.HighBidderUserID {
			continue
		}
		return id
	}
	return ""
}

func auctionViewOf(g *gamerepo.Game) *AuctionView {
	a := g.Auction
	if a == nil {
		return nil
	}
	hist := make([]AuctionEventView, 0, len(a.History))
	for _, ev := range a.History {
		hist = append(hist, AuctionEventView{
			Kind:     ev.Kind,
			UserID:   ev.UserID,
			Username: ev.Username,
			Amount:   ev.Amount,
		})
	}
	folded := append([]string(nil), a.FoldedUserIDs...)
	if folded == nil {
		folded = []string{}
	}
	deadline := ""
	if !a.BidderTurnStartedAt.IsZero() {
		deadline = a.BidderTurnStartedAt.Add(gamerepo.AuctionBidTurn).UTC().Format(time.RFC3339Nano)
	}
	return &AuctionView{
		BoardIndex:            a.BoardIndex,
		Slug:                  a.Slug,
		Name:                  a.Name,
		Kind:                  a.Kind,
		ListPrice:             a.ListPrice,
		HighBid:               a.HighBid,
		HighBidderUserID:      a.HighBidderUserID,
		HighBidderUsername:    playerUsername(g, a.HighBidderUserID),
		CurrentBidderUserID:   a.CurrentBidderUserID,
		CurrentBidderUsername: playerUsername(g, a.CurrentBidderUserID),
		MinBid:                auctionMinNext(a.HighBid),
		BidDeadline:           deadline,
		FoldedUserIDs:         folded,
		History:               hist,
		StartedByUserID:       a.StartedByUserID,
	}
}

func lastAuctionViewOf(g *gamerepo.Game) *LastAuctionView {
	if g.LastAuction == nil {
		return nil
	}
	la := g.LastAuction
	return &LastAuctionView{
		BoardIndex:     la.BoardIndex,
		SpaceName:      la.SpaceName,
		WinnerUserID:   la.WinnerUserID,
		WinnerUsername: la.WinnerUsername,
		Amount:         la.Amount,
		Void:           la.Void,
	}
}

// beginAuctionLocked starts a bank auction for the given offer. Pauses personal time banks.
func (s *service) beginAuctionLocked(g *gamerepo.Game, offer *BuyOfferView, startedBy string) {
	s.pauseCurrentBankLocked(g)
	s.cancelBankTimerLocked(g.ID)

	now := time.Now().UTC()
	g.Auction = &gamerepo.Auction{
		BoardIndex:          offer.BoardIndex,
		Slug:                offer.Slug,
		Name:                offer.Name,
		Kind:                offer.Kind,
		ListPrice:           offer.Price,
		HighBid:             0,
		HighBidderUserID:    "",
		CurrentBidderUserID: startedBy,
		BidderTurnStartedAt: now,
		FoldedUserIDs:       nil,
		History:             nil,
		StartedByUserID:     startedBy,
	}
	g.LastAuction = nil
	s.prepareAuctionTurnLocked(g)
}

func (s *service) prepareAuctionTurnLocked(g *gamerepo.Game) {
	a := g.Auction
	if a == nil {
		return
	}
	for {
		remaining := nonFoldedActive(g, a)
		if len(remaining) == 0 {
			s.settleAuctionVoidLocked(g)
			return
		}
		if len(remaining) == 1 {
			s.settleAuctionWinnerLocked(g, remaining[0])
			return
		}
		if a.CurrentBidderUserID == "" {
			a.CurrentBidderUserID = nextAuctionBidder(g, a, a.StartedByUserID)
			if a.CurrentBidderUserID == "" {
				// Only high bidder left with others folded — settle them.
				if a.HighBidderUserID != "" && !isFolded(a, a.HighBidderUserID) {
					s.settleAuctionWinnerLocked(g, a.HighBidderUserID)
					return
				}
				s.settleAuctionVoidLocked(g)
				return
			}
		}
		idx := playerIndexByID(g, a.CurrentBidderUserID)
		if idx < 0 || g.Players[idx].Resigned || isFolded(a, a.CurrentBidderUserID) {
			from := a.CurrentBidderUserID
			a.CurrentBidderUserID = nextAuctionBidder(g, a, from)
			if a.CurrentBidderUserID == "" {
				if a.HighBidderUserID != "" && !isFolded(a, a.HighBidderUserID) {
					s.settleAuctionWinnerLocked(g, a.HighBidderUserID)
					return
				}
				s.settleAuctionVoidLocked(g)
				return
			}
			continue
		}
		// High bidder never acts while holding high.
		if a.HighBidderUserID != "" && a.CurrentBidderUserID == a.HighBidderUserID {
			from := a.CurrentBidderUserID
			a.CurrentBidderUserID = nextAuctionBidder(g, a, from)
			if a.CurrentBidderUserID == "" {
				s.settleAuctionWinnerLocked(g, a.HighBidderUserID)
				return
			}
			continue
		}
		minBid := auctionMinNext(a.HighBid)
		if g.Players[idx].Cash < minBid {
			appendAuctionEvent(a, "auto_fold", a.CurrentBidderUserID, g.Players[idx].Username, 0)
			a.FoldedUserIDs = append(a.FoldedUserIDs, a.CurrentBidderUserID)
			if a.HighBidderUserID == a.CurrentBidderUserID {
				revertHighBidAfterFold(g, a)
			}
			from := a.CurrentBidderUserID
			a.CurrentBidderUserID = nextAuctionBidder(g, a, from)
			continue
		}
		a.BidderTurnStartedAt = time.Now().UTC()
		s.armAuctionTimerLocked(g)
		return
	}
}

func (s *service) settleAuctionWinnerLocked(g *gamerepo.Game, winnerID string) {
	a := g.Auction
	if a == nil {
		return
	}
	s.cancelAuctionTimerLocked(g.ID)

	amount := a.HighBid
	wIdx := playerIndexByID(g, winnerID)
	if wIdx < 0 || g.Players[wIdx].Resigned {
		s.settleAuctionVoidLocked(g)
		return
	}
	if amount <= 0 {
		amount = gamerepo.AuctionMinBid
	}
	if g.Players[wIdx].Cash < amount {
		s.settleAuctionVoidLocked(g)
		return
	}

	g.Players[wIdx].Cash -= amount
	g.Deeds = append(g.Deeds, gamerepo.Deed{
		BoardIndex:  a.BoardIndex,
		OwnerUserID: winnerID,
		Houses:      0,
		Mortgaged:   false,
	})
	g.LastAuction = &gamerepo.LastAuction{
		BoardIndex:     a.BoardIndex,
		SpaceName:      a.Name,
		WinnerUserID:   winnerID,
		WinnerUsername: g.Players[wIdx].Username,
		Amount:         amount,
		Void:           false,
	}
	g.Auction = nil
	g.SuppressBuyOffer = true
	s.resumeBanksAfterAuctionLocked(g)
}

func (s *service) settleAuctionVoidLocked(g *gamerepo.Game) {
	a := g.Auction
	if a == nil {
		return
	}
	s.cancelAuctionTimerLocked(g.ID)
	g.LastAuction = &gamerepo.LastAuction{
		BoardIndex: a.BoardIndex,
		SpaceName:  a.Name,
		Void:       true,
	}
	g.Auction = nil
	g.SuppressBuyOffer = true
	s.resumeBanksAfterAuctionLocked(g)
}

func (s *service) resumeBanksAfterAuctionLocked(g *gamerepo.Game) {
	if g.Status != gamerepo.StatusActive {
		return
	}
	// Lander (current turn) is still awaiting_end — restart their personal bank.
	s.startCurrentBankLocked(g)
}

func (s *service) maybeAutoStartAuctionLocked(g *gamerepo.Game, spaces []Space, payerIdx int) {
	if g == nil || g.Auction != nil || currentPlayerInDebt(g) {
		return
	}
	if payerIdx < 0 || payerIdx >= len(g.Players) || g.Players[payerIdx].Resigned {
		return
	}
	offer := openBuyOffer(g, spaces)
	if offer == nil {
		return
	}
	if g.Players[payerIdx].Cash >= offer.Price {
		return
	}
	s.beginAuctionLocked(g, offer, g.Players[payerIdx].UserID)
}

func (s *service) applyAuctionBidLocked(g *gamerepo.Game, userID string, amount int, auto bool) error {
	a := g.Auction
	if a == nil {
		return ErrNoAuction
	}
	if a.CurrentBidderUserID != userID {
		return ErrNotAuctionTurn
	}
	if isFolded(a, userID) {
		return ErrAlreadyFolded
	}
	idx := playerIndexByID(g, userID)
	if idx < 0 || g.Players[idx].Resigned {
		return ErrNotPlayer
	}
	minBid := auctionMinNext(a.HighBid)
	if amount < minBid {
		return ErrBidTooLow
	}
	if amount > g.Players[idx].Cash {
		return ErrCannotAfford
	}
	kind := "bid"
	if auto {
		kind = "auto_bid"
	}
	appendAuctionEvent(a, kind, userID, g.Players[idx].Username, amount)
	a.HighBid = amount
	a.HighBidderUserID = userID
	a.CurrentBidderUserID = nextAuctionBidder(g, a, userID)
	s.prepareAuctionTurnLocked(g)
	return nil
}

func (s *service) applyAuctionFoldLocked(g *gamerepo.Game, userID string, auto bool) error {
	a := g.Auction
	if a == nil {
		return ErrNoAuction
	}
	if a.CurrentBidderUserID != userID {
		return ErrNotAuctionTurn
	}
	if isFolded(a, userID) {
		return ErrAlreadyFolded
	}
	idx := playerIndexByID(g, userID)
	if idx < 0 {
		return ErrNotPlayer
	}
	kind := "fold"
	if auto {
		kind = "auto_fold"
	}
	appendAuctionEvent(a, kind, userID, g.Players[idx].Username, 0)
	a.FoldedUserIDs = append(a.FoldedUserIDs, userID)
	if a.HighBidderUserID == userID {
		revertHighBidAfterFold(g, a)
	}
	a.CurrentBidderUserID = nextAuctionBidder(g, a, userID)
	s.prepareAuctionTurnLocked(g)
	return nil
}

// foldPlayerFromAuctionLocked removes a resigning/disconnected player from the auction.
func (s *service) foldPlayerFromAuctionLocked(g *gamerepo.Game, userID string) {
	a := g.Auction
	if a == nil || isFolded(a, userID) {
		return
	}
	idx := playerIndexByID(g, userID)
	name := userID
	if idx >= 0 {
		name = g.Players[idx].Username
	}
	appendAuctionEvent(a, "fold", userID, name, 0)
	a.FoldedUserIDs = append(a.FoldedUserIDs, userID)
	if a.HighBidderUserID == userID {
		revertHighBidAfterFold(g, a)
	}
	if a.CurrentBidderUserID == userID {
		a.CurrentBidderUserID = nextAuctionBidder(g, a, userID)
	}
	s.prepareAuctionTurnLocked(g)
}

func (s *service) armAuctionTimerLocked(g *gamerepo.Game) {
	s.cancelAuctionTimerLocked(g.ID)
	if g.Auction == nil || g.Auction.BidderTurnStartedAt.IsZero() {
		return
	}
	deadline := g.Auction.BidderTurnStartedAt.Add(gamerepo.AuctionBidTurn)
	delay := max(time.Until(deadline), 0)
	gameID := g.ID
	s.auctionTimers[gameID] = time.AfterFunc(delay, func() {
		_ = s.onAuctionBidTimeout(context.Background(), gameID)
	})
}

func (s *service) cancelAuctionTimerLocked(gameID string) {
	if t, ok := s.auctionTimers[gameID]; ok {
		t.Stop()
		delete(s.auctionTimers, gameID)
	}
}

func (s *service) onAuctionBidTimeout(ctx context.Context, gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil {
		return err
	}
	a := g.Auction
	if a == nil {
		return nil
	}
	// Stale timer for a previous turn.
	if a.BidderTurnStartedAt.IsZero() {
		return nil
	}
	if time.Since(a.BidderTurnStartedAt) < gamerepo.AuctionBidTurn-50*time.Millisecond {
		return nil
	}
	userID := a.CurrentBidderUserID
	minBid := auctionMinNext(a.HighBid)
	idx := playerIndexByID(g, userID)
	if idx >= 0 && g.Players[idx].Cash >= minBid {
		if err := s.applyAuctionBidLocked(g, userID, minBid, true); err != nil {
			_ = s.applyAuctionFoldLocked(g, userID, true)
		}
	} else {
		_ = s.applyAuctionFoldLocked(g, userID, true)
	}
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return err
	}
	s.broadcast(gameID, Event{Type: "state", Game: s.viewOf(ctx, g)})
	return nil
}
