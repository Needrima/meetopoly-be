package webrtc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// HubChatMessageType is the locked DC type for ephemeral hub chat (Phase 17.0).
	HubChatMessageType = "hubChat"
	// MaxHubChatBytes rejects oversized hub chat DC payloads.
	MaxHubChatBytes = 512
	// MaxHubChatRunes caps visible message length (Unicode runes).
	MaxHubChatRunes = 280
	// MaxHubChatHz caps stamped hub chat fan-out per peer (Phase 17.0).
	MaxHubChatHz = 3
)

var hubChatMinInterval = time.Second / MaxHubChatHz

// HubChatMessage is an ephemeral hub chat line (no persistence).
// Identity and t are stamped by the SFU from the authenticated peer.
type HubChatMessage struct {
	Type     string `json:"type"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Text     string `json:"text"`
	// T is server send time (unix ms) for client ordering.
	T int64 `json:"t"`
}

// StampHubChat parses a client DC payload, forces type/identity from the peer,
// trims text, and returns canonical JSON for fan-out.
func StampHubChat(userID, username string, raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxHubChatBytes {
		return nil, fmt.Errorf("hubChat size")
	}
	var in HubChatMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("hubChat json: %w", err)
	}
	if in.Type != "" && in.Type != HubChatMessageType {
		return nil, fmt.Errorf("hubChat type")
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, fmt.Errorf("hubChat empty")
	}
	if utf8.RuneCountInString(text) > MaxHubChatRunes {
		return nil, fmt.Errorf("hubChat too long")
	}
	out := HubChatMessage{
		Type:     HubChatMessageType,
		UserID:   userID,
		Username: username,
		Text:     text,
		T:        time.Now().UTC().UnixMilli(),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxHubChatBytes {
		return nil, fmt.Errorf("hubChat stamped size")
	}
	return b, nil
}

// IsHubChatDC reports the hub chat type (bypasses pose Hz; uses allowHubChat).
func IsHubChatDC(msgType string) bool {
	return msgType == HubChatMessageType
}
