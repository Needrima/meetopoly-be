package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	tablerepo "meetopoly-be/internal/repository/table"
)

const (
	defaultLobbyTTL      = 15 * time.Minute
	defaultSweepInterval = 15 * time.Minute
)

type service struct {
	repo        tablerepo.Repository
	cfg         Config
	bcast       Broadcaster
	gameStarter GameStarter
	avatars     AvatarLookup
	mu          sync.Mutex
	holds       map[string]*time.Timer
}

// New builds a table Service.
func New(repo tablerepo.Repository, cfg Config) Service {
	if cfg.DisconnectHold <= 0 {
		cfg.DisconnectHold = 45 * time.Second
	}
	if cfg.LobbyTTL <= 0 {
		cfg.LobbyTTL = defaultLobbyTTL
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = defaultSweepInterval
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

func (s *service) SetAvatarLookup(l AvatarLookup) {
	s.avatars = l
}

func (s *service) UpdateSeatedUsername(ctx context.Context, userID, username string) error {
	userID = strings.TrimSpace(userID)
	username = strings.TrimSpace(username)
	if userID == "" || username == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.repo.FindLobbyByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil
		}
		return err
	}
	changed := false
	for i := range t.Seats {
		if t.Seats[i].UserID == userID {
			if t.Seats[i].Username != username {
				t.Seats[i].Username = username
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	if err := s.repo.Update(ctx, t); err != nil {
		return err
	}
	view := s.viewOf(ctx, t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})
	return nil
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

	// Resume existing **public** lobby seat (reconnect / remount), unless the
	// table is a broken half-start (status starting, no game) — those block
	// matchmaking. Private lobbies are left so public Join never resumes them.
	// Expired lobbies (Phase 20.6) are deleted and rematched.
	existing, err := s.repo.FindLobbyByUser(ctx, userID)
	if err == nil && existing.WorldID == worldID {
		if s.isLobbyExpired(existing) {
			_ = s.expireLobbyLocked(ctx, existing)
			existing = nil
		} else if existing.Private || isBrokenMatchmakingTable(existing) {
			_, _ = s.leaveLocked(ctx, existing, userID)
			existing = nil
		} else {
			s.clearHoldLocked(existing.ID, userID)
			for i := range existing.Seats {
				if existing.Seats[i].UserID == userID {
					existing.Seats[i].Holding = false
					existing.Seats[i].HoldEndsAt = nil
					existing.Seats[i].Username = username
					ensureSeatColor(existing, i)
				}
			}
			refreshLobbyTTLIfSolo(existing)
			if err := s.repo.Update(ctx, existing); err != nil {
				return nil, err
			}
			view := s.viewOf(ctx, existing)
			s.broadcast(existing.ID, Event{Type: "state", Table: view})
			return view, nil
		}
	}
	if err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}
	// Seated on another world lobby — leave it first.
	if existing != nil {
		if s.isLobbyExpired(existing) {
			_ = s.expireLobbyLocked(ctx, existing)
		} else {
			_, _ = s.leaveLocked(ctx, existing, userID)
		}
	}

	minCreated := time.Now().UTC().Add(-s.cfg.LobbyTTL)
	t, err := s.repo.FindOpenLobby(ctx, worldID, minCreated)
	if err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}
	if errors.Is(err, tablerepo.ErrNotFound) {
		t = newEmptyTable(worldID)
		if err := seatUser(t, userID, username); err != nil {
			return nil, err
		}
		refreshLobbyTTLIfSolo(t)
		if err := s.repo.Insert(ctx, t); err != nil {
			return nil, err
		}
		view := s.viewOf(ctx, t)
		s.broadcast(t.ID, Event{Type: "state", Table: view})
		return view, nil
	}

	if s.isLobbyExpired(t) {
		_ = s.expireLobbyLocked(ctx, t)
		t = newEmptyTable(worldID)
		if err := seatUser(t, userID, username); err != nil {
			return nil, err
		}
		refreshLobbyTTLIfSolo(t)
		if err := s.repo.Insert(ctx, t); err != nil {
			return nil, err
		}
		view := s.viewOf(ctx, t)
		s.broadcast(t.ID, Event{Type: "state", Table: view})
		return view, nil
	}

	if err := seatUser(t, userID, username); err != nil {
		return nil, err
	}
	refreshLobbyTTLIfSolo(t)
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, t)
	s.broadcast(t.ID, Event{Type: "state", Table: view})
	return view, nil
}

// CreatePrivate creates an invite-only lobby and seats the host (Phase 20).
func (s *service) CreatePrivate(ctx context.Context, userID, username, worldID string) (*View, error) {
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

	// Always a fresh private table — leave any existing lobby seat first.
	existing, err := s.repo.FindLobbyByUser(ctx, userID)
	if err == nil {
		_, _ = s.leaveLocked(ctx, existing, userID)
	} else if !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}

	const maxAttempts = 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		code, genErr := generateInviteCode()
		if genErr != nil {
			return nil, fmt.Errorf("invite code: %w", genErr)
		}
		t := newEmptyTable(worldID)
		t.Private = true
		t.InviteCode = code
		if seatErr := seatUser(t, userID, username); seatErr != nil {
			return nil, seatErr
		}
		if insertErr := s.repo.Insert(ctx, t); insertErr != nil {
			if mongo.IsDuplicateKeyError(insertErr) {
				continue
			}
			return nil, insertErr
		}
		view := s.viewOf(ctx, t)
		s.broadcast(t.ID, Event{Type: "state", Table: view})
		return view, nil
	}
	return nil, fmt.Errorf("invite code: exhausted unique attempts")
}

// JoinByInviteCode seats the caller into a private lobby identified by invite code.
// Valid only while status is lobby; sealed tables (started / in_game) return ErrWrongStatus.
func (s *service) JoinByInviteCode(ctx context.Context, userID, username, inviteCode string) (*View, error) {
	code := normalizeInviteCode(inviteCode)
	if code == "" {
		return nil, ErrInvalidInviteCode
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "Player"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, err := s.repo.FindByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, tablerepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if s.isLobbyExpired(t) {
		_ = s.expireLobbyLocked(ctx, t)
		return nil, ErrNotFound
	}

	// Resume if already seated on this table (reconnect / remount).
	if seat := findSeat(t, userID); seat != nil {
		if t.Status != tablerepo.StatusLobby {
			return nil, ErrWrongStatus
		}
		s.clearHoldLocked(t.ID, userID)
		seat.Holding = false
		seat.HoldEndsAt = nil
		seat.Username = username
		ensureSeatColor(t, seat.SeatIndex)
		refreshLobbyTTLIfSolo(t)
		if err := s.repo.Update(ctx, t); err != nil {
			return nil, err
		}
		view := s.viewOf(ctx, t)
		s.broadcast(t.ID, Event{Type: "state", Table: view})
		return view, nil
	}

	if t.Status != tablerepo.StatusLobby || t.GameID != "" {
		return nil, ErrWrongStatus
	}

	// Leave any other lobby seat first.
	existing, err := s.repo.FindLobbyByUser(ctx, userID)
	if err == nil && existing.ID != t.ID {
		if s.isLobbyExpired(existing) {
			_ = s.expireLobbyLocked(ctx, existing)
		} else {
			_, _ = s.leaveLocked(ctx, existing, userID)
		}
		// Re-load target in case leave somehow touched it (should not).
		t, err = s.repo.FindByInviteCode(ctx, code)
		if err != nil {
			if errors.Is(err, tablerepo.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if s.isLobbyExpired(t) {
			_ = s.expireLobbyLocked(ctx, t)
			return nil, ErrNotFound
		}
		if t.Status != tablerepo.StatusLobby || t.GameID != "" {
			return nil, ErrWrongStatus
		}
	} else if err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return nil, err
	}

	if err := seatUser(t, userID, username); err != nil {
		return nil, err
	}
	refreshLobbyTTLIfSolo(t)
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, t)
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
	return s.viewOf(ctx, t), nil
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
	if s.isLobbyExpired(t) {
		_ = s.expireLobbyLocked(ctx, t)
		return nil, ErrNotFound
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
		viewSeats := s.viewOf(ctx, t).Seats
		gameID, err := s.gameStarter.StartFromTable(ctx, t.ID, t.WorldID, viewSeats)
		if err != nil {
			// Keep lobby + Ready flags; do not leave status stuck on starting.
			if updErr := s.repo.Update(ctx, t); updErr != nil {
				return nil, updErr
			}
			view := s.viewOf(ctx, t)
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
	view := s.viewOf(ctx, t)
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

func (s *service) isLobbyExpired(t *tablerepo.Table) bool {
	if t == nil {
		return false
	}
	if t.Status != tablerepo.StatusLobby || t.GameID != "" {
		return false
	}
	if t.CreatedAt.IsZero() {
		return false
	}
	return !t.CreatedAt.UTC().Add(s.cfg.LobbyTTL).After(time.Now().UTC())
}

// refreshLobbyTTLIfSolo bumps createdAt when exactly one player is seated so a
// recycled public/private lobby gets a fresh LobbyTTL window (Phase 20.6 follow-up).
func refreshLobbyTTLIfSolo(t *tablerepo.Table) {
	if t == nil || t.Status != tablerepo.StatusLobby || t.GameID != "" {
		return
	}
	if tablerepo.OccupiedCount(t) != 1 {
		return
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
}

func (s *service) expiresAtUTC(t *tablerepo.Table) time.Time {
	if t == nil || t.CreatedAt.IsZero() {
		return time.Now().UTC().Add(s.cfg.LobbyTTL)
	}
	return t.CreatedAt.UTC().Add(s.cfg.LobbyTTL)
}

// expireLobbyLocked broadcasts expired, clears holds, and deletes the table.
// Caller must hold s.mu.
func (s *service) expireLobbyLocked(ctx context.Context, t *tablerepo.Table) error {
	if t == nil {
		return nil
	}
	view := s.viewOf(ctx, t)
	s.broadcast(t.ID, Event{Type: "expired", Table: view})
	for _, seat := range t.Seats {
		if seat.UserID != "" {
			s.clearHoldLocked(t.ID, seat.UserID)
		}
	}
	if err := s.repo.Delete(ctx, t.ID); err != nil && !errors.Is(err, tablerepo.ErrNotFound) {
		return err
	}
	return nil
}

// SweepExpired deletes unstarted lobbies past LobbyTTL (Phase 20.6).
func (s *service) SweepExpired(ctx context.Context) (int, error) {
	before := time.Now().UTC().Add(-s.cfg.LobbyTTL)
	list, err := s.repo.ListUnstartedCreatedBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	if len(list) == 0 {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, t := range list {
		if !s.isLobbyExpired(t) {
			continue
		}
		if err := s.expireLobbyLocked(ctx, t); err != nil {
			slog.Error("table sweep expire failed", "tableId", t.ID, "err", err)
			continue
		}
		n++
	}
	if n > 0 {
		slog.Info("table sweep expired lobbies", "count", n)
	}
	return n, nil
}

// StartSweeper runs SweepExpired on SweepInterval until ctx is cancelled.
func (s *service) StartSweeper(ctx context.Context) {
	interval := s.cfg.SweepInterval
	go func() {
		// Run once shortly after boot so restarts clean backlog without waiting a full interval.
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if _, err := s.SweepExpired(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("table sweep failed", "err", err)
				}
			case <-ticker.C:
				if _, err := s.SweepExpired(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("table sweep failed", "err", err)
				}
			}
		}
	}()
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
		return s.viewOf(ctx, t), nil
	}
	normalizeMatchmakingStatus(t)
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, t)
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
		return s.viewOf(ctx, t), nil
	}
	seat := findSeat(t, userID)
	if seat == nil {
		return s.viewOf(ctx, t), nil
	}
	ends := time.Now().UTC().Add(s.cfg.DisconnectHold)
	seat.Holding = true
	seat.HoldEndsAt = &ends
	seat.Ready = false

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	view := s.viewOf(ctx, t)
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

// seatPalette — distinct accents for up to 6 players (matches game pin palette).
var seatPalette = []string{
	"#6B3FA0",
	"#0072BB",
	"#1FB25A",
	"#F7941D",
	"#D93A96",
	"#FEF200",
}

func usedSeatColors(t *tablerepo.Table, exceptIndex int) map[string]bool {
	used := make(map[string]bool, len(t.Seats))
	for i, s := range t.Seats {
		if i == exceptIndex || s.UserID == "" || s.PinColor == "" {
			continue
		}
		used[strings.ToLower(s.PinColor)] = true
	}
	return used
}

func pickFreeSeatColor(t *tablerepo.Table, exceptIndex int) string {
	used := usedSeatColors(t, exceptIndex)
	for _, c := range seatPalette {
		if !used[strings.ToLower(c)] {
			return c
		}
	}
	return seatPalette[exceptIndex%len(seatPalette)]
}

// ensureSeatColor assigns a free palette color if missing or colliding.
func ensureSeatColor(t *tablerepo.Table, seatIndex int) {
	if seatIndex < 0 || seatIndex >= len(t.Seats) {
		return
	}
	cur := strings.ToLower(strings.TrimSpace(t.Seats[seatIndex].PinColor))
	used := usedSeatColors(t, seatIndex)
	if cur != "" && !used[cur] {
		return
	}
	t.Seats[seatIndex].PinColor = pickFreeSeatColor(t, seatIndex)
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
			t.Seats[i].PinColor = pickFreeSeatColor(t, i)
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

func (s *service) viewOf(ctx context.Context, t *tablerepo.Table) *View {
	seats := make([]SeatView, len(t.Seats))
	for i, seat := range t.Seats {
		sv := SeatView{
			SeatIndex: seat.SeatIndex,
			Ready:     seat.Ready,
			Holding:   seat.Holding,
		}
		if seat.UserID != "" {
			uid := seat.UserID
			sv.UserID = &uid
			if s.avatars != nil {
				if url := strings.TrimSpace(s.avatars.AvatarURLForUser(ctx, seat.UserID)); url != "" {
					sv.AvatarURL = &url
				}
			}
		}
		if seat.Username != "" {
			name := seat.Username
			sv.Username = &name
		}
		if seat.PinColor != "" {
			c := seat.PinColor
			sv.PinColor = &c
		}
		if seat.HoldEndsAt != nil {
			iso := seat.HoldEndsAt.UTC().Format(time.RFC3339)
			sv.HoldEndsAt = &iso
		}
		seats[i] = sv
	}
	return &View{
		ID:         t.ID,
		WorldID:    t.WorldID,
		Status:     t.Status,
		Private:    t.Private,
		InviteCode: inviteCodePtr(t.InviteCode),
		Seats:      seats,
		GameID:     gameIDPtr(t.GameID),
		ExpiresAt:  s.expiresAtUTC(t).Format(time.RFC3339),
	}
}

func gameIDPtr(id string) *string {
	if id == "" {
		return nil
	}
	v := id
	return &v
}

func inviteCodePtr(code string) *string {
	if code == "" {
		return nil
	}
	v := code
	return &v
}
