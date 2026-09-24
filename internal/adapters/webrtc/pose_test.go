package webrtc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStampPoseForcesIdentity(t *testing.T) {
	raw := []byte(`{"type":"pose","userId":"spoof","username":"Hacker","x":0.25,"y":0.75}`)
	out, err := StampPose("real-u", "Ada", raw)
	if err != nil {
		t.Fatal(err)
	}
	var msg PoseMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != PoseMessageType {
		t.Fatalf("type=%s", msg.Type)
	}
	if msg.UserID != "real-u" || msg.Username != "Ada" {
		t.Fatalf("identity=%s/%s", msg.UserID, msg.Username)
	}
	if msg.X != 0.25 || msg.Y != 0.75 {
		t.Fatalf("xy=%v,%v", msg.X, msg.Y)
	}
}

func TestStampPoseRejectsBadTypeAndSize(t *testing.T) {
	if _, err := StampPose("u", "A", []byte(`{"type":"chat","x":0,"y":0}`)); err == nil {
		t.Fatal("expected type error")
	}
	big := []byte(`{"type":"pose","x":0,"y":0,"pad":"` + strings.Repeat("x", MaxPoseBytes) + `"}`)
	if _, err := StampPose("u", "A", big); err == nil {
		t.Fatal("expected size error")
	}
}

func TestStampPoseAllowsMissingType(t *testing.T) {
	out, err := StampPose("u1", "Bob", []byte(`{"x":0.1,"y":0.2}`))
	if err != nil {
		t.Fatal(err)
	}
	var msg PoseMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != PoseMessageType || msg.UserID != "u1" {
		t.Fatalf("%+v", msg)
	}
}
