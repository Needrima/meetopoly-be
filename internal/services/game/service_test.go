package game

import (
	"context"
	"testing"

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
	if g.LastRoll != nil {
		lr := *g.LastRoll
		cp.LastRoll = &lr
	}
	return &cp
}

func seedTwoPlayer(t *testing.T, repo *memRepo) {
	t.Helper()
	g := &gamerepo.Game{
		ID:      "g1",
		TableID: "t1",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 0, PinColor: "#f00"},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0"},
		},
		CurrentTurn:   0,
		PassGoBonus:   200,
		TurnPhase:     gamerepo.TurnPhaseAwaitingRoll,
		DoublesStreak: 0,
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}
}

func TestRollDoesNotAdvanceTurn_EndTurnDoes(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo)
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
	svc := New(repo).(*service)
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
	svc := New(repo)
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
