package webrtc

import (
	"encoding/json"
	"fmt"
)

const (
	// PoseMessageType is the locked DataChannel payload type for board avatars (Phase 7.1).
	PoseMessageType = "pose"
	// MaxPoseBytes rejects oversized DC payloads (abuse / garbage).
	MaxPoseBytes = 512
)

// PoseMessage is the locked board-avatar pose shape (Phase 7.1).
// Coordinates are board-normalized 0..1 (same space as mobile poseNorm).
// Identity fields are always stamped by the SFU from the sending peer — clients must not be trusted for userId.
type PoseMessage struct {
	Type     string   `json:"type"`
	UserID   string   `json:"userId"`
	Username string   `json:"username"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	Rot      *float64 `json:"rot,omitempty"`
	// T is optional client send time (unix ms) for interpolation in Phase 7.2.
	T *int64 `json:"t,omitempty"`
}

// StampPose parses a client DC payload, forces type/identity from the authenticated peer,
// and returns canonical JSON for fan-out. Position fields are not validated until Phase 7.3.
func StampPose(userID, username string, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxPoseBytes {
		return nil, fmt.Errorf("pose size")
	}
	var in PoseMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("pose json: %w", err)
	}
	if in.Type != "" && in.Type != PoseMessageType {
		return nil, fmt.Errorf("pose type")
	}
	out := PoseMessage{
		Type:     PoseMessageType,
		UserID:   userID,
		Username: username,
		X:        in.X,
		Y:        in.Y,
		Rot:      in.Rot,
		T:        in.T,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxPoseBytes {
		return nil, fmt.Errorf("pose stamped size")
	}
	return b, nil
}
