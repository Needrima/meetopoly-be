package webrtc

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	// VideoMutedMessageType is the locked DC type for board cam-off (Phase 16.2).
	VideoMutedMessageType = "videoMuted"
	// VideoOrientationMessageType is the locked DC type for iOS upright correction.
	VideoOrientationMessageType = "videoOrientation"
	// MaxVideoControlBytes rejects oversized video control DC payloads.
	MaxVideoControlBytes = 256
)

// VideoMutedMessage announces whether a publisher's board camera is off.
// Remotes cannot observe sender track.enabled; they use this for AvatarPod.
type VideoMutedMessage struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
	Muted  bool   `json:"muted"`
}

// VideoOrientationMessage announces display rotation (deg CW) for a publisher.
type VideoOrientationMessage struct {
	Type        string  `json:"type"`
	UserID      string  `json:"userId"`
	RotationDeg float64 `json:"rotationDeg"`
}

type dcTypePeek struct {
	Type string `json:"type"`
}

// StampPresenceDC routes a presence DataChannel payload: pose → StampPose,
// videoMuted / videoOrientation → stamped control JSON. Unknown types are rejected.
func StampPresenceDC(userID, username string, raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("dc empty")
	}
	var peek dcTypePeek
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil, fmt.Errorf("dc json: %w", err)
	}
	switch peek.Type {
	case PoseMessageType, "":
		return StampPose(userID, username, raw)
	case VideoMutedMessageType:
		return StampVideoMuted(userID, raw)
	case VideoOrientationMessageType:
		return StampVideoOrientation(userID, raw)
	default:
		return nil, fmt.Errorf("dc type")
	}
}

// StampVideoMuted forces type/identity from the authenticated peer.
func StampVideoMuted(userID string, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxVideoControlBytes {
		return nil, fmt.Errorf("videoMuted size")
	}
	var in VideoMutedMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("videoMuted json: %w", err)
	}
	out := VideoMutedMessage{
		Type:   VideoMutedMessageType,
		UserID: userID,
		Muted:  in.Muted,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxVideoControlBytes {
		return nil, fmt.Errorf("videoMuted stamped size")
	}
	return b, nil
}

// StampVideoOrientation forces type/identity from the authenticated peer.
func StampVideoOrientation(userID string, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxVideoControlBytes {
		return nil, fmt.Errorf("videoOrientation size")
	}
	var in VideoOrientationMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("videoOrientation json: %w", err)
	}
	if math.IsNaN(in.RotationDeg) || math.IsInf(in.RotationDeg, 0) {
		return nil, fmt.Errorf("videoOrientation rotationDeg")
	}
	out := VideoOrientationMessage{
		Type:        VideoOrientationMessageType,
		UserID:      userID,
		RotationDeg: in.RotationDeg,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxVideoControlBytes {
		return nil, fmt.Errorf("videoOrientation stamped size")
	}
	return b, nil
}
