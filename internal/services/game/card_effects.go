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
		s.cardAdvanceToLocked(ctx, g, playerIdx, spaces, 5, landingOpts{})
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
	case CardChanceSpeedingFine:
		debitToBank(g, playerIdx, 15, "card", cardTitle(cardID))
	case CardChanceBuildingLoan:
		creditFromBank(g, playerIdx, 150, "card")
	case CardChanceChairman:
		payEachOtherPlayer(g, playerIdx, 50)
	case CardChanceGeneralRepairs:
		debitToBank(g, playerIdx, repairsDue(g, playerIdx, 25, 100), "card", cardTitle(cardID))

	case CardChestBankError:
		creditFromBank(g, playerIdx, 200, "card")
	case CardChestDoctorsFee:
		debitToBank(g, playerIdx, 50, "card", cardTitle(cardID))
	case CardChestSaleOfStock:
		creditFromBank(g, playerIdx, 50, "card")
	case CardChestHolidayFund:
		creditFromBank(g, playerIdx, 100, "card")
	case CardChestIncomeTaxRefund:
		creditFromBank(g, playerIdx, 20, "card")
	case CardChestBirthday:
		collectFromEachOtherPlayer(g, playerIdx, 10)
	case CardChestLifeInsurance:
		creditFromBank(g, playerIdx, 100, "card")
	case CardChestHospitalFees:
		debitToBank(g, playerIdx, 100, "card", cardTitle(cardID))
	case CardChestSchoolFees:
		debitToBank(g, playerIdx, 50, "card", cardTitle(cardID))
	case CardChestConsultancyFee:
		creditFromBank(g, playerIdx, 25, "card")
	case CardChestStreetRepairs:
		debitToBank(g, playerIdx, repairsDue(g, playerIdx, 40, 115), "card", cardTitle(cardID))
	case CardChestBeautyContest:
		creditFromBank(g, playerIdx, 10, "card")
	case CardChestInheritance:
		creditFromBank(g, playerIdx, 100, "card")
	}
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
	paidInFull := true
	if g.Players[playerIdx].Cash < amount {
		pay = g.Players[playerIdx].Cash
		paidInFull = false
	}
	g.Players[playerIdx].Cash -= pay
	g.LastPayment = &gamerepo.LastPayment{
		Kind:         kind,
		FromUserID:   g.Players[playerIdx].UserID,
		FromUsername: g.Players[playerIdx].Username,
		ToUserID:     "",
		ToUsername:   "Bank",
		Amount:       pay,
		BoardIndex:   g.Players[playerIdx].BoardIndex,
		SpaceName:    spaceName,
		PaidInFull:   paidInFull,
	}
	if !paidInFull {
		g.PendingPayment = &gamerepo.PendingPayment{
			Kind:       kind,
			Amount:     amount - pay,
			ToUserID:   "",
			BoardIndex: g.Players[playerIdx].BoardIndex,
			SpaceName:  spaceName,
		}
		g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	}
}

func payEachOtherPlayer(g *gamerepo.Game, playerIdx, each int) {
	if each <= 0 || playerIdx < 0 || playerIdx >= len(g.Players) {
		return
	}
	payer := &g.Players[playerIdx]
	paidTotal := 0
	for i := range g.Players {
		if i == playerIdx || g.Players[i].Resigned {
			continue
		}
		pay := each
		if payer.Cash < pay {
			pay = payer.Cash
		}
		if pay <= 0 {
			break
		}
		payer.Cash -= pay
		g.Players[i].Cash += pay
		paidTotal += pay
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
		PaidInFull:   true,
	}
	// Shortfall vs full obligation → pending to bank (raise funds / Phase 14).
	others := 0
	for i := range g.Players {
		if i != playerIdx && !g.Players[i].Resigned {
			others++
		}
	}
	owed := others * each
	if paidTotal < owed {
		g.PendingPayment = &gamerepo.PendingPayment{
			Kind:       "card",
			Amount:     owed - paidTotal,
			ToUserID:   "",
			BoardIndex: payer.BoardIndex,
			SpaceName:  "Elected Chairman",
		}
		g.LastPayment.PaidInFull = false
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
