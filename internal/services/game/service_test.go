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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, nil)
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
	svc := New(repo, spaces)
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
	svc := New(repo, spaces)
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

func TestBuyAlreadyOwned(t *testing.T) {
	repo := newMemRepo()
	spaces := memSpaces{
		{BoardIndex: 1, Slug: "lagos", Name: "Lagos", Kind: "property", Price: 60},
	}
	svc := New(repo, spaces)
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
