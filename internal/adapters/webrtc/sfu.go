package webrtc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	// PresenceDataChannelLabel carries board presence poses (Phase 7.1+).
	PresenceDataChannelLabel = "presence"
	// MaxPoseHz caps stamped pose fan-out per peer (Phase 7.4). Clients send ~10 Hz.
	MaxPoseHz = 20
	// MaxHubPeers is the locked cap for a single hub SFU room (Phase 8.3).
	MaxHubPeers = 16
)

var (
	poseMinInterval = time.Second / MaxPoseHz
	// ErrHubFull is returned when Attach would exceed MaxHubPeers on a hub room.
	ErrHubFull = fmt.Errorf("hub room full")
)

// SignalWriter sends JSON signaling messages to one peer's WebSocket.
type SignalWriter interface {
	WriteJSON(v any) error
}

// PeerInfo is a roster entry for welcome / join / leave events.
type PeerInfo struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// ICEServerJSON is exposed to clients in the welcome message.
type ICEServerJSON struct {
	URLs []string `json:"urls"`
}

// SFU holds in-memory board presence rooms (one PeerConnection per user).
type SFU struct {
	mu         sync.Mutex
	rooms      map[string]*room // roomID → room
	iceServers []webrtc.ICEServer
}

type room struct {
	id    string
	peers map[string]*peer // userID → peer
}

type peer struct {
	userID     string
	username   string
	pc         *webrtc.PeerConnection
	dc         *webrtc.DataChannel
	signal     SignalWriter
	lastPoseAt time.Time
}

// NewSFU builds a memory SFU with Google public STUN (no TURN in Phase 7.0).
func NewSFU() *SFU {
	return &SFU{
		rooms: make(map[string]*room),
		iceServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
}

// BoardRoomID returns the locked room id for a game board presence room.
func BoardRoomID(gameID string) string {
	return "board:" + gameID
}

// HubRoomID returns the locked Phase 8 hub presence room id.
// If hubID is already prefixed with "hub:", it is returned unchanged.
func HubRoomID(hubID string) string {
	hubID = strings.TrimSpace(hubID)
	if hubID == "" {
		return "hub:"
	}
	if strings.HasPrefix(hubID, "hub:") {
		return hubID
	}
	return "hub:" + hubID
}

// ICEServersJSON returns STUN config for the welcome payload.
func (s *SFU) ICEServersJSON() []ICEServerJSON {
	out := make([]ICEServerJSON, 0, len(s.iceServers))
	for _, ice := range s.iceServers {
		out = append(out, ICEServerJSON{URLs: append([]string(nil), ice.URLs...)})
	}
	return out
}

// Roster lists peers currently in the room (excluding excludeUserID when set).
func (s *SFU) Roster(roomID, excludeUserID string) []PeerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return nil
	}
	out := make([]PeerInfo, 0, len(r.peers))
	for id, p := range r.peers {
		if excludeUserID != "" && id == excludeUserID {
			continue
		}
		out = append(out, PeerInfo{UserID: p.userID, Username: p.username})
	}
	return out
}

// Attach registers a signaling sink for userID before SDP exchange.
// If the user was already attached, the previous PeerConnection is closed.
// Hub rooms (`hub:…`) reject a new userId when the room already has MaxHubPeers.
func (s *SFU) Attach(roomID, userID, username string, signal SignalWriter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.rooms[roomID]
	if r == nil {
		r = &room{id: roomID, peers: make(map[string]*peer)}
		s.rooms[roomID] = r
	}
	if old, ok := r.peers[userID]; ok {
		s.closePeerLocked(old)
		delete(r.peers, userID)
	} else if strings.HasPrefix(roomID, "hub:") && len(r.peers) >= MaxHubPeers {
		return ErrHubFull
	}
	r.peers[userID] = &peer{
		userID:   userID,
		username: username,
		signal:   signal,
	}
	return nil
}

// Detach removes the peer and closes their PeerConnection.
// When signal is non-nil, Detach is a no-op if that user was superseded by a newer Attach
// (reconnect) so an old WebSocket teardown cannot erase the replacement peer.
func (s *SFU) Detach(roomID, userID string, signal SignalWriter) (info PeerInfo, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return PeerInfo{}, false
	}
	p, exists := r.peers[userID]
	if !exists {
		return PeerInfo{}, false
	}
	if signal != nil && p.signal != signal {
		return PeerInfo{}, false
	}
	info = PeerInfo{UserID: p.userID, Username: p.username}
	s.closePeerLocked(p)
	delete(r.peers, userID)
	if len(r.peers) == 0 {
		delete(s.rooms, roomID)
	}
	return info, true
}

// HandleOffer accepts a client SDP offer, answers, and wires ICE + idle DataChannel.
func (s *SFU) HandleOffer(roomID, userID string, sdp string) error {
	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil {
		s.mu.Unlock()
		return fmt.Errorf("room not found")
	}
	p := r.peers[userID]
	if p == nil {
		s.mu.Unlock()
		return fmt.Errorf("peer not attached")
	}
	signal := p.signal
	s.mu.Unlock()

	cfg := webrtc.Configuration{ICEServers: s.iceServers}
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return fmt.Errorf("new peer connection: %w", err)
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cand := c.ToJSON()
		_ = signal.WriteJSON(map[string]any{
			"type":      "ice",
			"candidate": cand,
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		slog.Debug("presence pc state",
			"roomId", roomID,
			"userId", userID,
			"state", state.String(),
		)
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != PresenceDataChannelLabel {
			return
		}
		s.mu.Lock()
		peer := s.peerLocked(roomID, userID)
		if peer == nil {
			s.mu.Unlock()
			return
		}
		peer.dc = dc
		fromUser := peer.userID
		fromName := peer.username
		s.mu.Unlock()

		dc.OnOpen(func() {
			slog.Info("presence datachannel open",
				"roomId", roomID,
				"userId", fromUser,
				"label", dc.Label(),
			)
		})
		// Phase 7.1–7.4: stamp identity, rate-limit, fan out (no collision validation — 7.3 skipped).
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if !s.allowPose(roomID, fromUser) {
				return
			}
			stamped, err := StampPose(fromUser, fromName, msg.Data)
			if err != nil {
				return
			}
			s.forwardPose(roomID, fromUser, stamped)
		})
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return fmt.Errorf("set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return fmt.Errorf("set local description: %w", err)
	}

	s.mu.Lock()
	p = s.peerLocked(roomID, userID)
	if p == nil || p.signal != signal {
		s.mu.Unlock()
		_ = pc.Close()
		return fmt.Errorf("peer gone during offer")
	}
	if p.pc != nil {
		s.closePeerLocked(p)
	}
	p.pc = pc
	s.mu.Unlock()

	local := pc.LocalDescription()
	if local == nil {
		return fmt.Errorf("missing local description")
	}
	return signal.WriteJSON(map[string]any{
		"type": "answer",
		"sdp":  local.SDP,
	})
}

// AddICE applies a remote ICE candidate from the client.
func (s *SFU) AddICE(roomID, userID string, candidate webrtc.ICECandidateInit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peerLocked(roomID, userID)
	if p == nil || p.pc == nil {
		return fmt.Errorf("no peer connection")
	}
	return p.pc.AddICECandidate(candidate)
}

func (s *SFU) peerLocked(roomID, userID string) *peer {
	r := s.rooms[roomID]
	if r == nil {
		return nil
	}
	return r.peers[userID]
}

// allowPose returns true if this peer may fan out another pose (MaxPoseHz).
func (s *SFU) allowPose(roomID, userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peerLocked(roomID, userID)
	if p == nil {
		return false
	}
	now := time.Now()
	if !p.lastPoseAt.IsZero() && now.Sub(p.lastPoseAt) < poseMinInterval {
		return false
	}
	p.lastPoseAt = now
	return true
}

// forwardPose sends stamped pose bytes to every other open DataChannel in the room.
func (s *SFU) forwardPose(roomID, fromUserID string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return
	}
	for id, p := range r.peers {
		if id == fromUserID || p.dc == nil {
			continue
		}
		if p.dc.ReadyState() != webrtc.DataChannelStateOpen {
			continue
		}
		if err := p.dc.Send(payload); err != nil {
			slog.Debug("presence pose send",
				"roomId", roomID,
				"toUserId", id,
				"err", err,
			)
		}
	}
}

func (s *SFU) closePeerLocked(p *peer) {
	if p == nil {
		return
	}
	if p.dc != nil {
		_ = p.dc.Close()
		p.dc = nil
	}
	if p.pc != nil {
		_ = p.pc.Close()
		p.pc = nil
	}
}

// MarshalCandidate helps tests / debug.
func MarshalCandidate(c webrtc.ICECandidateInit) ([]byte, error) {
	return json.Marshal(c)
}
