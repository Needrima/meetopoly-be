package game

import (
	"crypto/rand"
	"math/big"

	gamerepo "meetopoly-be/internal/repository/game"
)

// Deck kinds (land specialType + LastCard.Deck).
const (
	DeckChance = "chance"
	DeckChest  = "community_chest"
)

// Chance card IDs (Phase 12.2 catalog; effects resolved in 12.3).
const (
	CardChanceAdvanceBoardwalk = "chance_advance_boardwalk"
	CardChanceAdvanceGO        = "chance_advance_go"
	CardChanceAdvanceIllinois  = "chance_advance_illinois"
	CardChanceAdvanceStCharles = "chance_advance_st_charles"
	CardChanceNearestRailroad  = "chance_nearest_railroad"
	CardChanceNearestUtility   = "chance_nearest_utility"
	CardChanceDividend         = "chance_dividend"
	CardChanceGetOutOfJail     = "chance_get_out_of_jail"
	CardChanceGoBack3          = "chance_go_back_3"
	CardChanceGoToJail         = "chance_go_to_jail"
	CardChanceGeneralRepairs   = "chance_general_repairs"
	CardChanceSpeedingFine     = "chance_speeding_fine"
	CardChanceReadingRailroad  = "chance_reading_railroad"
	CardChanceChairman         = "chance_chairman"
	CardChanceBuildingLoan     = "chance_building_loan"
)

// Community Chest card IDs.
const (
	CardChestAdvanceGO       = "chest_advance_go"
	CardChestBankError       = "chest_bank_error"
	CardChestDoctorsFee      = "chest_doctors_fee"
	CardChestSaleOfStock     = "chest_sale_of_stock"
	CardChestGetOutOfJail    = "chest_get_out_of_jail"
	CardChestGoToJail        = "chest_go_to_jail"
	CardChestHolidayFund     = "chest_holiday_fund"
	CardChestIncomeTaxRefund = "chest_income_tax_refund"
	CardChestBirthday        = "chest_birthday"
	CardChestLifeInsurance   = "chest_life_insurance"
	CardChestHospitalFees    = "chest_hospital_fees"
	CardChestSchoolFees      = "chest_school_fees"
	CardChestConsultancyFee  = "chest_consultancy_fee"
	CardChestStreetRepairs   = "chest_street_repairs"
	CardChestBeautyContest   = "chest_beauty_contest"
	CardChestInheritance     = "chest_inheritance"
)

// CardDef is catalog metadata for a Chance/Chest card.
type CardDef struct {
	ID    string
	Deck  string
	Title string
	// EffectKind reserved for Phase 12.3 resolve (stubbed in 12.2).
	EffectKind string
}

var cardCatalog = map[string]CardDef{
	CardChanceAdvanceBoardwalk: {ID: CardChanceAdvanceBoardwalk, Deck: DeckChance, Title: "Advance to Boardwalk", EffectKind: "move_to"},
	CardChanceAdvanceGO:        {ID: CardChanceAdvanceGO, Deck: DeckChance, Title: "Advance to GO", EffectKind: "move_to"},
	CardChanceAdvanceIllinois:  {ID: CardChanceAdvanceIllinois, Deck: DeckChance, Title: "Advance to Illinois Avenue", EffectKind: "move_to"},
	CardChanceAdvanceStCharles: {ID: CardChanceAdvanceStCharles, Deck: DeckChance, Title: "Advance to St. Charles Place", EffectKind: "move_to"},
	CardChanceNearestRailroad:  {ID: CardChanceNearestRailroad, Deck: DeckChance, Title: "Advance to nearest Railroad", EffectKind: "nearest_railroad"},
	CardChanceNearestUtility:   {ID: CardChanceNearestUtility, Deck: DeckChance, Title: "Advance to nearest Utility", EffectKind: "nearest_utility"},
	CardChanceDividend:         {ID: CardChanceDividend, Deck: DeckChance, Title: "Bank pays you dividend", EffectKind: "cash"},
	CardChanceGetOutOfJail:     {ID: CardChanceGetOutOfJail, Deck: DeckChance, Title: "Get Out of Jail Free", EffectKind: "goojf"},
	CardChanceGoBack3:          {ID: CardChanceGoBack3, Deck: DeckChance, Title: "Go Back 3 Spaces", EffectKind: "go_back"},
	CardChanceGoToJail:         {ID: CardChanceGoToJail, Deck: DeckChance, Title: "Go to Jail", EffectKind: "jail"},
	CardChanceGeneralRepairs:   {ID: CardChanceGeneralRepairs, Deck: DeckChance, Title: "Make general repairs", EffectKind: "repairs"},
	CardChanceSpeedingFine:     {ID: CardChanceSpeedingFine, Deck: DeckChance, Title: "Speeding fine", EffectKind: "cash"},
	CardChanceReadingRailroad:  {ID: CardChanceReadingRailroad, Deck: DeckChance, Title: "Take a trip to Reading Railroad", EffectKind: "move_to"},
	CardChanceChairman:         {ID: CardChanceChairman, Deck: DeckChance, Title: "Elected Chairman of the Board", EffectKind: "pay_each"},
	CardChanceBuildingLoan:     {ID: CardChanceBuildingLoan, Deck: DeckChance, Title: "Building loan matures", EffectKind: "cash"},

	CardChestAdvanceGO:       {ID: CardChestAdvanceGO, Deck: DeckChest, Title: "Advance to GO", EffectKind: "move_to"},
	CardChestBankError:       {ID: CardChestBankError, Deck: DeckChest, Title: "Bank error in your favor", EffectKind: "cash"},
	CardChestDoctorsFee:      {ID: CardChestDoctorsFee, Deck: DeckChest, Title: "Doctor's fee", EffectKind: "cash"},
	CardChestSaleOfStock:     {ID: CardChestSaleOfStock, Deck: DeckChest, Title: "From sale of stock", EffectKind: "cash"},
	CardChestGetOutOfJail:    {ID: CardChestGetOutOfJail, Deck: DeckChest, Title: "Get Out of Jail Free", EffectKind: "goojf"},
	CardChestGoToJail:        {ID: CardChestGoToJail, Deck: DeckChest, Title: "Go to Jail", EffectKind: "jail"},
	CardChestHolidayFund:     {ID: CardChestHolidayFund, Deck: DeckChest, Title: "Holiday fund matures", EffectKind: "cash"},
	CardChestIncomeTaxRefund: {ID: CardChestIncomeTaxRefund, Deck: DeckChest, Title: "Income tax refund", EffectKind: "cash"},
	CardChestBirthday:        {ID: CardChestBirthday, Deck: DeckChest, Title: "It's your birthday", EffectKind: "collect_each"},
	CardChestLifeInsurance:   {ID: CardChestLifeInsurance, Deck: DeckChest, Title: "Life insurance matures", EffectKind: "cash"},
	CardChestHospitalFees:    {ID: CardChestHospitalFees, Deck: DeckChest, Title: "Hospital fees", EffectKind: "cash"},
	CardChestSchoolFees:      {ID: CardChestSchoolFees, Deck: DeckChest, Title: "School fees", EffectKind: "cash"},
	CardChestConsultancyFee:  {ID: CardChestConsultancyFee, Deck: DeckChest, Title: "Consultancy fee", EffectKind: "cash"},
	CardChestStreetRepairs:   {ID: CardChestStreetRepairs, Deck: DeckChest, Title: "Street repairs", EffectKind: "repairs"},
	CardChestBeautyContest:   {ID: CardChestBeautyContest, Deck: DeckChest, Title: "Second prize in beauty contest", EffectKind: "cash"},
	CardChestInheritance:     {ID: CardChestInheritance, Deck: DeckChest, Title: "Inheritance", EffectKind: "cash"},
}

func chanceDeckTemplate() []string {
	return []string{
		CardChanceAdvanceBoardwalk,
		CardChanceAdvanceGO,
		CardChanceAdvanceIllinois,
		CardChanceAdvanceStCharles,
		CardChanceNearestRailroad,
		CardChanceNearestRailroad, // 2 copies
		CardChanceNearestUtility,
		CardChanceDividend,
		CardChanceGetOutOfJail,
		CardChanceGoBack3,
		CardChanceGoToJail,
		CardChanceGeneralRepairs,
		CardChanceSpeedingFine,
		CardChanceReadingRailroad,
		CardChanceChairman,
		CardChanceBuildingLoan,
	}
}

func chestDeckTemplate() []string {
	return []string{
		CardChestAdvanceGO,
		CardChestBankError,
		CardChestDoctorsFee,
		CardChestSaleOfStock,
		CardChestGetOutOfJail,
		CardChestGoToJail,
		CardChestHolidayFund,
		CardChestIncomeTaxRefund,
		CardChestBirthday,
		CardChestLifeInsurance,
		CardChestHospitalFees,
		CardChestSchoolFees,
		CardChestConsultancyFee,
		CardChestStreetRepairs,
		CardChestBeautyContest,
		CardChestInheritance,
	}
}

func shuffleDeck(ids []string) []string {
	out := append([]string(nil), ids...)
	for i := len(out) - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := 0
		if err == nil {
			j = int(jBig.Int64())
		}
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func newShuffledChanceDeck() []string {
	return shuffleDeck(chanceDeckTemplate())
}

func newShuffledChestDeck() []string {
	return shuffleDeck(chestDeckTemplate())
}

func ensureDecks(g *gamerepo.Game) {
	if g == nil {
		return
	}
	if len(g.ChanceDeck) == 0 {
		g.ChanceDeck = newShuffledChanceDeck()
	}
	if len(g.ChestDeck) == 0 {
		g.ChestDeck = newShuffledChestDeck()
	}
}

func cardTitle(id string) string {
	if def, ok := cardCatalog[id]; ok {
		return def.Title
	}
	return id
}

func isGetOutOfJailCard(id string) bool {
	return id == CardChanceGetOutOfJail || id == CardChestGetOutOfJail
}

// drawCardLocked pops the top card from the named deck, sets LastCard, handles GOOJF vs recycle.
func drawCardLocked(g *gamerepo.Game, playerIdx int, deck string) string {
	ensureDecks(g)
	var pile *[]string
	switch deck {
	case DeckChance:
		pile = &g.ChanceDeck
	case DeckChest:
		pile = &g.ChestDeck
	default:
		return ""
	}
	if len(*pile) == 0 {
		// All GOOJF held; reshuffle a fresh template minus cards held by players.
		*pile = reshuffleDeckMinusHeld(g, deck)
	}
	if len(*pile) == 0 {
		return ""
	}
	id := (*pile)[0]
	*pile = (*pile)[1:]

	if playerIdx >= 0 && playerIdx < len(g.Players) && isGetOutOfJailCard(id) {
		g.Players[playerIdx].GetOutOfJailFreeCards = append(
			g.Players[playerIdx].GetOutOfJailFreeCards, id,
		)
		g.Players[playerIdx].GetOutOfJailFree = len(g.Players[playerIdx].GetOutOfJailFreeCards)
	} else {
		*pile = append(*pile, id)
	}

	title := cardTitle(id)
	userID := ""
	username := ""
	if playerIdx >= 0 && playerIdx < len(g.Players) {
		userID = g.Players[playerIdx].UserID
		username = g.Players[playerIdx].Username
	}
	g.LastCard = &gamerepo.LastCard{
		Deck:     deck,
		CardID:   id,
		Title:    title,
		UserID:   userID,
		Username: username,
	}
	return id
}

func reshuffleDeckMinusHeld(g *gamerepo.Game, deck string) []string {
	var template []string
	switch deck {
	case DeckChance:
		template = chanceDeckTemplate()
	case DeckChest:
		template = chestDeckTemplate()
	default:
		return nil
	}
	held := map[string]int{}
	for _, p := range g.Players {
		for _, id := range p.GetOutOfJailFreeCards {
			held[id]++
		}
	}
	out := make([]string, 0, len(template))
	for _, id := range template {
		if held[id] > 0 {
			held[id]--
			continue
		}
		out = append(out, id)
	}
	return shuffleDeck(out)
}

func returnJailCardToDeck(g *gamerepo.Game, cardID string) {
	ensureDecks(g)
	switch cardID {
	case CardChanceGetOutOfJail:
		g.ChanceDeck = append(g.ChanceDeck, cardID)
	case CardChestGetOutOfJail:
		g.ChestDeck = append(g.ChestDeck, cardID)
	default:
		// Unknown — put on chance bottom.
		g.ChanceDeck = append(g.ChanceDeck, cardID)
	}
}

func popJailFreeCard(p *gamerepo.Player) (string, bool) {
	if p == nil || len(p.GetOutOfJailFreeCards) == 0 {
		// Legacy: count without ids (pre-12.2 docs) — synthesize a chance GOOJF.
		if p != nil && p.GetOutOfJailFree > 0 {
			p.GetOutOfJailFree--
			return CardChanceGetOutOfJail, true
		}
		return "", false
	}
	id := p.GetOutOfJailFreeCards[0]
	p.GetOutOfJailFreeCards = p.GetOutOfJailFreeCards[1:]
	p.GetOutOfJailFree = len(p.GetOutOfJailFreeCards)
	return id, true
}
