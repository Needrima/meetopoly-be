package webrtc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStampHubChat(t *testing.T) {
	out, err := StampHubChat("u1", "Ada", []byte(`{"type":"hubChat","userId":"spoof","text":"  hello  "}`))
	if err != nil {
		t.Fatal(err)
	}
	var msg HubChatMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != HubChatMessageType || msg.UserID != "u1" || msg.Username != "Ada" || msg.Text != "hello" {
		t.Fatalf("%+v", msg)
	}
	if msg.T <= 0 {
		t.Fatal("expected server t")
	}
}

func TestStampHubChatRejects(t *testing.T) {
	if _, err := StampHubChat("u", "A", []byte(`{"type":"hubChat","text":"   "}`)); err == nil {
		t.Fatal("empty text")
	}
	long := strings.Repeat("字", MaxHubChatRunes+1)
	raw, _ := json.Marshal(map[string]any{"type": HubChatMessageType, "text": long})
	if _, err := StampHubChat("u", "A", raw); err == nil {
		t.Fatal("too long")
	}
	if utf8.RuneCountInString(long) <= MaxHubChatRunes {
		t.Fatal("test setup")
	}
	big := []byte(`{"type":"hubChat","text":"x","pad":"` + strings.Repeat("x", MaxHubChatBytes) + `"}`)
	if _, err := StampHubChat("u", "A", big); err == nil {
		t.Fatal("size")
	}
	if _, err := StampHubChat("u", "A", []byte(`{"type":"pose","text":"hi"}`)); err == nil {
		t.Fatal("wrong type")
	}
}

func TestStampPresenceDCHubChat(t *testing.T) {
	out, err := StampPresenceDC("u9", "Zoe", []byte(`{"type":"hubChat","text":"yo"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	var msg HubChatMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.UserID != "u9" || msg.Text != "yo" {
		t.Fatalf("%+v", msg)
	}
	if _, err := StampPresenceDC("u9", "Zoe", []byte(`{"type":"hubChat","text":"yo"}`), false); err == nil {
		t.Fatal("board must reject hubChat")
	}
}

func TestAllowHubChatRate(t *testing.T) {
	sfu := NewSFU()
	hub := HubRoomID("africa-1:lagos")
	if err := sfu.Attach(hub, "u1", "Ada", "", &memSignal{}); err != nil {
		t.Fatal(err)
	}
	if !sfu.allowHubChat(hub, "u1") {
		t.Fatal("first chat ok")
	}
	if sfu.allowHubChat(hub, "u1") {
		t.Fatal("immediate second must be rate-limited")
	}
	time.Sleep(hubChatMinInterval + 5*time.Millisecond)
	if !sfu.allowHubChat(hub, "u1") {
		t.Fatal("after interval ok")
	}
}

func TestIsHubChatDC(t *testing.T) {
	if !IsHubChatDC(HubChatMessageType) {
		t.Fatal("hubChat")
	}
	if IsHubChatDC(PoseMessageType) || IsHubChatDC(VideoMutedMessageType) || IsHubChatDC("") {
		t.Fatal("non-chat")
	}
}
