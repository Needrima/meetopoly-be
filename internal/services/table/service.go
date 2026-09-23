package table

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	tablerepo "meetopoly-be/internal/repository/table"
)

type service struct {
	repo    tablerepo.Repository
	cfg     Config
	bcast   Broadcaster
	gameStarter GameStarter
	mu      sync.Mutex
	holds   map[string]*time.Timer
}

// New builds a table Service.
func New(repo tablerepo.Repository, cfg Config) Service {
	if cfg.DisconnectHold <= 0 {
		cfg.DisconnectHold = 45 * time.Second
	}
	return &service{
		repo:  repo,
		cfg:   cfg,
		holds: make(map[string]*time.Timer),
	}
}

func (s *service) SetBroadcaster(b Broadcaster) {
	s.bcast = b
}

func (s *service) SetGameStarter(g GameStarter) {
	s.gameStarter = g
}

func (s *service) Join(ctx context.Context, userID, username, worldID string) (*View, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, ErrInvalidWorld
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "Player"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Resume existing lobby seat (reconnect / remount), unless the table is a
	// broken half-start (status starting, no game) — those block matchmaking.
	existing, err := s.repo.FindLobbyByUser(ctx, userID)
	if err == nil && existing.WorldID == worldID {
		if isBrokenMatchmakingTable(existing) {
			_, _ = s.leaveLocked(ctx, existing, userID)
			existing = nil
		} else {
			s.clearHoldLocked(existing.ID, userID)
			for i := range existing.Seats {
				if existing.Seats[i].UserID == userID {
					existing.Seats[i].Holding = false
					existing.Seats[i].HoldEndsAt = nil
					existing.Seats[i].Username = username
				}
			}
			if err := s.repo.Update(ctx, existing); err != nil {
				return nil, err
			}
			view := toView(existing)
			s.broadcast(existing.ID, Event{Type: "state", Table: view})
			return view, nil
		}
	}
	if err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}
	// Seated on another world lobby — leave it first.
	if existing != nil {
		_, _ = s.leaveLocked(ctx, existing, userID)
	}

	t, err := s.repo.FindOpenLobby(ctx, worldID)
	if err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}
	if errors.Is(err, tablerepo.ErrNotFound) {
		t = newEmptyTable(worldID)
		if err := seatUser(t, userID, username); err != nil {
			return nil, err
		}
		if err := s.repo.Insert(ctx, t); err != nil {
			return nil, err
		}
		view := toView(t)
		s.broadcast(t.ID, Event{Type: "state", Table: view})
		return view, nil
	}

	if err := seatUser(t, userID, username); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := toView(t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})
	return view, nil
}

func (s *service) Get(ctx context.Context, tableID string) (*View, error) {
	t, err := s.repo.FindByID(ctx, tableID)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return toView(t), nil
}

func (s *service) SetReady(ctx context.Context, tableID, userID string, ready bool) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.repo.FindByID(ctx, tableID)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.Status != tablerepo.StatusLobby {
		return nil, ErrWrongStatus
	}
	if tablerepo.OccupiedCount(t) < tablerepo.MinSeats {
		return nil, ErrNeedPlayers
	}

	seat := findSeat(t, userID)
	if seat == nil {
		return nil, ErrNotSeated
	}
	if seat.Holding {
		return nil, ErrNotSeated
	}
	seat.Ready = ready

	// Never persist status=starting without a gameId — that orphans the table
	// from FindOpenLobby while FindLobbyByUser keeps resuming it (solo lobbies).
	started := false
	if ready && everyoneReady(t) && t.GameID == "" {
		if s.gameStarter == nil {
			return nil, ErrWrongStatus
		}
		viewSeats := toView(t).Seats
		gameID, err := s.gameStarter.StartFromTable(ctx, t.ID, t.WorldID, viewSeats)
		if err != nil {
			// Keep lobby + Ready flags; do not leave status stuck on starting.
			if updErr := s.repo.Update(ctx, t); updErr != nil {
				return nil, updErr
			}
			view := toView(t)
			s.broadcast(t.ID, Event{Type: "state", Table: view})
			return view, err
		}
		t.GameID = gameID
		t.Status = tablerepo.StatusInGame
		started = true
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := toView(t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})
	if started {
		s.broadcast(t.ID, Event{Type: "started", Table: view})
	}
	return view, nil
}

func (s *service) Leave(ctx context.Context, tableID, userID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.repo.FindByID(ctx, tableID)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.leaveLocked(ctx, t, userID)
}

func (s *service) leaveLocked(ctx context.Context, t *tablerepo.Table, userID string) (*View, error) {
	s.clearHoldLocked(t.ID, userID)
	cleared := false
	for i := range t.Seats {
		if t.Seats[i].UserID == userID {
			t.Seats[i] = emptySeat(t.Seats[i].SeatIndex)
			cleared = true
		}
	}
	if !cleared {
		return toView(t), nil
	}
	normalizeMatchmakingStatus(t)
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := toView(t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})
	return view, nil
}

// isBrokenMatchmakingTable is a half-started lobby with no game — resume would
// isolate the player from FindOpenLobby (status != lobby).
func isBrokenMatchmakingTable(t *tablerepo.Table) bool {
	if t == nil {
		return false
	}
	if t.GameID != "" {
		return false
	}
	if t.Status == tablerepo.StatusStarting {
		return true
	}
	return false
}

// normalizeMatchmakingStatus resets ready-gate and pulls stuck starting tables
// (no game yet) back into the joinable lobby pool.
func normalizeMatchmakingStatus(t *tablerepo.Table) {
	if tablerepo.OccupiedCount(t) < tablerepo.MinSeats {
		for i := range t.Seats {
			t.Seats[i].Ready = false
		}
	}
	if t.GameID == "" && t.Status != tablerepo.StatusLobby {
		t.Status = tablerepo.StatusLobby
	}
}

func (s *service) Disconnect(ctx context.Context, tableID, userID string) (*View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.repo.FindByID(ctx, tableID)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.Status != tablerepo.StatusLobby {
		return toView(t), nil
	}
	seat := findSeat(t, userID)
	if seat == nil {
		return toView(t), nil
	}
	ends := time.Now().UTC().Add(s.cfg.DisconnectHold)
	seat.Holding = true
	seat.HoldEndsAt = &ends
	seat.Ready = false

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := toView(t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})

	s.clearHoldLocked(tableID, userID)
	key := holdKey(tableID, userID)
	s.holds[key] = time.AfterFunc(s.cfg.DisconnectHold, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.holds, key)
		cur, err := s.repo.FindByID(context.Background(), tableID)
		if err != nil {
			return
		}
		seat := findSeat(cur, userID)
		if seat == nil || !seat.Holding {
			return
		}
		_, _ = s.leaveLocked(context.Background(), cur, userID)
	})

	return view, nil
}

func (s *service) clearHoldLocked(tableID, userID string) {
	key := holdKey(tableID, userID)
	if t, ok := s.holds[key]; ok {
		t.Stop()
		delete(s.holds, key)
	}
}

func (s *service) broadcast(tableID string, ev Event) {
	if s.bcast != nil {
		s.bcast.Broadcast(tableID, ev)
	}
}

func holdKey(tableID, userID string) string {
	return tableID + "\x00" + userID
}

func newEmptyTable(worldID string) *tablerepo.Table {
	now := time.Now().UTC()
	seats := make([]tablerepo.Seat, tablerepo.MaxSeats)
	for i := range seats {
		seats[i] = emptySeat(i)
	}
	return &tablerepo.Table{
		ID:        primitive.NewObjectID().Hex(),
		WorldID:   worldID,
		Status:    tablerepo.StatusLobby,
		Seats:     seats,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func emptySeat(index int) tablerepo.Seat {
	return tablerepo.Seat{SeatIndex: index}
}

func seatUser(t *tablerepo.Table, userID, username string) error {
	if findSeat(t, userID) != nil {
		return nil
	}
	for i := range t.Seats {
		if t.Seats[i].UserID == "" {
			t.Seats[i].UserID = userID
			t.Seats[i].Username = username
			t.Seats[i].Ready = false
			t.Seats[i].Holding = false
			t.Seats[i].HoldEndsAt = nil
			return nil
		}
	}
	return ErrFull
}

func findSeat(t *tablerepo.Table, userID string) *tablerepo.Seat {
	for i := range t.Seats {
		if t.Seats[i].UserID == userID {
			return &t.Seats[i]
		}
	}
	return nil
}

func everyoneReady(t *tablerepo.Table) bool {
	n := 0
	for _, s := range t.Seats {
		if s.UserID == "" {
			continue
		}
		n++
		if s.Holding || !s.Ready {
			return false
		}
	}
	return n >= tablerepo.MinSeats
}

func toView(t *tablerepo.Table) *View {
	seats := make([]SeatView, len(t.Seats))
	for i, s := range t.Seats {
		sv := SeatView{
			SeatIndex: s.SeatIndex,
			Ready:     s.Ready,
			Holding:   s.Holding,
		}
		if s.UserID != "" {
			uid := s.UserID
			sv.UserID = &uid
		}
		if s.Username != "" {
			name := s.Username
			sv.Username = &name
		}
		if s.HoldEndsAt != nil {
			iso := s.HoldEndsAt.UTC().Format(time.RFC3339)
			sv.HoldEndsAt = &iso
		}
		seats[i] = sv
	}
	return &View{
		ID:      t.ID,
		WorldID: t.WorldID,
		Status:  t.Status,
		Seats:   seats,
		GameID:  gameIDPtr(t.GameID),
	}
}

func gameIDPtr(id string) *string {
	if id == "" {
		return nil
	}
	v := id
	return &v
}
