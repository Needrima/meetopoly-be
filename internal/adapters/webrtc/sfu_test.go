package webrtc

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestHubRoomFull(t *testing.T) {
	sfu := NewSFU()
	room := HubRoomID("africa-1:lagos")
	for i := 0; i < MaxHubPeers; i++ {
		id := fmt.Sprintf("u%d", i)
		if err := sfu.Attach(room, id, id, "", &memSignal{}); err != nil {
			t.Fatalf("attach %d: %v", i, err)
		}
	}
	if err := sfu.Attach(room, "overflow", "X", "", &memSignal{}); !errors.Is(err, ErrHubFull) {
		t.Fatalf("want ErrHubFull got %v", err)
	}
	// Reconnect of an existing peer must still succeed.
	if err := sfu.Attach(room, "u0", "u0", "", &memSignal{}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
}

func TestHubRoomID(t *testing.T) {
	if got := HubRoomID("africa-1:lagos"); got != "hub:africa-1:lagos" {
		t.Fatalf("got %s", got)
	}
	if got := HubRoomID("hub:already"); got != "hub:already" {
		t.Fatalf("prefixed=%s", got)
	}
}

func TestAllowPoseRateLimit(t *testing.T) {
	sfu := NewSFU()
	room := BoardRoomID("rate")
	sig := &memSignal{}
	if err := sfu.Attach(room, "u1", "Ada", "", sig); err != nil {
		t.Fatal(err)
	}
	if !sfu.allowPose(room, "u1") {
		t.Fatal("first pose should pass")
	}
	if sfu.allowPose(room, "u1") {
		t.Fatal("immediate second pose should be rate-limited")
	}
	// Advance past min interval without sleeping wall clock.
	sfu.mu.Lock()
	p := sfu.peerLocked(room, "u1")
	p.lastPoseAt = time.Now().Add(-poseMinInterval - time.Millisecond)
	sfu.mu.Unlock()
	if !sfu.allowPose(room, "u1") {
		t.Fatal("pose after interval should pass")
	}
}

func TestAttachDetachRoster(t *testing.T) {
	sfu := NewSFU()
	room := BoardRoomID("game-a")
	a := &memSignal{}
	b := &memSignal{}
	if err := sfu.Attach(room, "u1", "Ada", "", a); err != nil {
		t.Fatal(err)
	}
	if err := sfu.Attach(room, "u2", "Bob", "", b); err != nil {
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
	if err := sfu.Attach(room, "u1", "Ada", "", oldSig); err != nil {
		t.Fatal(err)
	}
	if err := sfu.Attach(room, "u1", "Ada", "", newSig); err != nil {
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

func TestIsHubRoom(t *testing.T) {
	if !IsHubRoom("hub:africa-1:lagos") {
		t.Fatal("expected hub")
	}
	if IsHubRoom(BoardRoomID("g1")) {
		t.Fatal("board must not be hub")
	}
	if !IsBoardRoom(BoardRoomID("g1")) {
		t.Fatal("expected board")
	}
	if !IsVoiceRoom(HubRoomID("africa-1:lagos")) || !IsVoiceRoom(BoardRoomID("g1")) {
		t.Fatal("hub and board are voice rooms")
	}
}

func TestVoiceRoomsGetAudioPubsMap(t *testing.T) {
	sfu := NewSFU()
	hub := HubRoomID("africa-1:cairo")
	board := BoardRoomID("game-audio")
	if err := sfu.Attach(hub, "u1", "Ada", "", &memSignal{}); err != nil {
		t.Fatal(err)
	}
	if err := sfu.Attach(board, "u1", "Ada", "", &memSignal{}); err != nil {
		t.Fatal(err)
	}
	sfu.mu.Lock()
	hr := sfu.rooms[hub]
	br := sfu.rooms[board]
	sfu.mu.Unlock()
	if hr == nil || hr.audioPubs == nil {
		t.Fatal("hub room must allocate audioPubs")
	}
	if br == nil || br.audioPubs == nil {
		t.Fatal("board room must allocate audioPubs (Phase 10.4)")
	}
	if sfu.HubAudioPublisherCount(hub) != 0 {
		t.Fatal("no pubs yet")
	}
	if sfu.HubAudioPublisherCount(board) != 0 {
		t.Fatal("no pubs yet")
	}
}

func TestDetachClearsHubAudioPubSlot(t *testing.T) {
	sfu := NewSFU()
	hub := HubRoomID("africa-1:accra")
	sig := &memSignal{}
	if err := sfu.Attach(hub, "u1", "Ada", "", sig); err != nil {
		t.Fatal(err)
	}
	sfu.mu.Lock()
	r := sfu.rooms[hub]
	stop := make(chan struct{})
	r.audioPubs["u1"] = &hubAudioPub{fromUserID: "u1", stop: stop}
	sfu.mu.Unlock()
	if sfu.HubAudioPublisherCount(hub) != 1 {
		t.Fatal("expected one pub")
	}
	if _, ok := sfu.Detach(hub, "u1", sig); !ok {
		t.Fatal("detach")
	}
	if sfu.HubAudioPublisherCount(hub) != 0 {
		t.Fatal("pub must clear on detach")
	}
	select {
	case <-stop:
	default:
		t.Fatal("stop channel should be closed")
	}
}
