package table

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tablerepo "meetopoly-be/internal/repository/table"
)

type memRepo struct {
	mu     sync.Mutex
	tables map[string]*tablerepo.Table
}

func newMemRepo() *memRepo {
	return &memRepo{tables: make(map[string]*tablerepo.Table)}
}

func (m *memRepo) EnsureIndexes(context.Context) error { return nil }

func (m *memRepo) Insert(_ context.Context, t *tablerepo.Table) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *t
	cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
	m.tables[t.ID] = &cp
	return nil
}

func (m *memRepo) Update(_ context.Context, t *tablerepo.Table) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tables[t.ID]; !ok {
		return tablerepo.ErrNotFound
	}
	cp := *t
	cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
	cp.UpdatedAt = time.Now().UTC()
	m.tables[t.ID] = &cp
	return nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*tablerepo.Table, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tables[id]
	if !ok {
		return nil, tablerepo.ErrNotFound
	}
	cp := *t
	cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
	return &cp, nil
}

func (m *memRepo) FindOpenLobby(_ context.Context, worldID string) (*tablerepo.Table, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *tablerepo.Table
	for _, t := range m.tables {
		if t.WorldID != worldID || t.Status != tablerepo.StatusLobby {
			continue
		}
		if tablerepo.OccupiedCount(t) >= tablerepo.MaxSeats {
			continue
		}
		cp := *t
		cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
		if best == nil || cp.UpdatedAt.Before(best.UpdatedAt) {
			best = &cp
		}
	}
	if best == nil {
		return nil, tablerepo.ErrNotFound
	}
	return best, nil
}

func (m *memRepo) FindLobbyByUser(_ context.Context, userID string) (*tablerepo.Table, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *tablerepo.Table
	for _, t := range m.tables {
		if t.Status != tablerepo.StatusLobby && t.Status != tablerepo.StatusStarting {
			continue
		}
		seated := false
		for _, s := range t.Seats {
			if s.UserID == userID {
				seated = true
				break
			}
		}
		if !seated {
			continue
		}
		cp := *t
		cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
		if best == nil || cp.UpdatedAt.After(best.UpdatedAt) {
			best = &cp
		}
	}
	if best == nil {
		return nil, tablerepo.ErrNotFound
	}
	return best, nil
}

type stubStarter struct {
	id  string
	err error
}

func (s stubStarter) StartFromTable(context.Context, string, string, []SeatView) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.id, nil
}

func TestJoinAbandonsBrokenStartingAndMatches(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{DisconnectHold: time.Second}).(*service)

	broken := newEmptyTable("africa-1")
	broken.Status = tablerepo.StatusStarting
	broken.Seats[0].UserID = "user-a"
	broken.Seats[0].Username = "Needrima"
	if err := repo.Insert(context.Background(), broken); err != nil {
		t.Fatal(err)
	}

	open := newEmptyTable("africa-1")
	open.Seats[0].UserID = "user-b"
	open.Seats[0].Username = "ademola"
	if err := repo.Insert(context.Background(), open); err != nil {
		t.Fatal(err)
	}

	view, err := svc.Join(context.Background(), "user-a", "Needrima", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != open.ID {
		t.Fatalf("expected join open lobby %s, got %s", open.ID, view.ID)
	}
	if view.Status != tablerepo.StatusLobby {
		t.Fatalf("status=%s", view.Status)
	}
	occupied := 0
	for _, s := range view.Seats {
		if s.UserID != nil {
			occupied++
		}
	}
	if occupied != 2 {
		t.Fatalf("occupied=%d want 2", occupied)
	}

	storedBroken, err := repo.FindByID(context.Background(), broken.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedBroken.Status != tablerepo.StatusLobby {
		t.Fatalf("broken table status=%s want lobby", storedBroken.Status)
	}
	if tablerepo.OccupiedCount(storedBroken) != 0 {
		t.Fatalf("broken table still occupied")
	}
}

func TestSetReadyCreatesGameWithoutLeavingStarting(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{DisconnectHold: time.Second}).(*service)
	svc.SetGameStarter(stubStarter{id: "game-1"})

	tbl := newEmptyTable("africa-1")
	tbl.Seats[0].UserID = "a"
	tbl.Seats[0].Username = "A"
	tbl.Seats[0].Ready = true
	tbl.Seats[1].UserID = "b"
	tbl.Seats[1].Username = "B"
	if err := repo.Insert(context.Background(), tbl); err != nil {
		t.Fatal(err)
	}

	view, err := svc.SetReady(context.Background(), tbl.ID, "b", true)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != tablerepo.StatusInGame {
		t.Fatalf("status=%s want in_game", view.Status)
	}
	if view.GameID == nil || *view.GameID != "game-1" {
		t.Fatalf("gameId=%v", view.GameID)
	}
}

func TestSetReadyGameFailKeepsLobby(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{DisconnectHold: time.Second}).(*service)
	svc.SetGameStarter(stubStarter{err: errors.New("boom")})

	tbl := newEmptyTable("africa-1")
	tbl.Seats[0].UserID = "a"
	tbl.Seats[0].Username = "A"
	tbl.Seats[0].Ready = true
	tbl.Seats[1].UserID = "b"
	tbl.Seats[1].Username = "B"
	if err := repo.Insert(context.Background(), tbl); err != nil {
		t.Fatal(err)
	}

	view, err := svc.SetReady(context.Background(), tbl.ID, "b", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if view == nil || view.Status != tablerepo.StatusLobby {
		t.Fatalf("view=%v", view)
	}
	stored, _ := repo.FindByID(context.Background(), tbl.ID)
	if stored.Status != tablerepo.StatusLobby {
		t.Fatalf("persisted status=%s", stored.Status)
	}
	if stored.GameID != "" {
		t.Fatalf("gameId set")
	}
}
