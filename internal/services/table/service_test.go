package table

import (
	"context"
	"errors"
	"strings"
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
		if t.WorldID != worldID || t.Status != tablerepo.StatusLobby || t.Private {
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

func (m *memRepo) FindByInviteCode(_ context.Context, inviteCode string) (*tablerepo.Table, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tables {
		if t.InviteCode == inviteCode {
			cp := *t
			cp.Seats = append([]tablerepo.Seat(nil), t.Seats...)
			return &cp, nil
		}
	}
	return nil, tablerepo.ErrNotFound
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

func TestJoinAssignsUniquePinColors(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	a, err := svc.Join(context.Background(), "user-a", "A", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Join(context.Background(), "user-b", "B", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("expected same table %s vs %s", a.ID, b.ID)
	}
	var colors []string
	for _, s := range b.Seats {
		if s.UserID == nil {
			continue
		}
		if s.PinColor == nil || *s.PinColor == "" {
			t.Fatalf("seat %d missing pinColor", s.SeatIndex)
		}
		colors = append(colors, strings.ToLower(*s.PinColor))
	}
	if len(colors) != 2 {
		t.Fatalf("occupied=%d", len(colors))
	}
	if colors[0] == colors[1] {
		t.Fatalf("duplicate colors %v", colors)
	}
}

func TestCreatePrivateHasInviteCodeAndExcludesFromPublicJoin(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	priv, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Private {
		t.Fatal("expected private")
	}
	if priv.InviteCode == nil || len(*priv.InviteCode) != 8 {
		t.Fatalf("inviteCode=%v", priv.InviteCode)
	}
	code := *priv.InviteCode
	for _, r := range code {
		if !strings.ContainsRune(inviteAlphabet, r) {
			t.Fatalf("invite code has invalid rune %q in %q", r, code)
		}
	}

	pub, err := svc.Join(context.Background(), "guest", "Guest", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if pub.ID == priv.ID {
		t.Fatal("public Join seated into private table")
	}
	if pub.Private {
		t.Fatal("public table marked private")
	}
	if pub.InviteCode != nil {
		t.Fatalf("public inviteCode=%v", pub.InviteCode)
	}
}

func TestCreatePrivateLeavesExistingLobby(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	first, err := svc.Join(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("expected a new private table")
	}
	stored, err := repo.FindByID(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tablerepo.OccupiedCount(stored) != 0 {
		t.Fatalf("old public lobby still occupied")
	}
}

func TestNormalizeInviteCode(t *testing.T) {
	if got := normalizeInviteCode("a1b2c3d4"); got != "A1B2C3D4" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeInviteCode("A1B2C3D4"); got != "A1B2C3D4" {
		t.Fatalf("got %q", got)
	}
	if normalizeInviteCode("short") != "" {
		t.Fatal("expected empty for short")
	}
	if normalizeInviteCode("IIIIIIII") != "" {
		t.Fatal("expected empty for I (not in Crockford alphabet)")
	}
}

func TestJoinByInviteCodeHappyPath(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	host, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	code := *host.InviteCode

	guest, err := svc.JoinByInviteCode(context.Background(), "guest", "Guest", strings.ToLower(code))
	if err != nil {
		t.Fatal(err)
	}
	if guest.ID != host.ID {
		t.Fatalf("table %s want %s", guest.ID, host.ID)
	}
	occupied := 0
	for _, s := range guest.Seats {
		if s.UserID != nil {
			occupied++
		}
	}
	if occupied != 2 {
		t.Fatalf("occupied=%d want 2", occupied)
	}
}

func TestJoinByInviteCodeRejectsSealed(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{DisconnectHold: time.Second}).(*service)
	svc.SetGameStarter(stubStarter{id: "game-1"})

	host, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	code := *host.InviteCode
	if _, err := svc.JoinByInviteCode(context.Background(), "guest", "Guest", code); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetReady(context.Background(), host.ID, "host", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetReady(context.Background(), host.ID, "guest", true); err != nil {
		t.Fatal(err)
	}

	_, err = svc.JoinByInviteCode(context.Background(), "late", "Late", code)
	if !errors.Is(err, ErrWrongStatus) {
		t.Fatalf("err=%v want ErrWrongStatus", err)
	}
}

func TestJoinByInviteCodeRejectsFull(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	host, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	code := *host.InviteCode
	for i := 1; i < tablerepo.MaxSeats; i++ {
		uid := "u" + string(rune('0'+i))
		if _, err := svc.JoinByInviteCode(context.Background(), uid, "P"+uid, code); err != nil {
			t.Fatalf("seat %d: %v", i, err)
		}
	}
	_, err = svc.JoinByInviteCode(context.Background(), "overflow", "Overflow", code)
	if !errors.Is(err, ErrFull) {
		t.Fatalf("err=%v want ErrFull", err)
	}
}

func TestJoinByInviteCodeInvalidAndUnknown(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	if _, err := svc.JoinByInviteCode(context.Background(), "a", "A", "bad"); !errors.Is(err, ErrInvalidInviteCode) {
		t.Fatalf("err=%v want ErrInvalidInviteCode", err)
	}
	if _, err := svc.JoinByInviteCode(context.Background(), "a", "A", "ABCDEFGH"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestJoinByInviteCodeResumesSeat(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, Config{})

	host, err := svc.CreatePrivate(context.Background(), "host", "Host", "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	code := *host.InviteCode
	again, err := svc.JoinByInviteCode(context.Background(), "host", "Host2", code)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != host.ID {
		t.Fatalf("expected same table")
	}
	occupied := 0
	for _, s := range again.Seats {
		if s.UserID != nil {
			occupied++
		}
	}
	if occupied != 1 {
		t.Fatalf("occupied=%d want 1", occupied)
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
