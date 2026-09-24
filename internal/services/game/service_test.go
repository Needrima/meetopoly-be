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
	cp.Deeds = append([]gamerepo.Deed(nil), g.Deeds...)
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
	if view.Players[0].BoardIndex != 10 {
		t.Fatalf("should not move on third doubles, got %d", view.Players[0].BoardIndex)
	}
	if !view.CanEndTurn {
		t.Fatal("must end after third doubles")
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

func TestTimeBankExhaustedEliminatesPlayer(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, Config{})
	bank := gamerepo.TimeBankDuration.Milliseconds()
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
	if !view.Players[0].Resigned {
		t.Fatal("a should be eliminated")
	}
	if view.Status != gamerepo.StatusFinished {
		t.Fatalf("status=%s want finished (last player wins)", view.Status)
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
	if view.CanBuy {
		t.Fatal("should not canBuy after purchase")
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

func TestCannotAffordRentBlocksEnd(t *testing.T) {
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

	if g.Players[0].Cash != 0 || g.Players[1].Cash != 2040 {
		t.Fatalf("cash a=%d b=%d", g.Players[0].Cash, g.Players[1].Cash)
	}
	if g.PendingPayment == nil || g.PendingPayment.Amount != 60 {
		t.Fatalf("pending=%v", g.PendingPayment)
	}

	view, err := svc.Get(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if view.CanEndTurn || view.CanRoll {
		t.Fatalf("canEnd=%v canRoll=%v", view.CanEndTurn, view.CanRoll)
	}

	_, err = svc.EndTurn(context.Background(), "g1", "a")
	if !errors.Is(err, ErrMustSettle) {
		t.Fatalf("err=%v", err)
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
