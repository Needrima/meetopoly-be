package webrtc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStampPresenceDCRoutesPose(t *testing.T) {
	out, err := StampPresenceDC("u1", "Ada", []byte(`{"type":"pose","userId":"spoof","x":0.2,"y":0.8}`))
	if err != nil {
		t.Fatal(err)
	}
	var msg PoseMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.UserID != "u1" || msg.Type != PoseMessageType {
		t.Fatalf("%+v", msg)
	}
}

func TestStampPresenceDCVideoMuted(t *testing.T) {
	out, err := StampPresenceDC("u2", "Bob", []byte(`{"type":"videoMuted","userId":"spoof","muted":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var msg VideoMutedMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != VideoMutedMessageType || msg.UserID != "u2" || !msg.Muted {
		t.Fatalf("%+v", msg)
	}

	out2, err := StampPresenceDC("u2", "Bob", []byte(`{"type":"videoMuted","muted":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out2, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Muted {
		t.Fatalf("expected unmuted %+v", msg)
	}
}

func TestStampPresenceDCVideoOrientation(t *testing.T) {
	out, err := StampPresenceDC("u3", "Cara", []byte(`{"type":"videoOrientation","userId":"x","rotationDeg":90}`))
	if err != nil {
		t.Fatal(err)
	}
	var msg VideoOrientationMessage
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != VideoOrientationMessageType || msg.UserID != "u3" || msg.RotationDeg != 90 {
		t.Fatalf("%+v", msg)
	}
}

func TestStampPresenceDCRejectsUnknownAndBad(t *testing.T) {
	if _, err := StampPresenceDC("u", "A", []byte(`{"type":"chat"}`)); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := StampPresenceDC("u", "A", []byte(`{"type":"videoOrientation","rotationDeg":"nope"}`)); err == nil {
		t.Fatal("expected json error")
	}
	big := []byte(`{"type":"videoMuted","muted":true,"pad":"` + strings.Repeat("x", MaxVideoControlBytes) + `"}`)
	if _, err := StampPresenceDC("u", "A", big); err == nil {
		t.Fatal("expected size error")
	}
}

func TestIsVideoControlDC(t *testing.T) {
	if !IsVideoControlDC(VideoMutedMessageType) || !IsVideoControlDC(VideoOrientationMessageType) {
		t.Fatal("expected video control types")
	}
	if IsVideoControlDC(PoseMessageType) || IsVideoControlDC("chat") || IsVideoControlDC("") {
		t.Fatal("pose/unknown must not be treated as video control")
	}
	if PeekPresenceDCType([]byte(`{"type":"videoOrientation","rotationDeg":90}`)) != VideoOrientationMessageType {
		t.Fatal("peek orientation")
	}
}
