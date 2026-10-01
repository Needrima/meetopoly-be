package game

import (
	"context"
	"errors"
	"time"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	// ErrNotInDebt — bankrupt / debt-pay requires negative cash.
	ErrNotInDebt = errors.New("not in debt")
	// ErrDebtPayActive — debt-pay window already open.
	ErrDebtPayActive = errors.New("debt pay already active")
	// ErrNoDebtPay — no active debt-pay window.
	ErrNoDebtPay = errors.New("no debt pay active")
)

// DebtPayView is the active raise-funds window (Phase 14.0).
type DebtPayView struct {
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Deadline  string `json:"deadline"` // RFC3339 UTC
	Remaining int64  `json:"remainingMs"`
}

// LastBankruptcyView for client toasts (Phase 14.0).
type LastBankruptcyView struct {
	UserID         string `json:"userId"`
	Username       string `json:"username"`
	Reason         string `json:"reason"`
	OwedToUserID   string `json:"owedToUserId,omitempty"`
	OwedToUsername string `json:"owedToUsername,omitempty"`
	BankPaid       int    `json:"bankPaid,omitempty"`
}

// Bankrupt declares bankruptcy for the caller (Phase 14.0). Requires cash < 0.
func (s *service) Bankrupt(ctx context.Context, gameID, userID string) (*View, error) {
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

	playerIdx := playerIndexByID(g, userID)
	if playerIdx < 0 {
		return nil, ErrNotPlayer
	}
	if g.Players[playerIdx].Resigned {
		return nil, ErrAlreadyOut
	}
	if g.Players[playerIdx].Cash >= 0 {
		return nil, ErrNotInDebt
	}

	s.eliminatePlayerLocked(g, playerIdx, "declare")
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

// StartDebtPay opens the 2-minute raise-funds window (Phase 14.0). Current player, cash < 0.
func (s *service) StartDebtPay(ctx context.Context, gameID, userID string) (*View, error) {
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
	if g.Players[playerIdx].Cash >= 0 {
		return nil, ErrNotInDebt
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
		return nil, ErrMustRoll
	}
	if g.DebtPay != nil {
		return nil, ErrDebtPayActive
	}
	if g.Auction != nil {
		return nil, ErrAuctionActive
	}
	if g.Trade != nil {
		return nil, ErrTradeActive
	}
	if !canRaiseFunds(g, playerIdx) {
		s.eliminatePlayerLocked(g, playerIdx, "auto_insolvent")
		g.UpdatedAt = time.Now().UTC()
		if err := s.repo.Update(ctx, g); err != nil {
			return nil, err
		}
		view := s.viewOf(ctx, g)
		s.broadcast(gameID, Event{Type: "state", Game: view})
		return view, nil
	}

	s.startDebtPayLocked(g, playerIdx)
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, g)
	s.broadcast(gameID, Event{Type: "state", Game: view})
	return view, nil
}

func (s *service) startDebtPayLocked(g *gamerepo.Game, playerIdx int) {
	if g == nil || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	s.pauseCurrentBankLocked(g)
	s.cancelBankTimerLocked(g.ID)
	now := time.Now().UTC()
	g.DebtPay = &gamerepo.DebtPay{
		UserID:   g.Players[playerIdx].UserID,
		Deadline: now.Add(gamerepo.DebtPayDuration),
	}
	s.armDebtPayTimerLocked(g)
}

func (s *service) clearDebtPayLocked(g *gamerepo.Game) {
	if g == nil {
		return
	}
	s.cancelDebtPayTimerLocked(g.ID)
	g.DebtPay = nil
}

func (s *service) armDebtPayTimerLocked(g *gamerepo.Game) {
	if g == nil || g.DebtPay == nil {
		return
	}
	s.cancelDebtPayTimerLocked(g.ID)
	delay := time.Until(g.DebtPay.Deadline)
	if delay < 0 {
		delay = 0
	}
	gameID := g.ID
	s.debtPayTimers[gameID] = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.onDebtPayTimeoutLocked(context.Background(), gameID)
	})
}

func (s *service) cancelDebtPayTimerLocked(gameID string) {
	if t, ok := s.debtPayTimers[gameID]; ok {
		t.Stop()
		delete(s.debtPayTimers, gameID)
	}
}

func (s *service) onDebtPayTimeoutLocked(ctx context.Context, gameID string) {
	delete(s.debtPayTimers, gameID)
	g, err := s.repo.FindByID(ctx, gameID)
	if err != nil || g == nil || g.Status != gamerepo.StatusActive || g.DebtPay == nil {
		return
	}
	idx := playerIndexByID(g, g.DebtPay.UserID)
	if idx < 0 || g.Players[idx].Resigned || g.Players[idx].Cash >= 0 {
		s.clearDebtPayLocked(g)
		_ = s.repo.Update(ctx, g)
		return
	}
	s.eliminatePlayerLocked(g, idx, "auto_timeout")
	g.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, g); err != nil {
		return
	}
	s.broadcast(gameID, Event{Type: "state", Game: s.viewOf(ctx, g)})
}

// maybeHandleDebtOnTurnStartLocked runs when a player becomes current (Phase 14.0).
// Insolvent with nothing to raise → auto-bankrupt. Otherwise pause turn clock until Pay/Bankrupt.
func (s *service) maybeHandleDebtOnTurnStartLocked(g *gamerepo.Game) {
	idx := currentPlayerIndex(g)
	if idx < 0 || g.Players[idx].Resigned {
		return
	}
	if g.Players[idx].Cash >= 0 {
		return
	}
	if !canRaiseFunds(g, idx) {
		s.eliminatePlayerLocked(g, idx, "auto_insolvent")
		return
	}
	// Pause turn clock while deciding Pay | Bankruptcy (14.2 modal).
	s.pauseCurrentBankLocked(g)
}

// afterRaiseCheckDebtLocked clears debt-pay when settled; auto-bankrupts if still broke with no assets.
// Also resumes the turn clock if debt was cleared without an open debt-pay window
// (turn-start pause from maybeHandleDebtOnTurnStartLocked).
func (s *service) afterRaiseCheckDebtLocked(g *gamerepo.Game, playerIdx int) {
	if playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	if g.Players[playerIdx].Cash >= 0 {
		hadDebtPay := g.DebtPay != nil && g.DebtPay.UserID == g.Players[playerIdx].UserID
		if hadDebtPay {
			s.clearDebtPayLocked(g)
		}
		if currentPlayerIndex(g) == playerIdx && g.Auction == nil && g.Trade == nil {
			// Resume after debt-pay settle, or after raise that cleared debt while
			// the turn clock was paused at the Pay|Bankruptcy gate (no DebtPay yet).
			if hadDebtPay || g.TurnStartedAt.IsZero() {
				s.startCurrentBankLocked(g)
			}
		}
		return
	}
	if !canRaiseFunds(g, playerIdx) {
		s.eliminatePlayerLocked(g, playerIdx, "auto_insolvent")
	}
}

// canRaiseFunds — has buildings to sell or unmortgaged deeds (Phase 14.0). Trade excluded.
func canRaiseFunds(g *gamerepo.Game, playerIdx int) bool {
	if g == nil || playerIdx < 0 || playerIdx >= len(g.Players) {
		return false
	}
	uid := g.Players[playerIdx].UserID
	for _, d := range g.Deeds {
		if d.OwnerUserID != uid {
			continue
		}
		if d.Houses > 0 {
			return true
		}
		if !d.Mortgaged {
			return true
		}
	}
	return false
}

func currentPlayerInDebt(g *gamerepo.Game) bool {
	idx := currentPlayerIndex(g)
	return idx >= 0 && g.Players[idx].Cash < 0
}

func playerInDebt(g *gamerepo.Game, playerIdx int) bool {
	return playerIdx >= 0 && playerIdx < len(g.Players) && g.Players[playerIdx].Cash < 0
}

// applyPaymentShortfallLocked charges full `amount` (cash may go negative) and credits `pay` to creditor.
// `pay` should be min(availablePositiveCash, amount) before the charge.
func applyPaymentShortfallLocked(
	g *gamerepo.Game, payerIdx, amount, pay int, toUserID, kind, spaceName string, boardIndex int,
) {
	if g == nil || payerIdx < 0 || payerIdx >= len(g.Players) || amount <= 0 {
		return
	}
	payer := &g.Players[payerIdx]
	if pay < 0 {
		pay = 0
	}
	if pay > amount {
		pay = amount
	}
	available := payer.Cash
	if available < 0 {
		available = 0
	}
	if pay > available {
		pay = available
	}

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

	payer.Cash -= amount
	paidInFull := payer.Cash >= 0
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         kind,
		FromUserID:   payer.UserID,
		FromUsername: payer.Username,
		ToUserID:     toUserID,
		ToUsername:   toUsername,
		Amount:       pay,
		BoardIndex:   boardIndex,
		SpaceName:    spaceName,
		PaidInFull:   paidInFull,
	}
	if paidInFull {
		g.PendingPayment = nil
		return
	}
	remaining := -payer.Cash
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind:       kind,
		Amount:     remaining,
		FromUserID: payer.UserID,
		ToUserID:   toUserID,
		BoardIndex: boardIndex,
		SpaceName:  spaceName,
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
}

// applyRaiseTowardDebtLocked applies raised MeetCoin toward negative-cash debt (Phase 14.0).
// Creditor receives the debt portion; surplus stays with the raiser.
func applyRaiseTowardDebtLocked(g *gamerepo.Game, payerIdx, raised int) {
	if g == nil || payerIdx < 0 || payerIdx >= len(g.Players) || raised <= 0 {
		return
	}
	p := &g.Players[payerIdx]
	if p.Cash >= 0 {
		p.Cash += raised
		return
	}

	debt := -p.Cash
	pay := raised
	if pay > debt {
		pay = debt
	}
	toUserID := ""
	kind := "rent"
	boardIndex := p.BoardIndex
	spaceName := "Debt"
	if g.PendingPayment != nil && (g.PendingPayment.FromUserID == "" || g.PendingPayment.FromUserID == p.UserID) {
		toUserID = g.PendingPayment.ToUserID
		kind = g.PendingPayment.Kind
		boardIndex = g.PendingPayment.BoardIndex
		spaceName = g.PendingPayment.SpaceName
	}
	toUsername := "Bank"
	if toUserID != "" {
		for i := range g.Players {
			if g.Players[i].UserID == toUserID && !g.Players[i].Resigned {
				g.Players[i].Cash += pay
				toUsername = g.Players[i].Username
				break
			}
		}
	}

	p.Cash += raised
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         kind,
		FromUserID:   p.UserID,
		FromUsername: p.Username,
		ToUserID:     toUserID,
		ToUsername:   toUsername,
		Amount:       pay,
		BoardIndex:   boardIndex,
		SpaceName:    spaceName,
		PaidInFull:   p.Cash >= 0,
	}
	if p.Cash >= 0 {
		g.PendingPayment = nil
		return
	}
	if g.PendingPayment == nil {
		g.PendingPayment = &gamerepo.PendingPayment{
			Kind:       kind,
			FromUserID: p.UserID,
			ToUserID:   toUserID,
			BoardIndex: boardIndex,
			SpaceName:  spaceName,
		}
	}
	g.PendingPayment.Amount = -p.Cash
	g.PendingPayment.FromUserID = p.UserID
}

// wipePlayerAssetsToBankLocked clears houses/deeds/GOOJF to Bank/decks and Bank-pays remaining debt.
// Returned board indexes were owned by the wiped player (now unowned — no immediate re-auction).
func wipePlayerAssetsToBankLocked(g *gamerepo.Game, playerIdx int) (owedToUserID string, bankPaid int, wipedBoardIndexes []int) {
	if g == nil || playerIdx < 0 || playerIdx >= len(g.Players) {
		return "", 0, nil
	}
	p := &g.Players[playerIdx]

	if p.Cash < 0 {
		bankPaid = -p.Cash
		if g.PendingPayment != nil && (g.PendingPayment.FromUserID == p.UserID || g.PendingPayment.FromUserID == "") {
			owedToUserID = g.PendingPayment.ToUserID
		}
		if owedToUserID != "" {
			for i := range g.Players {
				if g.Players[i].UserID == owedToUserID && !g.Players[i].Resigned {
					g.Players[i].Cash += bankPaid
					break
				}
			}
		}
		p.Cash = 0
	}

	if g.PendingPayment != nil && (g.PendingPayment.FromUserID == p.UserID || g.PendingPayment.FromUserID == "") {
		g.PendingPayment = nil
	}

	for _, id := range p.GetOutOfJailFreeCards {
		returnJailCardToDeck(g, id)
	}
	p.GetOutOfJailFreeCards = nil
	p.GetOutOfJailFree = 0

	kept := make([]gamerepo.Deed, 0, len(g.Deeds))
	for _, d := range g.Deeds {
		if d.OwnerUserID == p.UserID {
			wipedBoardIndexes = append(wipedBoardIndexes, d.BoardIndex)
			continue
		}
		kept = append(kept, d)
	}
	g.Deeds = kept
	return owedToUserID, bankPaid, wipedBoardIndexes
}

// suppressBuyIfSittingOnWipedLocked blocks Buy|Auction when wipe makes the
// current player's occupied tile unowned under their feet (they already landed
// while it was owned — e.g. paid rent). They End/Roll away; a future landing
// can offer Buy|Auction.
func suppressBuyIfSittingOnWipedLocked(g *gamerepo.Game, wiped []int) {
	if g == nil || len(wiped) == 0 {
		return
	}
	idx := currentPlayerIndex(g)
	if idx < 0 || g.Players[idx].Resigned {
		return
	}
	if g.LastRoll == nil || g.LastRoll.UserID != g.Players[idx].UserID {
		return
	}
	bi := g.Players[idx].BoardIndex
	if g.LastRoll.ToIndex != bi {
		return
	}
	for _, w := range wiped {
		if w == bi {
			g.SuppressBuyOffer = true
			return
		}
	}
}

// eliminatePlayerLocked marks resigned, wipes assets to Bank, advances turn if needed (Phase 14.0).
func (s *service) eliminatePlayerLocked(g *gamerepo.Game, playerIdx int, reason string) {
	if g == nil || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	if g.Players[playerIdx].Resigned {
		return
	}
	userID := g.Players[playerIdx].UserID
	wasCurrent := g.Players[playerIdx].TurnOrder == g.CurrentTurn

	if g.Auction != nil {
		s.foldPlayerFromAuctionLocked(g, userID)
	}
	if g.Trade != nil && (g.Trade.FromUserID == userID || g.Trade.ToUserID == userID) {
		s.clearTradeLocked(g)
	}
	if g.DebtPay != nil && g.DebtPay.UserID == userID {
		s.clearDebtPayLocked(g)
	}
	if wasCurrent {
		s.pauseCurrentBankLocked(g)
	}

	owedTo, bankPaid, wiped := wipePlayerAssetsToBankLocked(g, playerIdx)
	g.Players[playerIdx].Resigned = true
	g.Players[playerIdx].HubID = ""
	g.Players[playerIdx].HubRevision++
	g.Players[playerIdx].Cash = 0
	g.Players[playerIdx].InJail = false
	g.Players[playerIdx].JailTurns = 0

	g.LastBankruptcy = &gamerepo.LastBankruptcy{
		UserID:       userID,
		Username:     g.Players[playerIdx].Username,
		Reason:       reason,
		OwedToUserID: owedTo,
		BankPaid:     bankPaid,
	}
	if reason == "resign" || reason == "turn_timeout" {
		g.LastForfeit = &gamerepo.LastForfeit{
			UserID:   userID,
			Username: g.Players[playerIdx].Username,
			Reason:   reason,
			Strikes:  g.Players[playerIdx].TurnTimeouts,
		}
	}

	if finishIfOneActive(g) {
		s.clearBankClockLocked(g)
		s.cancelAuctionTimerLocked(g.ID)
		s.cancelTradeTimerLocked(g.ID)
		s.clearDebtPayLocked(g)
		g.Auction = nil
		g.Trade = nil
		return
	}

	// Phase 14.3 cancelled: no Bank re-auction queue. Deeds stay unowned until
	// a future landing. If someone is already sitting on a wiped tile (paid rent
	// this turn), suppress Buy|Auction so they can End / Roll away.
	suppressBuyIfSittingOnWipedLocked(g, wiped)

	if wasCurrent {
		advanceToNextActive(g)
		g.DoublesStreak = 0
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		g.SuppressBuyOffer = false
		// Re-apply after advance: next player may also be sitting on wiped land
		// with a stale matching lastRoll (rare); suppress if needed.
		suppressBuyIfSittingOnWipedLocked(g, wiped)
		if g.Auction == nil {
			s.startFreshTurnClockLocked(g)
			s.maybeHandleDebtOnTurnStartLocked(g)
		}
	}
}
