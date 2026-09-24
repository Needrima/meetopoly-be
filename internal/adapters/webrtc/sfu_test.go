package webrtc

import (
	"sync"
	"testing"
)

type memSignal struct {
	mu   sync.Mutex
	msgs []any
}

func (m *memSignal) WriteJSON(v any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, v)
	return nil
}

func TestBoardRoomID(t *testing.T) {
	if got := BoardRoomID("g1"); got != "board:g1" {
		t.Fatalf("got %s", got)
	}
}

func TestAttachDetachRoster(t *testing.T) {
	sfu := NewSFU()
	room := BoardRoomID("game-a")
	a := &memSignal{}
	b := &memSignal{}
	if err := sfu.Attach(room, "u1", "Ada", a); err != nil {
		t.Fatal(err)
	}
	if err := sfu.Attach(room, "u2", "Bob", b); err != nil {
		t.Fatal(err)
	}
	roster := sfu.Roster(room, "u1")
	if len(roster) != 1 || roster[0].UserID != "u2" {
		t.Fatalf("roster=%v", roster)
	}
	info, ok := sfu.Detach(room, "u2", b)
	if !ok || info.Username != "Bob" {
		t.Fatalf("detach=%v ok=%v", info, ok)
	}
	if len(sfu.Roster(room, "")) != 1 {
		t.Fatalf("expected u1 still in room")
	}
	_, ok = sfu.Detach(room, "u1", a)
	if !ok {
		t.Fatal("expected detach u1")
	}
	if len(sfu.Roster(room, "")) != 0 {
		t.Fatal("room should be empty")
	}
}

func TestDetachIgnoresStaleSignal(t *testing.T) {
	sfu := NewSFU()
	room := BoardRoomID("game-b")
	oldSig := &memSignal{}
	newSig := &memSignal{}
	if err := sfu.Attach(room, "u1", "Ada", oldSig); err != nil {
		t.Fatal(err)
	}
	if err := sfu.Attach(room, "u1", "Ada", newSig); err != nil {
		t.Fatal(err)
	}
	if _, ok := sfu.Detach(room, "u1", oldSig); ok {
		t.Fatal("stale signal must not detach")
	}
	if len(sfu.Roster(room, "")) != 1 {
		t.Fatal("peer should remain after stale detach")
	}
	info, ok := sfu.Detach(room, "u1", newSig)
	if !ok || info.UserID != "u1" {
		t.Fatalf("detach current=%v ok=%v", info, ok)
	}
}

func TestICEServersJSON(t *testing.T) {
	sfu := NewSFU()
	servers := sfu.ICEServersJSON()
	if len(servers) != 1 || len(servers[0].URLs) != 1 {
		t.Fatalf("%v", servers)
	}
	if servers[0].URLs[0] != "stun:stun.l.google.com:19302" {
		t.Fatalf("stun=%s", servers[0].URLs[0])
	}
}
