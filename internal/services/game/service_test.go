package game

import (
	"context"
	"errors"
	"testing"
	"time"

	gamerepo "meetopoly-be/internal/repository/game"
)

type memRepo struct {
	byID    map[string]*gamerepo.Game
	byTable map[string]string
}

func newMemRepo() *memRepo {
	return &memRepo{
		byID:    make(map[string]*gamerepo.Game),
		byTable: make(map[string]string),
	}
}

func (m *memRepo) EnsureIndexes(context.Context) error { return nil }

func (m *memRepo) Insert(_ context.Context, g *gamerepo.Game) error {
	cp := cloneGame(g)
	m.byID[g.ID] = cp
	m.byTable[g.TableID] = g.ID
	return nil
}

func (m *memRepo) Update(_ context.Context, g *gamerepo.Game) error {
	if _, ok := m.byID[g.ID]; !ok {
		return gamerepo.ErrNotFound
	}
	m.byID[g.ID] = cloneGame(g)
	return nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*gamerepo.Game, error) {
	g, ok := m.byID[id]
	if !ok {
		return nil, gamerepo.ErrNotFound
	}
	return cloneGame(g), nil
}

func (m *memRepo) FindByTableID(_ context.Context, tableID string) (*gamerepo.Game, error) {
	id, ok := m.byTable[tableID]
	if !ok {
		return nil, gamerepo.ErrNotFound
	}
	return m.FindByID(context.Background(), id)
}

func cloneGame(g *gamerepo.Game) *gamerepo.Game {
	cp := *g
	cp.Players = append([]gamerepo.Player(nil), g.Players...)
	for i := range cp.Players {
		cp.Players[i].GetOutOfJailFreeCards = append([]string(nil), g.Players[i].GetOutOfJailFreeCards...)
	}
	cp.Deeds = append([]gamerepo.Deed(nil), g.Deeds...)
	cp.ChanceDeck = append([]string(nil), g.ChanceDeck...)
	cp.ChestDeck = append([]string(nil), g.ChestDeck...)
	if g.LastRoll != nil {
		lr := *g.LastRoll
		cp.LastRoll = &lr
	}
	if g.LastPayment != nil {
		lp := *g.LastPayment
		cp.LastPayment = &lp
	}
	if g.PendingPayment != nil {
		pp := *g.PendingPayment
		cp.PendingPayment = &pp
	}
	if g.LastCard != nil {
		lc := *g.LastCard
		cp.LastCard = &lc
	}
	if g.Auction != nil {
		a := *g.Auction
		a.FoldedUserIDs = append([]string(nil), g.Auction.FoldedUserIDs...)
		a.History = append([]gamerepo.AuctionEvent(nil), g.Auction.History...)
		cp.Auction = &a
	}
	if g.LastAuction != nil {
		la := *g.LastAuction
		cp.LastAuction = &la
	}
	if g.Trade != nil {
		tr := *g.Trade
		tr.Give.BoardIndexes = append([]int(nil), g.Trade.Give.BoardIndexes...)
		tr.Take.BoardIndexes = append([]int(nil), g.Trade.Take.BoardIndexes...)
		cp.Trade = &tr
	}
	if g.LastTrade != nil {
		lt := *g.LastTrade
		cp.LastTrade = &lt
	}
	if g.LastForfeit != nil {
		lf := *g.LastForfeit
		cp.LastForfeit = &lf
	}
	return &cp
}

func seedTwoPlayer(t *testing.T, repo *memRepo) {
	t.Helper()
	bank := gamerepo.TimeBankDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g1",
		TableID: "t1",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#f00", TimeRemainingMs: bank},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		DoublesStreak: 0,
		TurnStartedAt: time.Now().UTC(),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

func TestRollDoesNotAdvanceTurn_EndTurnDoes(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	var view *View
	var err error
	// Keep rolling until non-doubles so we hit awaiting_end.
	for i := 0; i < 40; i++ {
		view, err = svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.CurrentUserID != "a" {
			t.Fatalf("turn advanced on roll: %s", view.CurrentUserID)
		}
		if !view.LastRoll.IsDoubles {
			break
		}
		if !view.CanRoll {
			t.Fatal("doubles should allow another roll")
		}
	}
	if view.LastRoll.IsDoubles {
		t.Fatal("could not get a non-doubles roll")
	}
	if view.TurnPhase != gamerepo.TurnPhaseAwaitingEnd || !view.CanEndTurn {
		t.Fatalf("phase=%s canEnd=%v", view.TurnPhase, view.CanEndTurn)
	}
	if _, err := svc.Roll(context.Background(), "g1", "a"); err == nil {
		t.Fatal("expected must end turn")
	}

	view, err = svc.EndTurn(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("expected b, got %s", view.CurrentUserID)
	}
	if view.TurnPhase != gamerepo.TurnPhaseAwaitingRoll || !view.CanRoll {
		t.Fatalf("phase=%s", view.TurnPhase)
	}
}

func TestThirdDoublesSkipsMoveAndRequiresEnd(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	// Force doubles streak to 2, then inject a doubles roll via mutating before Roll
	// by setting streak and using many attempts — instead set streak=2 and mock by
	// directly calling with controlled dice is hard; set DoublesStreak=2 and patch
	// board, then loop until we get doubles once.
	cur, _ := repo.FindByID(context.Background(), "g1")
	cur.DoublesStreak = 2
	cur.Players[0].BoardIndex = 10
	_ = repo.Update(context.Background(), cur)

	var view *View
	var err error
	found := false
	for i := 0; i < 80; i++ {
		// Reset streak before each attempt if previous roll wasn't doubles
		cur, _ = repo.FindByID(context.Background(), "g1")
		if cur.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
			cur.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		}
		cur.DoublesStreak = 2
		cur.Players[0].BoardIndex = 10
		cur.Players[0].InJail = false
		cur.CurrentTurn = 0
		_ = repo.Update(context.Background(), cur)

		view, err = svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.LastRoll.IsDoubles {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no doubles")
	}
	if !view.LastRoll.ThirdDoubles {
		t.Fatal("expected thirdDoubles")
	}
	if view.Players[0].BoardIndex != gamerepo.JailBoardIndex {
		t.Fatalf("should be on Jail after third doubles, got %d", view.Players[0].BoardIndex)
	}
	if !view.Players[0].InJail {
		t.Fatal("expected inJail after third doubles")
	}
	if view.Players[0].JailTurns != 0 {
		t.Fatalf("jailTurns=%d want 0", view.Players[0].JailTurns)
	}
	if view.LastRoll.ToIndex != gamerepo.JailBoardIndex {
		t.Fatalf("lastRoll.toIndex=%d want jail", view.LastRoll.ToIndex)
	}
	if view.LastRoll.DoublesStreak != 3 {
		t.Fatalf("doublesStreak on lastRoll=%d want 3", view.LastRoll.DoublesStreak)
	}
	if view.DoublesStreak != 0 {
		t.Fatalf("game doublesStreak=%d want 0 after jail", view.DoublesStreak)
	}
	if !view.CanEndTurn {
		t.Fatal("must end after third doubles")
	}
	if view.CanRoll {
		t.Fatal("cannot roll after third doubles")
	}
}

func TestGoToJailLandingSendsToJail(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 10, Slug: "jail", Name: "Jail", Kind: "special", SpecialType: "jail"},
		{BoardIndex: 30, Slug: "go-to-jail", Name: "Go to Jail", Kind: "special", SpecialType: "go_to_jail"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 30
	g.Players[0].Cash = 2000
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	g.DoublesStreak = 1
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 2, Total: 3,
		FromIndex: 27, ToIndex: 30, DoublesStreak: 1,
	}
	_ = repo.Update(context.Background(), g)

	g, _ = repo.FindByID(context.Background(), "g1")
	svc.resolveLandingLocked(context.Background(), g, 0, 3)
	_ = repo.Update(context.Background(), g)

	view := svc.viewOf(context.Background(), g)
	if view.Players[0].BoardIndex != 10 {
		t.Fatalf("boardIndex=%d want 10", view.Players[0].BoardIndex)
	}
	if !view.Players[0].InJail {
		t.Fatal("expected inJail")
	}
	if view.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
		t.Fatalf("phase=%s", view.TurnPhase)
	}
	if view.DoublesStreak != 0 {
		t.Fatalf("doublesStreak=%d want 0", view.DoublesStreak)
	}
	if view.LastRoll == nil || view.LastRoll.ToIndex != 10 {
		t.Fatalf("lastRoll.toIndex=%v", view.LastRoll)
	}
	if view.Players[0].Cash != 2000 {
		t.Fatalf("cash=%d — teleport must not award GO", view.Players[0].Cash)
	}
}

func TestJailJustVisitingDoesNotJail(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 10, Slug: "jail", Name: "Jail", Kind: "special", SpecialType: "jail"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 10
	g.Players[0].InJail = false
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 2, Die2: 3, Total: 5,
		FromIndex: 5, ToIndex: 10,
	}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].InJail {
		t.Fatal("Just Visiting must not set inJail")
	}
	if g.Players[0].BoardIndex != 10 {
		t.Fatalf("boardIndex=%d", g.Players[0].BoardIndex)
	}
}

func TestRollBlockedWhileInJail(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = gamerepo.JailBoardIndex
	g.Players[0].InJail = true
	g.Players[0].JailTurns = 0
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	_ = repo.Update(context.Background(), g)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !view.CanRoll {
		t.Fatal("CanRoll should be true while inJail with jailTurns < 3")
	}
	if !view.CanPayJailFine {
		t.Fatal("CanPayJailFine expected with cash >= 100")
	}
}

func TestPayJailFineLeavesJail(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = gamerepo.JailBoardIndex
	g.Players[0].InJail = true
	g.Players[0].JailTurns = 2
	g.Players[0].Cash = 2000
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	_ = repo.Update(context.Background(), g)

	view, err := svc.PayJailFine(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].InJail {
		t.Fatal("expected out of jail")
	}
	if view.Players[0].Cash != 1900 {
		t.Fatalf("cash=%d want 1900", view.Players[0].Cash)
	}
	if view.Players[0].BoardIndex != gamerepo.JailBoardIndex {
		t.Fatalf("pin should stay on jail until roll, got %d", view.Players[0].BoardIndex)
	}
	if view.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
		t.Fatalf("phase=%s", view.TurnPhase)
	}
	if !view.CanRoll || view.CanPayJailFine {
		t.Fatalf("canRoll=%v canPay=%v", view.CanRoll, view.CanPayJailFine)
	}
	if view.LastPayment == nil || view.LastPayment.Kind != "jail_fine" || view.LastPayment.Amount != 100 {
		t.Fatalf("lastPayment=%v", view.LastPayment)
	}
}

func TestUseJailCardLeavesJail(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].InJail = true
	g.Players[0].GetOutOfJailFreeCards = []string{CardChanceGetOutOfJail}
	g.Players[0].GetOutOfJailFree = 1
	g.ChanceDeck = []string{CardChanceDividend}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	_ = repo.Update(context.Background(), g)

	view, err := svc.UseJailCard(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].InJail || view.Players[0].GetOutOfJailFree != 0 {
		t.Fatalf("inJail=%v cards=%d", view.Players[0].InJail, view.Players[0].GetOutOfJailFree)
	}
	g2, _ := repo.FindByID(context.Background(), "g1")
	if len(g2.ChanceDeck) != 2 || g2.ChanceDeck[len(g2.ChanceDeck)-1] != CardChanceGetOutOfJail {
		t.Fatalf("chanceDeck=%v — GOOJF should return to bottom", g2.ChanceDeck)
	}
	if view.TurnPhase != gamerepo.TurnPhaseAwaitingRoll {
		t.Fatalf("phase=%s", view.TurnPhase)
	}
}

func TestChanceChestCatalogSizes(t *testing.T) {
	if len(chanceDeckTemplate()) != 16 {
		t.Fatalf("chance=%d", len(chanceDeckTemplate()))
	}
	if len(chestDeckTemplate()) != 16 {
		t.Fatalf("chest=%d", len(chestDeckTemplate()))
	}
	rr := 0
	for _, id := range chanceDeckTemplate() {
		if id == CardChanceNearestRailroad {
			rr++
		}
	}
	if rr != 2 {
		t.Fatalf("nearest railroad copies=%d want 2", rr)
	}
}

func TestCreateFromSeatsShufflesDecks(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	view, err := svc.CreateFromSeats(context.Background(), "t1", "africa-1", []SeatInput{
		{UserID: "a", Username: "A", SeatIndex: 0},
		{UserID: "b", Username: "B", SeatIndex: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := repo.FindByID(context.Background(), view.ID)
	if len(g.ChanceDeck) != 16 || len(g.ChestDeck) != 16 {
		t.Fatalf("chance=%d chest=%d", len(g.ChanceDeck), len(g.ChestDeck))
	}
}

func TestDrawChanceCardOnLand(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceDividend, CardChanceSpeedingFine}
	g.ChestDeck = newShuffledChestDeck()
	_ = repo.Update(context.Background(), g)

	g, _ = repo.FindByID(context.Background(), "g1")
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	_ = repo.Update(context.Background(), g)

	view := svc.viewOf(context.Background(), g)
	if view.LastCard == nil || view.LastCard.CardID != CardChanceDividend {
		t.Fatalf("lastCard=%v", view.LastCard)
	}
	if view.Players[0].Cash != 2050 {
		t.Fatalf("cash=%d want 2050 after dividend", view.Players[0].Cash)
	}
	if len(g.ChanceDeck) != 2 || g.ChanceDeck[0] != CardChanceSpeedingFine || g.ChanceDeck[1] != CardChanceDividend {
		t.Fatalf("chanceDeck=%v", g.ChanceDeck)
	}
}

func TestDrawGetOutOfJailFreeHoldsCard(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 2, Slug: "chest", Name: "Chest", Kind: "special", SpecialType: "community_chest"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 2
	g.ChestDeck = []string{CardChestGetOutOfJail, CardChestBankError}
	g.ChanceDeck = newShuffledChanceDeck()
	_ = repo.Update(context.Background(), g)

	g, _ = repo.FindByID(context.Background(), "g1")
	svc.resolveLandingLocked(context.Background(), g, 0, 2)

	if g.Players[0].GetOutOfJailFree != 1 {
		t.Fatalf("goojf=%d", g.Players[0].GetOutOfJailFree)
	}
	if len(g.Players[0].GetOutOfJailFreeCards) != 1 || g.Players[0].GetOutOfJailFreeCards[0] != CardChestGetOutOfJail {
		t.Fatalf("cards=%v", g.Players[0].GetOutOfJailFreeCards)
	}
	if len(g.ChestDeck) != 1 || g.ChestDeck[0] != CardChestBankError {
		t.Fatalf("chestDeck=%v — GOOJF must not recycle", g.ChestDeck)
	}
	if g.LastCard == nil || g.LastCard.CardID != CardChestGetOutOfJail {
		t.Fatalf("lastCard=%v", g.LastCard)
	}
}

func TestCardBuildingLoanSetsCashDelta(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceBuildingLoan}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", FromIndex: 2, ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].Cash != 2150 {
		t.Fatalf("cash=%d want 2150", g.Players[0].Cash)
	}
	if g.LastCard == nil || g.LastCard.CashDelta != 150 {
		t.Fatalf("lastCard=%v want cashDelta 150", g.LastCard)
	}
	view := svc.viewOf(context.Background(), g)
	if view.LastCard == nil || view.LastCard.CashDelta != 150 {
		t.Fatalf("view.lastCard=%v", view.LastCard)
	}
}

func TestCardChairmanCashDeltaIsPayEachTimesOthers(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.Players[1].Cash = 2000
	g.ChanceDeck = []string{CardChanceChairman}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.LastCard == nil || g.LastCard.CashDelta != -50 {
		t.Fatalf("lastCard cashDelta=%v want -50 (1 other × 50)", g.LastCard)
	}
}

func TestCardSpeedingFineCashDeltaNegative(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceSpeedingFine}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.LastCard == nil || g.LastCard.CashDelta != -15 {
		t.Fatalf("lastCard=%v want cashDelta -15", g.LastCard)
	}
}

func TestCardAdvanceTitleUsesWorldLocationName(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
		{BoardIndex: 11, Slug: "tallinn", Name: "Tallinn", Kind: "property"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.ChanceDeck = []string{CardChanceAdvanceStCharles}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", FromIndex: 2, ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].BoardIndex != 11 {
		t.Fatalf("board=%d want 11", g.Players[0].BoardIndex)
	}
	if g.LastCard == nil || g.LastCard.Title != "Advance to Tallinn" {
		t.Fatalf("lastCard.title=%v want Advance to Tallinn", g.LastCard)
	}
}

func TestCardAdvanceToGO(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 0, Slug: "go", Name: "GO", Kind: "special", SpecialType: "go"},
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceAdvanceGO}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", FromIndex: 2, ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].BoardIndex != 0 {
		t.Fatalf("board=%d", g.Players[0].BoardIndex)
	}
	if g.Players[0].Cash != 2200 {
		t.Fatalf("cash=%d want 2200", g.Players[0].Cash)
	}
	if g.LastRoll.ToIndex != 0 {
		t.Fatalf("lastRoll.to=%d", g.LastRoll.ToIndex)
	}
}

func TestCardGoToJail(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 10, Slug: "jail", Name: "Jail", Kind: "special", SpecialType: "jail"},
		{BoardIndex: 2, Slug: "chest", Name: "Chest", Kind: "special", SpecialType: "community_chest"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 2
	g.ChestDeck = []string{CardChestGoToJail}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", ToIndex: 2}
	svc.resolveLandingLocked(context.Background(), g, 0, 2)

	if !g.Players[0].InJail || g.Players[0].BoardIndex != 10 {
		t.Fatalf("inJail=%v board=%d", g.Players[0].InJail, g.Players[0].BoardIndex)
	}
	if g.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
		t.Fatalf("phase=%s", g.TurnPhase)
	}
}

func TestCardTripAirportGoesToIndex15NoPassGoFromChance7(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 0, Slug: "go", Name: "GO", Kind: "special", SpecialType: "go"},
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
		{BoardIndex: 15, Slug: "cdg", Name: "Charles de Gaulle Airport", Kind: "railroad", Price: 200, Rents: []int{25}},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceReadingRailroad}
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", FromIndex: 2, ToIndex: 7, Total: 5,
	}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].BoardIndex != 15 {
		t.Fatalf("board=%d want 15", g.Players[0].BoardIndex)
	}
	// 7 → 15 is forward on the same side — no Pass GO.
	if g.Players[0].Cash != 2000 {
		t.Fatalf("cash=%d want 2000 (no wrap)", g.Players[0].Cash)
	}
	if g.LastCard == nil || g.LastCard.Title != "Take a trip to Charles de Gaulle Airport" {
		t.Fatalf("lastCard=%v", g.LastCard)
	}
}

func TestCardTripAirportWrapsPastGOFromLateChance(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 0, Slug: "go", Name: "GO", Kind: "special", SpecialType: "go"},
		{BoardIndex: 36, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
		{BoardIndex: 15, Slug: "air", Name: "Left Airport", Kind: "railroad"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 36
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceReadingRailroad}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", FromIndex: 30, ToIndex: 36, Total: 6}
	svc.resolveLandingLocked(context.Background(), g, 0, 6)

	if g.Players[0].BoardIndex != 15 {
		t.Fatalf("board=%d want 15", g.Players[0].BoardIndex)
	}
	if g.Players[0].Cash != 2200 {
		t.Fatalf("cash=%d want 2200 (pass GO)", g.Players[0].Cash)
	}
	if g.LastRoll == nil || !g.LastRoll.PassedGo || g.LastRoll.PassGoAmount < 200 {
		t.Fatalf("lastRoll passGO=%v amt=%v", g.LastRoll.PassedGo, g.LastRoll.PassGoAmount)
	}
}

func TestCardNearestRailroadDoubleRent(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
		{BoardIndex: 15, Slug: "rr", Name: "Air", Kind: "railroad", Price: 200, Rents: []int{25, 50, 100, 200}},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.Players[1].Cash = 2000
	g.Deeds = []gamerepo.Deed{{BoardIndex: 15, OwnerUserID: "b"}}
	g.ChanceDeck = []string{CardChanceNearestRailroad}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", ToIndex: 7, Total: 5}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].BoardIndex != 15 {
		t.Fatalf("board=%d want 15", g.Players[0].BoardIndex)
	}
	// Base railroad rent 25 × 2 = 50
	if g.Players[0].Cash != 1950 || g.Players[1].Cash != 2050 {
		t.Fatalf("cash a=%d b=%d", g.Players[0].Cash, g.Players[1].Cash)
	}
}

func TestCardGoBack3(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 7, Slug: "chance", Name: "Chance", Kind: "special", SpecialType: "chance"},
		{BoardIndex: 4, Slug: "tax", Name: "Tax", Kind: "special", SpecialType: "tax", TaxAmount: 200},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 7
	g.Players[0].Cash = 2000
	g.ChanceDeck = []string{CardChanceGoBack3}
	g.LastRoll = &gamerepo.LastRoll{UserID: "a", ToIndex: 7}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)

	if g.Players[0].BoardIndex != 4 {
		t.Fatalf("board=%d want 4", g.Players[0].BoardIndex)
	}
	if g.Players[0].Cash != 1800 {
		t.Fatalf("cash=%d want 1800 after tax", g.Players[0].Cash)
	}
}

func TestCardRepairsAndBirthday(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = 2000
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "a", Houses: 2},
		{BoardIndex: 3, OwnerUserID: "a", Houses: 5},
	}
	g.LastCard = &gamerepo.LastCard{
		Deck: DeckChance, CardID: CardChanceGeneralRepairs, Title: "Make general repairs",
		UserID: "a", Username: "A",
	}
	// Chance general repairs: 2*25 + 100 = 150
	svc.applyCardEffectLocked(context.Background(), g, 0, CardChanceGeneralRepairs)
	if g.Players[0].Cash != 1850 {
		t.Fatalf("after repairs cash=%d", g.Players[0].Cash)
	}
	if g.LastCard.CashDelta != -150 {
		t.Fatalf("repairs cashDelta=%d want -150", g.LastCard.CashDelta)
	}

	g.Players[0].Cash = 2000
	g.Players[1].Cash = 2000
	g.LastCard = &gamerepo.LastCard{
		Deck: DeckChest, CardID: CardChestBirthday, Title: "It's your birthday",
		UserID: "a", Username: "A",
	}
	svc.applyCardEffectLocked(context.Background(), g, 0, CardChestBirthday)
	if g.Players[0].Cash != 2010 || g.Players[1].Cash != 1990 {
		t.Fatalf("birthday a=%d b=%d", g.Players[0].Cash, g.Players[1].Cash)
	}
	if g.LastCard.CashDelta != 10 {
		t.Fatalf("birthday cashDelta=%d want 10", g.LastCard.CashDelta)
	}
}

func TestNextIndexForward(t *testing.T) {
	d, wrap := nextIndexForward(7, railroadIndices)
	if d != 15 || wrap {
		t.Fatalf("got %d wrap=%v", d, wrap)
	}
	d, wrap = nextIndexForward(35, railroadIndices)
	if d != 5 || !wrap {
		t.Fatalf("got %d wrap=%v", d, wrap)
	}
}

func TestJailDoublesExitMoves(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	found := false
	for i := 0; i < 80; i++ {
		g, _ := repo.FindByID(context.Background(), "g1")
		g.Players[0].BoardIndex = gamerepo.JailBoardIndex
		g.Players[0].InJail = true
		g.Players[0].JailTurns = 0
		g.Players[0].Cash = 2000
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		g.CurrentTurn = 0
		g.DoublesStreak = 0
		_ = repo.Update(context.Background(), g)

		view, err := svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if !view.LastRoll.IsDoubles {
			continue
		}
		found = true
		if view.Players[0].InJail {
			t.Fatal("doubles should leave jail")
		}
		want := (gamerepo.JailBoardIndex + view.LastRoll.Total) % 40
		if view.Players[0].BoardIndex != want {
			t.Fatalf("board=%d want %d", view.Players[0].BoardIndex, want)
		}
		if view.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
			t.Fatalf("no re-roll after jail doubles, phase=%s", view.TurnPhase)
		}
		break
	}
	if !found {
		t.Fatal("no doubles from jail")
	}
}

func TestJailFailedAttemptsThenForcedPay(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	// Two failed non-doubles attempts
	for attempt := 1; attempt <= 2; attempt++ {
		got := false
		for i := 0; i < 80; i++ {
			g, _ := repo.FindByID(context.Background(), "g1")
			g.Players[0].BoardIndex = gamerepo.JailBoardIndex
			g.Players[0].InJail = true
			g.Players[0].JailTurns = attempt - 1
			g.Players[0].Cash = 2000
			g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
			g.CurrentTurn = 0
			_ = repo.Update(context.Background(), g)

			view, err := svc.Roll(context.Background(), "g1", "a")
			if err != nil {
				t.Fatal(err)
			}
			if view.LastRoll.IsDoubles {
				continue
			}
			got = true
			if !view.Players[0].InJail {
				t.Fatal("should stay in jail")
			}
			if view.Players[0].JailTurns != attempt {
				t.Fatalf("jailTurns=%d want %d", view.Players[0].JailTurns, attempt)
			}
			if view.TurnPhase != gamerepo.TurnPhaseAwaitingEnd {
				t.Fatalf("phase=%s", view.TurnPhase)
			}
			break
		}
		if !got {
			t.Fatalf("no non-doubles for attempt %d", attempt)
		}
	}

	// Third fail with cash → pay + move
	got := false
	for i := 0; i < 80; i++ {
		g, _ := repo.FindByID(context.Background(), "g1")
		g.Players[0].BoardIndex = gamerepo.JailBoardIndex
		g.Players[0].InJail = true
		g.Players[0].JailTurns = 2
		g.Players[0].Cash = 2000
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		g.CurrentTurn = 0
		_ = repo.Update(context.Background(), g)

		view, err := svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.LastRoll.IsDoubles {
			continue
		}
		got = true
		if view.Players[0].InJail {
			t.Fatal("third fail with cash should leave jail")
		}
		if view.Players[0].Cash != 1900 {
			t.Fatalf("cash=%d want 1900", view.Players[0].Cash)
		}
		want := (gamerepo.JailBoardIndex + view.LastRoll.Total) % 40
		if view.Players[0].BoardIndex != want {
			t.Fatalf("board=%d want %d", view.Players[0].BoardIndex, want)
		}
		break
	}
	if !got {
		t.Fatal("no non-doubles on third attempt")
	}
}

func TestJailThirdFailBrokeRequiresPay(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	got := false
	for i := 0; i < 80; i++ {
		g, _ := repo.FindByID(context.Background(), "g1")
		g.Players[0].BoardIndex = gamerepo.JailBoardIndex
		g.Players[0].InJail = true
		g.Players[0].JailTurns = 2
		g.Players[0].Cash = 50
		g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		g.CurrentTurn = 0
		_ = repo.Update(context.Background(), g)

		view, err := svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.LastRoll.IsDoubles {
			continue
		}
		got = true
		if !view.Players[0].InJail || view.Players[0].JailTurns != 3 {
			t.Fatalf("inJail=%v turns=%d", view.Players[0].InJail, view.Players[0].JailTurns)
		}
		if view.Players[0].Cash != 50 {
			t.Fatalf("cash should be unchanged, got %d", view.Players[0].Cash)
		}
		if view.CanRoll {
			t.Fatal("CanRoll false until pay after 3 fails")
		}
		_, err = svc.Roll(context.Background(), "g1", "a")
		if !errors.Is(err, ErrMustLeaveJail) && !errors.Is(err, ErrMustEndTurn) {
			// awaiting_end so must end first
			if view.TurnPhase == gamerepo.TurnPhaseAwaitingEnd && !errors.Is(err, ErrMustEndTurn) {
				t.Fatalf("err=%v", err)
			}
		}
		break
	}
	if !got {
		t.Fatal("no non-doubles")
	}
}

func TestPassGoStillWorks(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)
	cur, _ := repo.FindByID(context.Background(), "g1")
	cur.Players[0].BoardIndex = 38
	_ = repo.Update(context.Background(), cur)

	for i := 0; i < 40; i++ {
		cur, _ = repo.FindByID(context.Background(), "g1")
		cur.Players[0].BoardIndex = 38
		cur.Players[0].Cash = 2000
		cur.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
		cur.DoublesStreak = 0
		cur.CurrentTurn = 0
		_ = repo.Update(context.Background(), cur)

		view, err := svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.LastRoll.ThirdDoubles {
			continue
		}
		wantTo := (38 + view.LastRoll.Total) % 40
		if view.LastRoll.ToIndex != wantTo {
			t.Fatalf("to=%d", view.LastRoll.ToIndex)
		}
		passed := 38+view.LastRoll.Total >= 40
		if view.LastRoll.PassedGo != passed {
			t.Fatalf("passedGo")
		}
		wantCash := 2000
		if passed {
			wantCash = 2200
		}
		if view.Players[0].Cash != wantCash {
			t.Fatalf("cash=%d", view.Players[0].Cash)
		}
		return
	}
	t.Fatal("exhausted")
}

func TestResignLastPlayerWins(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	view, err := svc.Resign(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != gamerepo.StatusFinished {
		t.Fatalf("status=%s", view.Status)
	}
	if view.WinnerUserID != "b" || view.WinnerUsername != "B" {
		t.Fatalf("winner=%s/%s", view.WinnerUserID, view.WinnerUsername)
	}
	if !view.Players[0].Resigned {
		t.Fatal("a should be resigned")
	}
	if view.Players[1].Resigned {
		t.Fatal("b should still be active")
	}
}

func TestResignAdvancesTurnWhenCurrentLeaves(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TimeBankDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g2",
		TableID: "t2",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#f00", TimeRemainingMs: bank},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0", TimeRemainingMs: bank},
			{UserID: "c", Username: "C", SeatIndex: 2, TurnOrder: 2, Cash: 2000, BoardIndex: 0, PinColor: "#00f", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		TurnStartedAt: time.Now().UTC(),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Resign(context.Background(), "g2", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != gamerepo.StatusActive {
		t.Fatalf("status=%s", view.Status)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s", view.CurrentUserID)
	}
}

func TestTurnClockFirstTimeoutForcesEndTurn(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TurnClockDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g1",
		TableID: "t1",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#f00", TimeRemainingMs: 500},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		TurnStartedAt: time.Now().UTC().Add(-2 * time.Second),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Resigned {
		t.Fatal("first timeout should not resign")
	}
	if view.Players[0].TurnTimeouts != 1 {
		t.Fatalf("timeouts=%d", view.Players[0].TurnTimeouts)
	}
	if view.LastForfeit == nil || view.LastForfeit.Reason != "turn_strike" || view.LastForfeit.Strikes != 1 {
		t.Fatalf("lastForfeit=%+v", view.LastForfeit)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s want b", view.CurrentUserID)
	}
}

func TestTurnClockSecondTimeoutAutoResigns(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TurnClockDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g1",
		TableID: "t1",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#f00", TimeRemainingMs: 500, TurnTimeouts: 1},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		TurnStartedAt: time.Now().UTC().Add(-2 * time.Second),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Players[0].Resigned {
		t.Fatal("a should be eliminated on second timeout")
	}
	if view.LastForfeit == nil || view.LastForfeit.Reason != "turn_timeout" || view.LastForfeit.Strikes != 2 {
		t.Fatalf("lastForfeit=%+v", view.LastForfeit)
	}
	if view.Status != gamerepo.StatusFinished {
		t.Fatalf("status=%s want finished", view.Status)
	}
	if view.WinnerUserID != "b" {
		t.Fatalf("winner=%s", view.WinnerUserID)
	}
}

func TestEndTurnPausesBankAndStartsNext(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)

	g, err := repo.FindByID(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.TurnStartedAt = time.Now().UTC().Add(-5 * time.Second)
	startBank := g.Players[0].TimeRemainingMs
	if err := repo.Update(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.EndTurn(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s", view.CurrentUserID)
	}
	if view.Players[0].TimeRemainingMs >= startBank {
		t.Fatalf("a bank should have drained: %d >= %d", view.Players[0].TimeRemainingMs, startBank)
	}
	if view.TurnStartedAt == "" {
		t.Fatal("expected turnStartedAt for b")
	}
}

type memSpaces []Space

func (m memSpaces) ListSpaces(context.Context, string) ([]Space, error) {
	return m, nil
}

func TestBuyUnownedProperty(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
		{BoardIndex: 5, Slug: "air-1", Name: "Air Hub", Kind: "railroad", Price: 200},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 2000
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	_ = repo.Update(context.Background(), g)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !view.CanBuy || view.BuyOffer == nil || view.BuyOffer.Price != 60 {
		t.Fatalf("canBuy=%v offer=%v", view.CanBuy, view.BuyOffer)
	}

	view, err = svc.Buy(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Cash != 1940 {
		t.Fatalf("cash=%d", view.Players[0].Cash)
	}
	if len(view.Deeds) != 1 || view.Deeds[0].OwnerUserID != "a" || view.Deeds[0].BoardIndex != 1 {
		t.Fatalf("deeds=%v", view.Deeds)
	}
	if view.Deeds[0].Houses != 0 || view.Deeds[0].Mortgaged {
		t.Fatalf("new deed houses=%d mortgaged=%v want 0/false", view.Deeds[0].Houses, view.Deeds[0].Mortgaged)
	}
	if view.CanBuy {
		t.Fatal("should not canBuy after purchase")
	}
}

func TestEndTurnBlockedWhileBuyOffer(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 2000
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	_ = repo.Update(context.Background(), g)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !view.CanBuy || view.BuyOffer == nil {
		t.Fatalf("expected buy offer, canBuy=%v offer=%v", view.CanBuy, view.BuyOffer)
	}
	if view.CanEndTurn {
		t.Fatal("canEndTurn must be false while buyOffer is open")
	}

	_, err = svc.EndTurn(context.Background(), "g1", "a")
	if !errors.Is(err, ErrMustResolveBuy) {
		t.Fatalf("err=%v want ErrMustResolveBuy", err)
	}

	view, err = svc.Buy(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.BuyOffer != nil || !view.CanEndTurn {
		t.Fatalf("after buy: offer=%v canEnd=%v", view.BuyOffer, view.CanEndTurn)
	}

	view, err = svc.EndTurn(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s want b", view.CurrentUserID)
	}
}

func TestBuyCannotAfford(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 50
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	_ = repo.Update(context.Background(), g)

	_, err := svc.Buy(context.Background(), "g1", "a")
	if !errors.Is(err, ErrCannotAfford) {
		t.Fatalf("err=%v", err)
	}
}

func TestRentPaidOnOwnedProperty(t *testing.T) {
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2, 10, 30, 90, 160, 250}, ColorGroup: "brown"},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, Rents: []int{4, 20, 60, 180, 320, 450}, ColorGroup: "brown"},
	}
	deeds := []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b"}}
	amount, to, kind, name := rentDueForLanding(spaces, deeds, 1, "a", 7)
	if amount != 2 || to != "b" || kind != "rent" || name != "Lagos" {
		t.Fatalf("got amount=%d to=%s kind=%s name=%s", amount, to, kind, name)
	}
	deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "b"},
		{BoardIndex: 3, OwnerUserID: "b"},
	}
	amount, _, _, _ = rentDueForLanding(spaces, deeds, 1, "a", 7)
	if amount != 4 {
		t.Fatalf("monopoly rent=%d want 4", amount)
	}
}

func TestRentWithHousesAndHotel(t *testing.T) {
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2, 10, 30, 90, 160, 250}, ColorGroup: "brown"},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, Rents: []int{4, 20, 60, 180, 320, 450}, ColorGroup: "brown"},
	}
	// Monopoly + 1–4 houses + hotel use Rents[houses].
	for houses, want := range map[int]int{1: 10, 2: 30, 3: 90, 4: 160, 5: 250} {
		deeds := []gamerepo.Deed{
			{BoardIndex: 1, OwnerUserID: "b", Houses: houses},
			{BoardIndex: 3, OwnerUserID: "b"},
		}
		amount, _, _, _ := rentDueForLanding(spaces, deeds, 1, "a", 7)
		if amount != want {
			t.Fatalf("houses=%d rent=%d want %d", houses, amount, want)
		}
	}
	// Houses without monopoly → site rent only (defensive; impossible under classic rules).
	deeds := []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b", Houses: 3}}
	amount, _, _, _ := rentDueForLanding(spaces, deeds, 1, "a", 7)
	if amount != 2 {
		t.Fatalf("houses without monopoly rent=%d want 2", amount)
	}
}

func seedBrownMonopoly(t *testing.T, repo *memRepo, ownerID string, houses1, houses3 int) {
	t.Helper()
	g, err := repo.FindByID(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: ownerID, Houses: houses1},
		{BoardIndex: 3, OwnerUserID: ownerID, Houses: houses3},
	}
	if err := repo.Update(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

func brownBuildSpaces() memSpaces {
	return memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2, 10, 30, 90, 160, 250}, ColorGroup: "brown", HouseCost: 50},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, Rents: []int{4, 20, 60, 180, 320, 450}, ColorGroup: "brown", HouseCost: 50},
	}
}

func TestBuildHouseOnMonopoly(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	view, err := svc.Build(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Cash != 1950 {
		t.Fatalf("cash=%d want 1950", view.Players[0].Cash)
	}
	var h1, h3 int
	for _, d := range view.Deeds {
		switch d.BoardIndex {
		case 1:
			h1 = d.Houses
		case 3:
			h3 = d.Houses
		}
	}
	if h1 != 1 || h3 != 0 {
		t.Fatalf("houses board1=%d board3=%d want 1/0", h1, h3)
	}
}

func TestBuildEvenBuildEnforced(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 0)

	_, err := svc.Build(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrUnevenBuild) {
		t.Fatalf("err=%v want ErrUnevenBuild", err)
	}
	view, err := svc.Build(context.Background(), "g1", "a", 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 3 && d.Houses != 1 {
			t.Fatalf("board3 houses=%d want 1", d.Houses)
		}
	}
}

func TestBuildRequiresMonopoly(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a"}}
	_ = repo.Update(context.Background(), g)

	_, err := svc.Build(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrNoMonopoly) {
		t.Fatalf("err=%v want ErrNoMonopoly", err)
	}
}

func TestBuildCannotAfford(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = 40
	_ = repo.Update(context.Background(), g)

	_, err := svc.Build(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrCannotAfford) {
		t.Fatalf("err=%v want ErrCannotAfford", err)
	}
}

func TestBuildHotelAndMax(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 4, 4)

	view, err := svc.Build(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 && d.Houses != 5 {
			t.Fatalf("hotel houses=%d want 5", d.Houses)
		}
	}
	_, err = svc.Build(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrMaxBuilt) {
		t.Fatalf("err=%v want ErrMaxBuilt", err)
	}
}

func TestBuildNotYourTurn(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "b", 0, 0)

	_, err := svc.Build(context.Background(), "g1", "b", 1)
	if !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("err=%v want ErrNotYourTurn", err)
	}
}

func TestBuildMortgagedSetBlocked(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "a", Houses: 0, Mortgaged: true},
		{BoardIndex: 3, OwnerUserID: "a", Houses: 0},
	}
	_ = repo.Update(context.Background(), g)

	_, err := svc.Build(context.Background(), "g1", "a", 3)
	if !errors.Is(err, ErrMortgagedSet) {
		t.Fatalf("err=%v want ErrMortgagedSet", err)
	}
}

func TestSellBuildingRefundAndEvenSell(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 2, 1)

	// Must sell from the taller deed first (board 1 has 2).
	_, err := svc.SellBuilding(context.Background(), "g1", "a", 3)
	if !errors.Is(err, ErrUnevenSell) {
		t.Fatalf("err=%v want ErrUnevenSell", err)
	}

	view, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// houseCost 50 → refund 25; start cash 2000.
	if view.Players[0].Cash != 2025 {
		t.Fatalf("cash=%d want 2025", view.Players[0].Cash)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 && d.Houses != 1 {
			t.Fatalf("board1 houses=%d want 1", d.Houses)
		}
	}
}

func TestSellBuildingNothingToSell(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	_, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrNothingToSell) {
		t.Fatalf("err=%v want ErrNothingToSell", err)
	}
}

func TestSellBuildingHotelStep(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 5, 5)

	view, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 && d.Houses != 4 {
			t.Fatalf("after hotel sell houses=%d want 4", d.Houses)
		}
	}
	if view.Players[0].Cash != 2025 {
		t.Fatalf("cash=%d want 2025", view.Players[0].Cash)
	}
}

func TestSellBuildingDuringDebtAppliesRefund(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 1)

	g, _ := repo.FindByID(context.Background(), "g1")
	// Owed 30 after paying what they had: cash −30, B already credited earlier.
	g.Players[0].Cash = -30
	g.Players[1].Cash = 2030
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 30, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// +25 refund → cash −5; B gains 25.
	if view.Players[0].Cash != -5 {
		t.Fatalf("payer cash=%d want -5", view.Players[0].Cash)
	}
	if view.Players[1].Cash != 2055 {
		t.Fatalf("owner cash=%d want 2055", view.Players[1].Cash)
	}
	if view.PendingPayment == nil || view.PendingPayment.Amount != 5 {
		t.Fatalf("pending=%+v want amount 5", view.PendingPayment)
	}
}

func TestSellBuildingClearsDebt(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 1)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -20
	g.Players[1].Cash = 2020
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 20, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// +25 → pay 20 debt, keep 5.
	if view.Players[0].Cash != 5 {
		t.Fatalf("payer cash=%d want 5", view.Players[0].Cash)
	}
	if view.Players[1].Cash != 2040 {
		t.Fatalf("owner cash=%d want 2040", view.Players[1].Cash)
	}
	if view.PendingPayment != nil {
		t.Fatalf("pending should be cleared, got %+v", view.PendingPayment)
	}
}

func TestSellBuildingPartialDebt(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 1)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -40
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 40, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// +25 → cash −15, pending 15.
	if view.Players[0].Cash != -15 {
		t.Fatalf("cash=%d want -15", view.Players[0].Cash)
	}
	if view.PendingPayment == nil || view.PendingPayment.Amount != 15 {
		t.Fatalf("pending=%+v want amount 15", view.PendingPayment)
	}
}

func TestMortgageAndRedeem(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2, 10, 30, 90, 160, 250}, ColorGroup: "brown", HouseCost: 50},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, Rents: []int{4, 20, 60, 180, 320, 450}, ColorGroup: "brown", HouseCost: 50},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	view, err := svc.Mortgage(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// price 60 → mortgage 30; cash 2000+30.
	if view.Players[0].Cash != 2030 {
		t.Fatalf("cash=%d want 2030", view.Players[0].Cash)
	}
	var mortgaged bool
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 {
			mortgaged = d.Mortgaged
		}
	}
	if !mortgaged {
		t.Fatal("expected mortgaged")
	}

	// Redeem: 30 + 3 = 33.
	view, err = svc.Redeem(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Cash != 1997 {
		t.Fatalf("cash=%d want 1997", view.Players[0].Cash)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 && d.Mortgaged {
			t.Fatal("expected unmortgaged")
		}
	}
}

func TestMortgageRequiresSellBuildings(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 0)

	_, err := svc.Mortgage(context.Background(), "g1", "a", 3)
	if !errors.Is(err, ErrMustSellBuildings) {
		t.Fatalf("err=%v want ErrMustSellBuildings", err)
	}
}

func TestMortgageAlreadyMortgaged(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	if _, err := svc.Mortgage(context.Background(), "g1", "a", 1); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Mortgage(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrAlreadyMortgaged) {
		t.Fatalf("err=%v want ErrAlreadyMortgaged", err)
	}
}

func TestRedeemNotMortgaged(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	_, err := svc.Redeem(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrNotMortgaged) {
		t.Fatalf("err=%v want ErrNotMortgaged", err)
	}
}

func TestRedeemCannotAfford(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	if _, err := svc.Mortgage(context.Background(), "g1", "a", 1); err != nil {
		t.Fatal(err)
	}
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = 10
	_ = repo.Update(context.Background(), g)

	_, err := svc.Redeem(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrCannotAfford) {
		t.Fatalf("err=%v want ErrCannotAfford", err)
	}
}

func TestMortgageDuringDebtAppliesPayout(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 0, 0)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -20
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 20, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.Mortgage(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// +30 mortgage → pay 20 debt → cash 10.
	if view.Players[0].Cash != 10 {
		t.Fatalf("cash=%d want 10", view.Players[0].Cash)
	}
	if view.PendingPayment != nil {
		t.Fatalf("pending should clear, got %+v", view.PendingPayment)
	}
}

func TestRedeemBlockedDuringDebt(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -10
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "a", Mortgaged: true},
		{BoardIndex: 3, OwnerUserID: "a"},
	}
	g.PendingPayment = &gamerepo.PendingPayment{Kind: "rent", Amount: 10, FromUserID: "a", ToUserID: "b"}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	_, err := svc.Redeem(context.Background(), "g1", "a", 1)
	if !errors.Is(err, ErrMustSettle) {
		t.Fatalf("err=%v want ErrMustSettle", err)
	}
}

func TestRentZeroWhenMortgaged(t *testing.T) {
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2, 10, 30, 90, 160, 250}, ColorGroup: "brown"},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, Rents: []int{4, 20, 60, 180, 320, 450}, ColorGroup: "brown"},
	}
	deeds := []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "b", Mortgaged: true},
		{BoardIndex: 3, OwnerUserID: "b"},
	}
	amount, to, kind, _ := rentDueForLanding(spaces, deeds, 1, "a", 7)
	if amount != 0 || to != "" || kind != "" {
		t.Fatalf("mortgaged rent amount=%d to=%s kind=%s want 0", amount, to, kind)
	}
}

func TestMortgageRailroad(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 5, Slug: "air", Name: "Air", Kind: "railroad", Price: 200, Rents: []int{25, 50, 100, 200}},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{{BoardIndex: 5, OwnerUserID: "a"}}
	_ = repo.Update(context.Background(), g)

	view, err := svc.Mortgage(context.Background(), "g1", "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Cash != 2100 {
		t.Fatalf("cash=%d want 2100", view.Players[0].Cash)
	}
}

func TestBuyAlreadyOwned(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b"}}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	_ = repo.Update(context.Background(), g)

	_, err := svc.Buy(context.Background(), "g1", "a")
	if !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("err=%v", err)
	}
}

func TestRentAutoCollectOnRoll(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2}, ColorGroup: "brown"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 2000
	g.Players[1].Cash = 2000
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b"}}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	if g.Players[0].Cash != 1998 || g.Players[1].Cash != 2002 {
		t.Fatalf("cash a=%d b=%d", g.Players[0].Cash, g.Players[1].Cash)
	}
	if g.LastPayment == nil || !g.LastPayment.PaidInFull || g.LastPayment.Amount != 2 {
		t.Fatalf("lastPayment=%v", g.LastPayment)
	}
	if g.PendingPayment != nil {
		t.Fatal("expected no pending")
	}
}

func TestTaxAutoCollect(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 4, Slug: "tax", Name: "Income Tax", Kind: "special", SpecialType: "tax", TaxAmount: 200},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 4
	g.Players[0].Cash = 2000
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	if g.Players[0].Cash != 1800 {
		t.Fatalf("cash=%d", g.Players[0].Cash)
	}
	if g.LastPayment == nil || g.LastPayment.Kind != "tax" || g.LastPayment.ToUserID != "" {
		t.Fatalf("payment=%v", g.LastPayment)
	}
}

func TestOwnTileNoRent(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{2}, ColorGroup: "brown"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 2000
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a"}}
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	if g.Players[0].Cash != 2000 || g.LastPayment != nil {
		t.Fatalf("cash=%d pay=%v", g.Players[0].Cash, g.LastPayment)
	}
}

func TestCannotAffordRentGoesNegativeAllowsEnd(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{100}, ColorGroup: "brown"},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 40
	g.Players[1].Cash = 2000
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b"}}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	_ = repo.Update(context.Background(), g)

	if g.Players[0].Cash != -60 || g.Players[1].Cash != 2040 {
		t.Fatalf("cash a=%d b=%d", g.Players[0].Cash, g.Players[1].Cash)
	}
	if g.PendingPayment == nil || g.PendingPayment.Amount != 60 || g.PendingPayment.FromUserID != "a" {
		t.Fatalf("pending=%v", g.PendingPayment)
	}

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if !view.CanEndTurn || view.CanRoll {
		t.Fatalf("canEnd=%v canRoll=%v", view.CanEndTurn, view.CanRoll)
	}

	view, err = svc.EndTurn(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s want b", view.CurrentUserID)
	}
	// A's debt persists; B can play.
	if view.CanRoll != true {
		t.Fatal("B should be able to roll")
	}
}

func TestRailroadAndUtilityRent(t *testing.T) {
	spaces := memSpaces{
		{BoardIndex: 5, Slug: "a1", Name: "Air1", Kind: "railroad", Price: 200, Rents: []int{25, 50, 100, 200}},
		{BoardIndex: 15, Slug: "a2", Name: "Air2", Kind: "railroad", Price: 200, Rents: []int{25, 50, 100, 200}},
		{BoardIndex: 12, Slug: "elc", Name: "Power", Kind: "utility", Price: 150, UtilityMultiplier: []int{4, 10}},
		{BoardIndex: 28, Slug: "wtr", Name: "Water", Kind: "utility", Price: 150, UtilityMultiplier: []int{4, 10}},
	}
	deeds := []gamerepo.Deed{{BoardIndex: 5, OwnerUserID: "b"}}
	amt, _, _, _ := rentDueForLanding(spaces, deeds, 5, "a", 7)
	if amt != 25 {
		t.Fatalf("1 rail=%d", amt)
	}
	deeds = append(deeds, gamerepo.Deed{BoardIndex: 15, OwnerUserID: "b"})
	amt, _, _, _ = rentDueForLanding(spaces, deeds, 5, "a", 7)
	if amt != 50 {
		t.Fatalf("2 rail=%d", amt)
	}
	deeds = []gamerepo.Deed{{BoardIndex: 12, OwnerUserID: "b"}}
	amt, _, _, _ = rentDueForLanding(spaces, deeds, 12, "a", 8)
	if amt != 32 {
		t.Fatalf("1 util=%d want 32", amt)
	}
	deeds = append(deeds, gamerepo.Deed{BoardIndex: 28, OwnerUserID: "b"})
	amt, _, _, _ = rentDueForLanding(spaces, deeds, 12, "a", 8)
	if amt != 80 {
		t.Fatalf("2 util=%d want 80", amt)
	}
}

func TestSetPinColor(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TimeBankDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g-pin",
		TableID: "t-pin",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#ed1b24", TimeRemainingMs: bank},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0072bb", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		TurnStartedAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.SetPinColor(context.Background(), "g-pin", "a", "#8B4513")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].PinColor != "#8b4513" {
		t.Fatalf("pin=%s", view.Players[0].PinColor)
	}

	_, err = svc.SetPinColor(context.Background(), "g-pin", "a", "red")
	if !errors.Is(err, ErrInvalidPinColor) {
		t.Fatalf("err=%v", err)
	}

	_, err = svc.SetPinColor(context.Background(), "g-pin", "a", "#0072BB")
	if !errors.Is(err, ErrInvalidPinColor) {
		t.Fatalf("taken color err=%v", err)
	}

	_, err = svc.SetPinColor(context.Background(), "g-pin", "z", "#112233")
	if !errors.Is(err, ErrNotPlayer) {
		t.Fatalf("err=%v", err)
	}
}

func TestEnterLeaveHub(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TimeBankDuration.Milliseconds()
	g := &gamerepo.Game{
		ID:      "g-hub",
		TableID: "t-hub",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#ed1b24", TimeRemainingMs: bank},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0072bb", TimeRemainingMs: bank},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		TurnStartedAt: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	view, err := svc.EnterHub(context.Background(), "g-hub", "a", "hub:africa-1:lagos", nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubID != "hub:africa-1:lagos" {
		t.Fatalf("hubId=%q", view.Players[0].HubID)
	}

	rev0 := int64(0)
	view, err = svc.EnterHub(context.Background(), "g-hub", "a", "hub:africa-1:lagos", &rev0)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubID != "hub:africa-1:lagos" {
		t.Fatal("idempotent enter")
	}

	_, err = svc.EnterHub(context.Background(), "g-hub", "a", "bad", nil)
	if !errors.Is(err, ErrInvalidHubID) {
		t.Fatalf("err=%v", err)
	}

	view, err = svc.LeaveHub(context.Background(), "g-hub", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubID != "" {
		t.Fatalf("cleared hubId=%q", view.Players[0].HubID)
	}
	if view.Players[0].HubRevision < 1 {
		t.Fatalf("hubRevision=%d after leave", view.Players[0].HubRevision)
	}

	// Stale enter with revision from before leave must not restick hubId.
	stale := int64(0)
	view, err = svc.EnterHub(context.Background(), "g-hub", "a", "hub:africa-1:lagos", &stale)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubID != "" {
		t.Fatalf("stale enter stuck hubId=%q", view.Players[0].HubID)
	}

	// Fresh enter with current revision succeeds.
	cur := view.Players[0].HubRevision
	view, err = svc.EnterHub(context.Background(), "g-hub", "a", "hub:africa-1:lagos", &cur)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubID != "hub:africa-1:lagos" {
		t.Fatalf("fresh enter hubId=%q", view.Players[0].HubID)
	}

	// Leave while already empty still bumps revision (guards in-flight enter).
	view, err = svc.LeaveHub(context.Background(), "g-hub", "a")
	if err != nil {
		t.Fatal(err)
	}
	afterFirst := view.Players[0].HubRevision
	view, err = svc.LeaveHub(context.Background(), "g-hub", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].HubRevision <= afterFirst {
		t.Fatalf("empty leave should bump revision %d → %d", afterFirst, view.Players[0].HubRevision)
	}
}

func TestDisconnectHoldExpiresResigns(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{DisconnectHold: 40 * time.Millisecond})
	seedTwoPlayer(t, repo)

	if err := svc.Disconnect(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != gamerepo.StatusFinished {
		t.Fatalf("status=%s", view.Status)
	}
	if view.WinnerUserID != "b" {
		t.Fatalf("winner=%s", view.WinnerUserID)
	}
	if !view.Players[0].Resigned {
		t.Fatal("a should be resigned after hold")
	}
}

func TestCancelDisconnectHoldKeepsPlayer(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{DisconnectHold: 200 * time.Millisecond})
	seedTwoPlayer(t, repo)

	if err := svc.Disconnect(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	svc.CancelDisconnectHold("g1", "a")
	time.Sleep(250 * time.Millisecond)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != gamerepo.StatusActive {
		t.Fatalf("status=%s", view.Status)
	}
	if view.Players[0].Resigned {
		t.Fatal("a should still be active after cancel")
	}
}

func seedBuyOffer(t *testing.T, repo *memRepo, cashA, cashB int) {
	t.Helper()
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = cashA
	g.Players[1].Cash = cashB
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	_ = repo.Update(context.Background(), g)
}

func TestStartAuctionAndFoldWinsAtBid(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBuyOffer(t, repo, 2000, 2000)

	view, err := svc.StartAuction(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.Auction == nil || view.BuyOffer != nil {
		t.Fatalf("auction=%v buyOffer=%v", view.Auction, view.BuyOffer)
	}
	if view.Auction.CurrentBidderUserID != "a" || view.Auction.MinBid != 1 {
		t.Fatalf("bidder=%s min=%d", view.Auction.CurrentBidderUserID, view.Auction.MinBid)
	}
	if view.CanEndTurn || view.TurnStartedAt != "" {
		t.Fatalf("banks should pause; canEnd=%v started=%q", view.CanEndTurn, view.TurnStartedAt)
	}

	view, err = svc.AuctionBid(context.Background(), "g1", "a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if view.Auction == nil || view.Auction.HighBid != 10 || view.Auction.HighBidderUserID != "a" {
		t.Fatalf("after bid: %+v", view.Auction)
	}
	if view.Auction.CurrentBidderUserID != "b" {
		t.Fatalf("next bidder=%s want b", view.Auction.CurrentBidderUserID)
	}

	view, err = svc.AuctionFold(context.Background(), "g1", "b")
	if err != nil {
		t.Fatal(err)
	}
	if view.Auction != nil {
		t.Fatal("auction should clear after last fold")
	}
	if view.LastAuction == nil || view.LastAuction.Void || view.LastAuction.Amount != 10 {
		t.Fatalf("lastAuction=%+v", view.LastAuction)
	}
	if view.LastAuction.WinnerUserID != "a" {
		t.Fatalf("winner=%s", view.LastAuction.WinnerUserID)
	}
	if len(view.Deeds) != 1 || view.Deeds[0].OwnerUserID != "a" {
		t.Fatalf("deeds=%+v", view.Deeds)
	}
	if view.Players[0].Cash != 1990 {
		t.Fatalf("cash=%d want 1990", view.Players[0].Cash)
	}
	if !view.CanEndTurn {
		t.Fatal("lander should end turn after auction")
	}
}

func TestAuctionAllFoldNoBidsAwardsOneMeetCoin(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBuyOffer(t, repo, 2000, 2000)

	if _, err := svc.StartAuction(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	view, err := svc.AuctionFold(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	// A folded with no bids → only B left → B wins at 1
	if view.Auction != nil {
		t.Fatal("expected settle")
	}
	if view.LastAuction == nil || view.LastAuction.WinnerUserID != "b" || view.LastAuction.Amount != 1 {
		t.Fatalf("lastAuction=%+v", view.LastAuction)
	}
	if view.Players[1].Cash != 1999 {
		t.Fatalf("b cash=%d", view.Players[1].Cash)
	}
}

func TestAuctionVoidWhenWinnerCannotPayOne(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBuyOffer(t, repo, 2000, 0)

	if _, err := svc.StartAuction(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	view, err := svc.AuctionFold(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.LastAuction == nil || !view.LastAuction.Void {
		t.Fatalf("want void, got %+v", view.LastAuction)
	}
	if len(view.Deeds) != 0 {
		t.Fatalf("deeds=%+v", view.Deeds)
	}
	if !view.CanEndTurn {
		t.Fatal("canEnd after void")
	}
}

func TestAutoStartAuctionWhenCannotAfford(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 50
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	g.LastRoll = &gamerepo.LastRoll{
		UserID: "a", Username: "A", Die1: 1, Die2: 0, Total: 1,
		FromIndex: 0, ToIndex: 1,
	}
	svc.resolveLandingLocked(context.Background(), g, 0, 1)
	_ = repo.Update(context.Background(), g)

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if view.BuyOffer != nil {
		t.Fatal("broke lander must not see buyOffer")
	}
	if view.Auction == nil || view.Auction.BoardIndex != 1 {
		t.Fatalf("auction=%+v", view.Auction)
	}
}

func TestAuctionBidTooLowAndNotYourTurn(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBuyOffer(t, repo, 2000, 2000)

	if _, err := svc.StartAuction(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuctionBid(context.Background(), "g1", "b", 5); !errors.Is(err, ErrNotAuctionTurn) {
		t.Fatalf("err=%v", err)
	}
	if _, err := svc.AuctionBid(context.Background(), "g1", "a", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuctionBid(context.Background(), "g1", "b", 10); !errors.Is(err, ErrBidTooLow) {
		t.Fatalf("err=%v", err)
	}
}

func TestAuctionHighBidderSitsOutUntilOutbid(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	// need 3 players for sit-out clarity
	bank := gamerepo.TimeBankDuration.Milliseconds()
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players = append(g.Players, gamerepo.Player{
		UserID: "c", Username: "C", SeatIndex: 2, TurnOrder: 2, Cash: 2000,
		BoardIndex: 0, PinColor: "#00f", TimeRemainingMs: bank,
	})
	_ = repo.Update(context.Background(), g)
	seedBuyOffer(t, repo, 2000, 2000)

	if _, err := svc.StartAuction(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	view, err := svc.AuctionBid(context.Background(), "g1", "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	if view.Auction.CurrentBidderUserID != "b" {
		t.Fatalf("want b, got %s", view.Auction.CurrentBidderUserID)
	}
	view, err = svc.AuctionBid(context.Background(), "g1", "b", 6)
	if err != nil {
		t.Fatal(err)
	}
	// high is b — next should be c (skip b), not a yet... a is not high, so after b, next is c then a
	if view.Auction.CurrentBidderUserID != "c" {
		t.Fatalf("want c (skip high b), got %s", view.Auction.CurrentBidderUserID)
	}
	view, err = svc.AuctionFold(context.Background(), "g1", "c")
	if err != nil {
		t.Fatal(err)
	}
	if view.Auction.CurrentBidderUserID != "a" {
		t.Fatalf("want a after c fold, got %s", view.Auction.CurrentBidderUserID)
	}
}

func TestProposeTradeAndAcceptDeedForCash(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "a"},
		{BoardIndex: 3, OwnerUserID: "b"},
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{BoardIndexes: []int{1}},
		TradeSideInput{Cash: 50},
	)
	if err != nil {
		t.Fatal(err)
	}
	if view.Trade == nil || view.Trade.ToUserID != "b" {
		t.Fatalf("trade=%v", view.Trade)
	}
	if view.CanRoll || view.TurnStartedAt != "" {
		t.Fatalf("clock paused / roll blocked: canRoll=%v started=%q", view.CanRoll, view.TurnStartedAt)
	}

	view, err = svc.AcceptTrade(context.Background(), "g1", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Trade != nil {
		t.Fatal("trade should clear")
	}
	if view.LastTrade == nil || view.LastTrade.Outcome != TradeOutcomeAccepted {
		t.Fatalf("lastTrade=%+v", view.LastTrade)
	}
	if view.LastTrade.FromUserID != "a" || view.LastTrade.ToUserID != "b" {
		t.Fatalf("lastTrade parties=%+v", view.LastTrade)
	}
	own := map[int]string{}
	for _, d := range view.Deeds {
		own[d.BoardIndex] = d.OwnerUserID
	}
	if own[1] != "b" || own[3] != "b" {
		t.Fatalf("deeds=%v", own)
	}
	if view.Players[0].Cash != 2050 || view.Players[1].Cash != 1950 {
		t.Fatalf("cash a=%d b=%d", view.Players[0].Cash, view.Players[1].Cash)
	}
}

func TestProposeTradeRejectsCashForCash(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	_, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{Cash: 10},
		TradeSideInput{Cash: 20},
	)
	if !errors.Is(err, ErrTradeNeedsDeed) {
		t.Fatalf("err=%v", err)
	}
}

func TestDeclineTradeResumesClock(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a"}}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	_ = repo.Update(context.Background(), g)

	if _, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{BoardIndexes: []int{1}},
		TradeSideInput{Cash: 1},
	); err != nil {
		t.Fatal(err)
	}
	view, err := svc.DeclineTrade(context.Background(), "g1", "b")
	if err != nil {
		t.Fatal(err)
	}
	if view.Trade != nil {
		t.Fatal("expected nil trade")
	}
	if view.LastTrade == nil || view.LastTrade.Outcome != TradeOutcomeDeclined {
		t.Fatalf("lastTrade=%+v", view.LastTrade)
	}
	if view.TurnStartedAt == "" {
		t.Fatal("expected clock resumed")
	}
}

func TestAcceptTradeRedeemAllMortgage(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a", Mortgaged: true}}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	if _, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{BoardIndexes: []int{1}},
		TradeSideInput{Cash: 10},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptTrade(context.Background(), "g1", "b", ""); !errors.Is(err, ErrTradeMortgageChoice) {
		t.Fatalf("err=%v", err)
	}
	view, err := svc.AcceptTrade(context.Background(), "g1", "b", MortgageRedeemAll)
	if err != nil {
		t.Fatal(err)
	}
	// redeemCost(60) = 30+3 = 33; b pays 10 cash + 33 redeem, receives deed unmortgaged
	if view.Players[1].Cash != 2000-10-33 {
		t.Fatalf("b cash=%d", view.Players[1].Cash)
	}
	for _, d := range view.Deeds {
		if d.BoardIndex == 1 {
			if d.OwnerUserID != "b" || d.Mortgaged {
				t.Fatalf("deed=%+v", d)
			}
		}
	}
}

func TestProposeTradeBlockedDuringAuction(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	seedBuyOffer(t, repo, 2000, 2000)
	if _, err := svc.StartAuction(context.Background(), "g1", "a"); err != nil {
		t.Fatal(err)
	}
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].GetOutOfJailFree = 1
	g.Players[0].GetOutOfJailFreeCards = []string{"chance_get_out_of_jail"}
	_ = repo.Update(context.Background(), g)
	_, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{GetOutOfJailFree: 1},
		TradeSideInput{Cash: 5},
	)
	if !errors.Is(err, ErrAuctionActive) {
		t.Fatalf("err=%v", err)
	}
}

func TestBankruptWipesAssetsAndPaysCreditor(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, ColorGroup: "brown", HouseCost: 50},
		{BoardIndex: 3, Slug: "accra", Name: "Accra", Kind: "property", Price: 60, ColorGroup: "brown", HouseCost: 50},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -80
	g.Players[0].GetOutOfJailFree = 1
	g.Players[0].GetOutOfJailFreeCards = []string{CardChanceGetOutOfJail}
	g.Players[1].Cash = 2000
	g.Deeds = []gamerepo.Deed{
		{BoardIndex: 1, OwnerUserID: "a", Houses: 2},
		{BoardIndex: 3, OwnerUserID: "a", Houses: 1},
	}
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 80, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.ChanceDeck = []string{CardChanceDividend}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err := svc.Bankrupt(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Players[0].Resigned {
		t.Fatal("a should be resigned")
	}
	if view.Players[0].Cash != 0 {
		t.Fatalf("a cash=%d", view.Players[0].Cash)
	}
	// Bank pays remaining 80 to b.
	if view.Players[1].Cash != 2080 {
		t.Fatalf("b cash=%d want 2080", view.Players[1].Cash)
	}
	if len(view.Deeds) != 0 {
		t.Fatalf("deeds should be wiped, got %d", len(view.Deeds))
	}
	if view.PendingPayment != nil {
		t.Fatal("pending should clear")
	}
	if view.LastBankruptcy == nil || view.LastBankruptcy.Reason != "declare" || view.LastBankruptcy.BankPaid != 80 {
		t.Fatalf("lastBankruptcy=%+v", view.LastBankruptcy)
	}
	g2, _ := repo.FindByID(context.Background(), "g1")
	if len(g2.ChanceDeck) == 0 || g2.ChanceDeck[len(g2.ChanceDeck)-1] != CardChanceGetOutOfJail {
		t.Fatalf("GOOJF should return to chance bottom: %v", g2.ChanceDeck)
	}
	if view.Status != gamerepo.StatusFinished || view.WinnerUserID != "b" {
		t.Fatalf("status=%s winner=%s", view.Status, view.WinnerUserID)
	}
}

func TestBankruptRequiresNegativeCash(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, memSpaces{}, Config{})
	seedTwoPlayer(t, repo)
	_, err := svc.Bankrupt(context.Background(), "g1", "a")
	if !errors.Is(err, ErrNotInDebt) {
		t.Fatalf("err=%v want ErrNotInDebt", err)
	}
}

func TestDebtPayStartAndSettle(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, brownBuildSpaces(), Config{})
	seedTwoPlayer(t, repo)
	seedBrownMonopoly(t, repo, "a", 1, 1)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -20
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "rent", Amount: 20, FromUserID: "a", ToUserID: "b", BoardIndex: 5, SpaceName: "Debt",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	g.TurnStartedAt = time.Time{}
	_ = repo.Update(context.Background(), g)

	view, err := svc.StartDebtPay(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.DebtPay == nil || view.DebtPay.UserID != "a" {
		t.Fatalf("debtPay=%+v", view.DebtPay)
	}
	if view.TurnStartedAt != "" {
		t.Fatal("turn clock should be paused during debt pay")
	}
	if view.CanProposeTrade {
		t.Fatal("trade blocked during debt")
	}

	view, err = svc.SellBuilding(context.Background(), "g1", "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if view.Players[0].Cash < 0 {
		t.Fatalf("cash should clear debt, got %d", view.Players[0].Cash)
	}
	if view.DebtPay != nil {
		t.Fatal("debt pay should clear after settle")
	}
	if view.PendingPayment != nil {
		t.Fatal("pending should clear")
	}
}

func TestDebtPayAutoBankruptWhenNoAssets(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, memSpaces{}, Config{})
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -50
	g.PendingPayment = &gamerepo.PendingPayment{
		Kind: "tax", Amount: 50, FromUserID: "a", ToUserID: "", BoardIndex: 4, SpaceName: "Tax",
	}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingRoll
	_ = repo.Update(context.Background(), g)

	view, err := svc.StartDebtPay(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Players[0].Resigned {
		t.Fatal("should auto-bankrupt with nothing to raise")
	}
	if view.LastBankruptcy == nil || view.LastBankruptcy.Reason != "auto_insolvent" {
		t.Fatalf("lastBankruptcy=%+v", view.LastBankruptcy)
	}
}

func TestResignWipesDeedsToBank(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a", Houses: 3}}
	g.Players[0].GetOutOfJailFreeCards = []string{CardChestGetOutOfJail}
	g.Players[0].GetOutOfJailFree = 1
	g.ChestDeck = []string{CardChestBankError}
	_ = repo.Update(context.Background(), g)

	view, err := svc.Resign(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Players[0].Resigned || len(view.Deeds) != 0 {
		t.Fatalf("resigned=%v deeds=%d", view.Players[0].Resigned, len(view.Deeds))
	}
	g2, _ := repo.FindByID(context.Background(), "g1")
	if len(g2.ChestDeck) < 2 || g2.ChestDeck[len(g2.ChestDeck)-1] != CardChestGetOutOfJail {
		t.Fatalf("chestDeck=%v", g2.ChestDeck)
	}
	if view.LastBankruptcy == nil || view.LastBankruptcy.Reason != "resign" {
		t.Fatalf("lastBankruptcy=%+v", view.LastBankruptcy)
	}
}

func TestTradeBlockedWhileInDebt(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces, Config{})
	seedTwoPlayer(t, repo)
	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].Cash = -10
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "a"}}
	g.PendingPayment = &gamerepo.PendingPayment{Kind: "rent", Amount: 10, FromUserID: "a", ToUserID: "b"}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	_, err := svc.ProposeTrade(context.Background(), "g1", "a", "b",
		TradeSideInput{BoardIndexes: []int{1}},
		TradeSideInput{Cash: 5},
	)
	if !errors.Is(err, ErrMustSettle) {
		t.Fatalf("err=%v want ErrMustSettle", err)
	}
}

func TestTurnStartInsolventAutoBankrupts(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60, Rents: []int{100}},
	}
	svc := New(repo, spaces, Config{}).(*service)
	seedTwoPlayer(t, repo)

	g, _ := repo.FindByID(context.Background(), "g1")
	g.Players[0].BoardIndex = 1
	g.Players[0].Cash = 40
	g.Deeds = []gamerepo.Deed{{BoardIndex: 1, OwnerUserID: "b"}}
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	svc.resolveLandingLocked(context.Background(), g, 0, 5)
	_ = repo.Update(context.Background(), g)

	// A ends with debt and no deeds → on B's turn OK; when turn returns to A with no assets → bankrupt.
	// Give A no deeds (already none). End turn to B, then force A's turn start via EndTurn from B after stubbing.
	view, err := svc.EndTurn(context.Background(), "g1", "a")
	if err != nil {
		t.Fatal(err)
	}
	if view.CurrentUserID != "b" {
		t.Fatalf("current=%s", view.CurrentUserID)
	}
	g, _ = repo.FindByID(context.Background(), "g1")
	g.TurnPhase = gamerepo.TurnPhaseAwaitingEnd
	_ = repo.Update(context.Background(), g)

	view, err = svc.EndTurn(context.Background(), "g1", "b")
	if err != nil {
		t.Fatal(err)
	}
	// A becomes current with cash < 0 and no raisable assets → auto_insolvent.
	if !view.Players[0].Resigned {
		t.Fatal("a should auto-bankrupt on turn start")
	}
	if view.LastBankruptcy == nil || view.LastBankruptcy.Reason != "auto_insolvent" {
		t.Fatalf("lastBankruptcy=%+v", view.LastBankruptcy)
	}
}
