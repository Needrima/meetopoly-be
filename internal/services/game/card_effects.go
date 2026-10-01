package game

import (
	"context"

	gamerepo "meetopoly-be/internal/repository/game"
)

var (
	railroadIndices = []int{5, 15, 25, 35}
	utilityIndices  = []int{12, 28}
)

type landingOpts struct {
	// rentMultiplier — Chance "nearest railroad" pays double rent (2). 0/1 = normal.
	rentMultiplier int
	// utilityDiceTotal — if > 0 and landing on owned utility, rent = 10 × this (Chance nearest utility).
	utilityDiceTotal int
}

// applyCardEffectLocked resolves a drawn Chance/Chest card (Phase 12.3).
// GOOJF is already held on draw (12.2). Server applies immediately (lock A).
func (s *service) applyCardEffectLocked(ctx context.Context, g *gamerepo.Game, playerIdx int, cardID string) {
	if playerIdx < 0 || playerIdx >= len(g.Players) || cardID == "" {
		return
	}
	spaces := s.loadSpaces(ctx, g.WorldID)

	switch cardID {
	case CardChanceGetOutOfJail, CardChestGetOutOfJail:
		return // held on draw

	case CardChanceAdvanceBoardwalk:
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, 39, landingOpts{})
	case CardChanceAdvanceGO, CardChestAdvanceGO:
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, gamerepo.GoBoardIndex, landingOpts{})
	case CardChanceAdvanceIllinois:
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, 24, landingOpts{})
	case CardChanceAdvanceStCharles:
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, 11, landingOpts{})
	case CardChanceReadingRailroad:
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, 15, landingOpts{})
	case CardChanceNearestRailroad:
		dest, _ := nextIndexForward(g.Players[playerIdx].BoardIndex, railroadIndices)
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, dest, landingOpts{rentMultiplier: 2})
	case CardChanceNearestUtility:
		dest, _ := nextIndexForward(g.Players[playerIdx].BoardIndex, utilityIndices)
		d1, d2 := rollDie(), rollDie()
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, dest, landingOpts{utilityDiceTotal: d1 + d2})
	case CardChanceGoBack3:
		s.cardGoBackLocked(ctx, g, playerIdx, spaces, 3)

	case CardChanceGoToJail, CardChestGoToJail:
		jail := sendPlayerToJail(g, playerIdx, spaces)
		if g.LastRoll != nil {
			g.LastRoll.ToIndex = jail
			g.LastRoll.PassedGo = false
			g.LastRoll.PassGoAmount = 0
		}

	case CardChanceDividend:
		creditFromBank(g, playerIdx, 50, "card")
		setLastCardCashDelta(g, 50)
	case CardChanceSpeedingFine:
		debitToBank(g, playerIdx, 15, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -15)
	case CardChanceBuildingLoan:
		creditFromBank(g, playerIdx, 150, "card")
		setLastCardCashDelta(g, 150)
	case CardChanceChairman:
		others := countOtherActivePlayers(g, playerIdx)
		payEachOtherPlayer(g, playerIdx, 50)
		setLastCardCashDelta(g, -(others * 50))
	case CardChanceGeneralRepairs:
		due := repairsDue(g, playerIdx, 25, 100)
		debitToBank(g, playerIdx, due, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -due)

	case CardChestBankError:
		creditFromBank(g, playerIdx, 200, "card")
		setLastCardCashDelta(g, 200)
	case CardChestDoctorsFee:
		debitToBank(g, playerIdx, 50, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -50)
	case CardChestSaleOfStock:
		creditFromBank(g, playerIdx, 50, "card")
		setLastCardCashDelta(g, 50)
	case CardChestHolidayFund:
		creditFromBank(g, playerIdx, 100, "card")
		setLastCardCashDelta(g, 100)
	case CardChestIncomeTaxRefund:
		creditFromBank(g, playerIdx, 20, "card")
		setLastCardCashDelta(g, 20)
	case CardChestBirthday:
		others := countOtherActivePlayers(g, playerIdx)
		collectFromEachOtherPlayer(g, playerIdx, 10)
		setLastCardCashDelta(g, others*10)
	case CardChestLifeInsurance:
		creditFromBank(g, playerIdx, 100, "card")
		setLastCardCashDelta(g, 100)
	case CardChestHospitalFees:
		debitToBank(g, playerIdx, 100, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -100)
	case CardChestSchoolFees:
		debitToBank(g, playerIdx, 50, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -50)
	case CardChestConsultancyFee:
		creditFromBank(g, playerIdx, 25, "card")
		setLastCardCashDelta(g, 25)
	case CardChestStreetRepairs:
		due := repairsDue(g, playerIdx, 40, 115)
		debitToBank(g, playerIdx, due, "card", cardTitle(cardID))
		setLastCardCashDelta(g, -due)
	case CardChestBeautyContest:
		creditFromBank(g, playerIdx, 10, "card")
		setLastCardCashDelta(g, 10)
	case CardChestInheritance:
		creditFromBank(g, playerIdx, 100, "card")
		setLastCardCashDelta(g, 100)
	}
}

func setLastCardCashDelta(g *gamerepo.Game, delta int) {
	if g == nil || g.LastCard == nil || delta == 0 {
		return
	}
	g.LastCard.CashDelta = delta
}

func countOtherActivePlayers(g *gamerepo.Game, playerIdx int) int {
	if g == nil {
		return 0
	}
	n := 0
	for i := range g.Players {
		if i != playerIdx && !g.Players[i].Resigned {
			n++
		}
	}
	return n
}

// nextIndexForward finds the next board index in targets strictly ahead (wrapping past GO).
func nextIndexForward(from int, targets []int) (dest int, passedGO bool) {
	if len(targets) == 0 {
		return from, false
	}
	best := -1
	for _, t := range targets {
		if t > from && (best < 0 || t < best) {
			best = t
		}
	}
	if best >= 0 {
		return best, false
	}
	// Wrap to smallest target.
	best = targets[0]
	for _, t := range targets[1:] {
		if t < best {
			best = t
		}
	}
	return best, from != best
}

// cardAdvanceToLocked moves forward to dest (collect Pass GO when wrapping / landing GO).
func (s *service) cardAdvanceToLocked(
	ctx context.Context, g *gamerepo.Game, playerIdx int, spaces []Space, dest int, opts landingOpts,
) {
	from := g.Players[playerIdx].BoardIndex
	if dest < 0 {
		dest = 0
	}
	dest = dest % gamerepo.BoardSpaceCount

	passedGo := false
	passAmt := 0
	if dest == gamerepo.GoBoardIndex {
		// Advance to GO always collects salary.
		passedGo = true
	} else if dest < from {
		passedGo = true
	}
	if passedGo {
		passAmt = g.PassGoBonus
		if passAmt <= 0 {
			passAmt = gamerepo.PassGoBonus
		}
		g.Players[playerIdx].Cash += passAmt
	}
	g.Players[playerIdx].BoardIndex = dest
	if g.LastRoll != nil {
		g.LastRoll.ToIndex = dest
		if passedGo {
			g.LastRoll.PassedGo = true
			g.LastRoll.PassGoAmount += passAmt
		}
	}

	dice := 0
	if g.LastRoll != nil {
		dice = g.LastRoll.Total
	}
	if opts.utilityDiceTotal > 0 {
		dice = opts.utilityDiceTotal
	}
	s.resolveLandingWithOpts(ctx, g, playerIdx, dice, opts)
}

func (s *service) cardGoBackLocked(ctx context.Context, g *gamerepo.Game, playerIdx int, spaces []Space, steps int) {
	from := g.Players[playerIdx].BoardIndex
	to := (from - steps) % gamerepo.BoardSpaceCount
	if to < 0 {
		to += gamerepo.BoardSpaceCount
	}
	g.Players[playerIdx].BoardIndex = to
	if g.LastRoll != nil {
		g.LastRoll.ToIndex = to
	}
	dice := 0
	if g.LastRoll != nil {
		dice = g.LastRoll.Total
	}
	s.resolveLandingWithOpts(ctx, g, playerIdx, dice, landingOpts{})
}

func repairsDue(g *gamerepo.Game, playerIdx int, perHouse, perHotel int) int {
	if playerIdx < 0 || playerIdx >= len(g.Players) {
		return 0
	}
	uid := g.Players[playerIdx].UserID
	total := 0
	for _, d := range g.Deeds {
		if d.OwnerUserID != uid {
			continue
		}
		h := d.Houses
		if h < 0 {
			h = 0
		}
		if h >= 5 {
			total += perHotel
		} else {
			total += h * perHouse
		}
	}
	return total
}

func creditFromBank(g *gamerepo.Game, playerIdx, amount int, kind string) {
	if amount <= 0 || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	g.Players[playerIdx].Cash += amount
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         kind,
		FromUserID:   "",
		FromUsername: "Bank",
		ToUserID:     g.Players[playerIdx].UserID,
		ToUsername:   g.Players[playerIdx].Username,
		Amount:       amount,
		BoardIndex:   g.Players[playerIdx].BoardIndex,
		SpaceName:    "Card",
		PaidInFull:   true,
	}
}

func debitToBank(g *gamerepo.Game, playerIdx, amount int, kind, spaceName string) {
	if amount <= 0 || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	pay := amount
	if g.Players[playerIdx].Cash < amount {
		if g.Players[playerIdx].Cash > 0 {
			pay = g.Players[playerIdx].Cash
		} else {
			pay = 0
		}
	}
	applyPaymentShortfallLocked(
		g, playerIdx, amount, pay, "", kind, spaceName, g.Players[playerIdx].BoardIndex,
	)
}

func payEachOtherPlayer(g *gamerepo.Game, playerIdx, each int) {
	if each <= 0 || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	payer := &g.Players[playerIdx]
	others := 0
	for i := range g.Players {
		if i != playerIdx && !g.Players[i].Resigned {
			others++
		}
	}
	owed := others * each
	if owed <= 0 {
		return
	}
	startCash := payer.Cash
	paidTotal := 0
	for i := range g.Players {
		if i == playerIdx || g.Players[i].Resigned {
			continue
		}
		need := each
		avail := payer.Cash
		if avail < 0 {
			avail = 0
		}
		pay := need
		if avail < pay {
			pay = avail
		}
		if pay > 0 {
			payer.Cash -= pay
			g.Players[i].Cash += pay
			paidTotal += pay
		}
	}
	// Drive cash to startCash - owed (negative shortfall).
	target := startCash - owed
	if payer.Cash > target {
		payer.Cash = target
	}
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         "card",
		FromUserID:   payer.UserID,
		FromUsername: payer.Username,
		ToUserID:     "",
		ToUsername:   "Players",
		Amount:       paidTotal,
		BoardIndex:   payer.BoardIndex,
		SpaceName:    "Elected Chairman",
		PaidInFull:   paidTotal >= owed,
	}
	if paidTotal < owed {
		g.PendingPayment = &gamerepo.PendingPayment{
			Kind:       "card",
			Amount:     owed - paidTotal,
			FromUserID: payer.UserID,
			ToUserID:   "",
			BoardIndex: payer.BoardIndex,
			SpaceName:  "Elected Chairman",
		}
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	}
}

func collectFromEachOtherPlayer(g *gamerepo.Game, playerIdx, each int) {
	if each <= 0 || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	collector := &g.Players[playerIdx]
	got := 0
	for i := range g.Players {
		if i == playerIdx || g.Players[i].Resigned {
			continue
		}
		pay := each
		if g.Players[i].Cash < pay {
			pay = g.Players[i].Cash
		}
		if pay <= 0 {
			continue
		}
		g.Players[i].Cash -= pay
		collector.Cash += pay
		got += pay
	}
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         "card",
		FromUserID:   "",
		FromUsername: "Players",
		ToUserID:     collector.UserID,
		ToUsername:   collector.Username,
		Amount:       got,
		BoardIndex:   collector.BoardIndex,
		SpaceName:    "Birthday",
		PaidInFull:   true,
	}
}
