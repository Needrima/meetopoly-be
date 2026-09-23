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

func TestRollMovesPinAwardsPassGoAndAdvancesTurn(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo)

	g := &gamerepo.Game{
		ID:      "g1",
		TableID: "t1",
		WorldID: "africa-1",
		Status:  gamerepo.StatusActive,
		Players: []gamerepo.Player{
			{UserID: "a", Username: "A", SeatIndex: 0, TurnOrder: 0, Cash: 2000, BoardIndex: 38, PinColor: "#f00"},
			{UserID: "b", Username: "B", SeatIndex: 1, TurnOrder: 1, Cash: 2000, BoardIndex: 0, PinColor: "#0f0"},
		},
		CurrentTurn: 0,
		PassGoBonus: 200,
	}
	if err := repo.Insert(context.Background(), g); err != nil {
		t.Fatal(err)
	}

	// Force deterministic dice by rolling many times until we get a known path —
	// instead patch via multiple rolls checking invariants.
	var view *View
	var err error
	for i := 0; i < 40; i++ {
		// Reset turn to A before each attempt by re-inserting state if needed.
		cur, _ := repo.FindByID(context.Background(), "g1")
		cur.CurrentTurn = 0
		cur.Players[0].BoardIndex = 38
		cur.Players[0].Cash = 2000
		cur.Players[1].BoardIndex = 0
		_ = repo.Update(context.Background(), cur)

		view, err = svc.Roll(context.Background(), "g1", "a")
		if err != nil {
			t.Fatal(err)
		}
		if view.LastRoll == nil {
			t.Fatal("missing lastRoll")
		}
		if view.LastRoll.FromIndex != 38 {
			t.Fatalf("from=%d", view.LastRoll.FromIndex)
		}
		if view.LastRoll.Total < 2 || view.LastRoll.Total > 12 {
			t.Fatalf("total=%d", view.LastRoll.Total)
		}
		wantTo := (38 + view.LastRoll.Total) % 40
		if view.LastRoll.ToIndex != wantTo {
			t.Fatalf("to=%d want %d", view.LastRoll.ToIndex, wantTo)
		}
		passed := 38+view.LastRoll.Total >= 40
		if view.LastRoll.PassedGo != passed {
			t.Fatalf("passedGo=%v want %v", view.LastRoll.PassedGo, passed)
		}
		a := view.Players[0]
		if a.BoardIndex != wantTo {
			t.Fatalf("pin=%d", a.BoardIndex)
		}
		wantCash := 2000
		if passed {
			wantCash = 2200
		}
		if a.Cash != wantCash {
			t.Fatalf("cash=%d want %d", a.Cash, wantCash)
		}
		if view.CurrentUserID != "b" {
			t.Fatalf("turn=%s want b", view.CurrentUserID)
		}
		// Prove not-your-turn
		if _, err := svc.Roll(context.Background(), "g1", "a"); err == nil {
			t.Fatal("expected not your turn")
		}
		return
	}
}
