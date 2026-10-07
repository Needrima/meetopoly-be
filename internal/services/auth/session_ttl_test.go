package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	sessionrepo "meetopoly-be/internal/repository/session"
)

type memSessions struct {
	mu      sync.Mutex
	byToken map[string]*sessionEntry
	touches int
}

type sessionEntry struct {
	data      sessionrepo.TokenData
	expiresAt time.Time // zero = no expiry (legacy)
	ttl       time.Duration
}

func newMemSessions() *memSessions {
	return &memSessions{byToken: map[string]*sessionEntry{}}
}

func (m *memSessions) Create(_ context.Context, token string, data sessionrepo.TokenData, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := &sessionEntry{data: data, ttl: ttl}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}
	m.byToken[token] = e
	return nil
}

func (m *memSessions) Get(_ context.Context, token string) (*sessionrepo.TokenData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byToken[token]
	if !ok {
		return nil, sessionrepo.ErrNotFound
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(m.byToken, token)
		return nil, sessionrepo.ErrNotFound
	}
	cp := e.data
	return &cp, nil
}

func (m *memSessions) Delete(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byToken, token)
	return nil
}

func (m *memSessions) DeleteAllForUser(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for t, e := range m.byToken {
		if e.data.UserID == userID && e.data.Kind == sessionrepo.KindSession {
			delete(m.byToken, t)
		}
	}
	return nil
}

func (m *memSessions) TTL(_ context.Context, token string) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byToken[token]
	if !ok {
		return 0, sessionrepo.ErrNotFound
	}
	if e.expiresAt.IsZero() {
		return -1 * time.Second, nil
	}
	rem := time.Until(e.expiresAt)
	if rem <= 0 {
		delete(m.byToken, token)
		return 0, sessionrepo.ErrNotFound
	}
	return rem, nil
}

func (m *memSessions) Touch(_ context.Context, token string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byToken[token]
	if !ok {
		return sessionrepo.ErrNotFound
	}
	m.touches++
	e.ttl = ttl
	e.expiresAt = time.Now().Add(ttl)
	return nil
}

func (m *memSessions) createdTTL(token string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.byToken[token]; ok {
		return e.ttl
	}
	return 0
}

func TestSessionTTLDefault(t *testing.T) {
	s := &service{cfg: Config{}, sessions: newMemSessions()}
	if got := s.sessionTTL(); got != 30*24*time.Hour {
		t.Fatalf("default sessionTTL = %v want 30d", got)
	}
}

func TestSessionTTLFromConfig(t *testing.T) {
	want := 48 * time.Hour
	s := &service{cfg: Config{SessionTTL: want}, sessions: newMemSessions()}
	if got := s.sessionTTL(); got != want {
		t.Fatalf("sessionTTL = %v want %v", got, want)
	}
}

func TestResolveSessionSlidesWhenUnderHalf(t *testing.T) {
	sess := newMemSessions()
	ttl := 2 * time.Hour
	s := &service{cfg: Config{SessionTTL: ttl}, sessions: sess}
	token := "tok-slide"
	_ = sess.Create(context.Background(), token, sessionrepo.TokenData{
		UserID: "u1",
		Email:  "a@b.co",
		Kind:   sessionrepo.KindSession,
	}, ttl)
	// Force remaining under half.
	sess.mu.Lock()
	sess.byToken[token].expiresAt = time.Now().Add(ttl/2 - time.Minute)
	sess.mu.Unlock()

	uid, err := s.ResolveSession(context.Background(), token)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if uid != "u1" {
		t.Fatalf("userID = %q", uid)
	}
	if sess.touches != 1 {
		t.Fatalf("touches = %d want 1", sess.touches)
	}
	rem, err := sess.TTL(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if rem < ttl-time.Minute {
		t.Fatalf("after slide remaining %v want ~%v", rem, ttl)
	}
}

func TestResolveSessionNoSlideWhenFresh(t *testing.T) {
	sess := newMemSessions()
	ttl := 2 * time.Hour
	s := &service{cfg: Config{SessionTTL: ttl}, sessions: sess}
	token := "tok-fresh"
	_ = sess.Create(context.Background(), token, sessionrepo.TokenData{
		UserID: "u1",
		Kind:   sessionrepo.KindSession,
	}, ttl)

	if _, err := s.ResolveSession(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if sess.touches != 0 {
		t.Fatalf("touches = %d want 0 (still above half TTL)", sess.touches)
	}
}

func TestResolveSessionSlidesLegacyNoExpiry(t *testing.T) {
	sess := newMemSessions()
	ttl := time.Hour
	s := &service{cfg: Config{SessionTTL: ttl}, sessions: sess}
	token := "tok-legacy"
	_ = sess.Create(context.Background(), token, sessionrepo.TokenData{
		UserID: "u1",
		Kind:   sessionrepo.KindSession,
	}, 0) // no expiry

	if _, err := s.ResolveSession(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if sess.touches != 1 {
		t.Fatalf("touches = %d want 1 (legacy -1)", sess.touches)
	}
	if got := sess.createdTTL(token); got != ttl {
		t.Fatalf("ttl after touch = %v want %v", got, ttl)
	}
}

func TestResolveSessionRejectsExpired(t *testing.T) {
	sess := newMemSessions()
	s := &service{cfg: Config{SessionTTL: time.Hour}, sessions: sess}
	token := "tok-dead"
	_ = sess.Create(context.Background(), token, sessionrepo.TokenData{
		UserID: "u1",
		Kind:   sessionrepo.KindSession,
	}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	_, err := s.ResolveSession(context.Background(), token)
	if err != ErrUnauthorized {
		t.Fatalf("err = %v want ErrUnauthorized", err)
	}
}
