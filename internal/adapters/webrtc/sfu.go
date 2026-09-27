package webrtc

import (
	"encoding/json"
	"fmt"
	"io"
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
	// Country is ISO 3166-1 alpha-2 when known (Phase 9.0a).
	Country string `json:"country,omitempty"`
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
	// audioPubs: hub-only published mic relays (Phase 10.0). Key = publisher userID.
	audioPubs map[string]*hubAudioPub
}

type hubAudioPub struct {
	fromUserID string
	track      *webrtc.TrackLocalStaticRTP
	stop       chan struct{}
}

type peer struct {
	userID     string
	username   string
	country    string
	pc         *webrtc.PeerConnection
	dc         *webrtc.DataChannel
	signal     SignalWriter
	lastPoseAt time.Time
	negotiating bool
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

// IsHubRoom reports whether roomID is a hub presence room (Phase 8 / 10).
func IsHubRoom(roomID string) bool {
	return strings.HasPrefix(roomID, "hub:")
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
		out = append(out, PeerInfo{UserID: p.userID, Username: p.username, Country: p.country})
	}
	return out
}

// HubAudioPublisherCount returns how many hub audio pubs are active (tests / debug).
func (s *SFU) HubAudioPublisherCount(roomID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil || r.audioPubs == nil {
		return 0
	}
	return len(r.audioPubs)
}

// Attach registers a signaling sink for userID before SDP exchange.
// If the user was already attached, the previous PeerConnection is closed.
// Hub rooms (`hub:…`) reject a new userId when the room already has MaxHubPeers.
// country is optional ISO 3166-1 alpha-2 (Phase 9.0a).
func (s *SFU) Attach(roomID, userID, username, country string, signal SignalWriter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.rooms[roomID]
	if r == nil {
		r = &room{id: roomID, peers: make(map[string]*peer)}
		if IsHubRoom(roomID) {
			r.audioPubs = make(map[string]*hubAudioPub)
		}
		s.rooms[roomID] = r
	}
	if old, ok := r.peers[userID]; ok {
		s.unpublishHubAudioLocked(r, userID)
		s.closePeerLocked(old)
		delete(r.peers, userID)
	} else if IsHubRoom(roomID) && len(r.peers) >= MaxHubPeers {
		return ErrHubFull
	}
	r.peers[userID] = &peer{
		userID:   userID,
		username: username,
		country:  strings.ToUpper(strings.TrimSpace(country)),
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
	info = PeerInfo{UserID: p.userID, Username: p.username, Country: p.country}
	s.unpublishHubAudioLocked(r, userID)
	s.closePeerLocked(p)
	delete(r.peers, userID)
	if len(r.peers) == 0 {
		delete(s.rooms, roomID)
	}
	return info, true
}

// HandleOffer accepts a client SDP offer, answers, and wires ICE + DataChannel.
// Hub rooms also attach existing audio pubs before answering (Phase 10.0).
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
	hub := IsHubRoom(roomID)
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

	if hub {
		pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			s.onHubTrack(roomID, userID, remote)
		})
	}

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return fmt.Errorf("set remote description: %w", err)
	}

	if hub {
		if err := s.addExistingHubAudioToPC(roomID, userID, pc); err != nil {
			slog.Warn("hub add existing audio", "roomId", roomID, "userId", userID, "err", err)
		}
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

// HandleAnswer applies a client SDP answer to an SFU-initiated renegotiation offer (hub audio).
func (s *SFU) HandleAnswer(roomID, userID string, sdp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peerLocked(roomID, userID)
	if p == nil || p.pc == nil {
		return fmt.Errorf("no peer connection")
	}
	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}
	if err := p.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}
	p.negotiating = false
	return nil
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

func (s *SFU) onHubTrack(roomID, fromUserID string, remote *webrtc.TrackRemote) {
	if remote.Kind() != webrtc.RTPCodecTypeAudio {
		slog.Debug("hub ignoring non-audio track",
			"roomId", roomID,
			"userId", fromUserID,
			"kind", remote.Kind().String(),
		)
		return
	}

	local, err := webrtc.NewTrackLocalStaticRTP(
		remote.Codec().RTPCodecCapability,
		"audio",
		"hub-"+fromUserID,
	)
	if err != nil {
		slog.Warn("hub local track", "roomId", roomID, "userId", fromUserID, "err", err)
		return
	}

	stop := make(chan struct{})
	pub := &hubAudioPub{fromUserID: fromUserID, track: local, stop: stop}

	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil || r.audioPubs == nil {
		s.mu.Unlock()
		return
	}
	if old := r.audioPubs[fromUserID]; old != nil {
		s.stopHubAudioPubLocked(old)
	}
	r.audioPubs[fromUserID] = pub

	// Snapshot peers that should receive this pub.
	type recv struct {
		userID string
		pc     *webrtc.PeerConnection
		signal SignalWriter
	}
	recvs := make([]recv, 0, len(r.peers))
	for id, p := range r.peers {
		if id == fromUserID || p.pc == nil {
			continue
		}
		recvs = append(recvs, recv{userID: id, pc: p.pc, signal: p.signal})
	}
	s.mu.Unlock()

	go relayHubAudio(remote, local, stop)

	for _, rv := range recvs {
		if _, err := rv.pc.AddTrack(local); err != nil {
			slog.Debug("hub AddTrack", "toUserId", rv.userID, "err", err)
			continue
		}
		if err := s.negotiateOffer(roomID, rv.userID); err != nil {
			slog.Debug("hub renegotiate", "toUserId", rv.userID, "err", err)
		}
	}

	slog.Info("hub audio published",
		"roomId", roomID,
		"userId", fromUserID,
		"receivers", len(recvs),
	)
}

func relayHubAudio(remote *webrtc.TrackRemote, local *webrtc.TrackLocalStaticRTP, stop <-chan struct{}) {
	buf := make([]byte, 1500)
	for {
		select {
		case <-stop:
			return
		default:
		}
		n, _, err := remote.Read(buf)
		if err != nil {
			if err != io.EOF {
				slog.Debug("hub audio read", "err", err)
			}
			return
		}
		if _, err := local.Write(buf[:n]); err != nil {
			return
		}
	}
}

func (s *SFU) addExistingHubAudioToPC(roomID, userID string, pc *webrtc.PeerConnection) error {
	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil || r.audioPubs == nil {
		s.mu.Unlock()
		return nil
	}
	tracks := make([]*webrtc.TrackLocalStaticRTP, 0, len(r.audioPubs))
	for pubUser, pub := range r.audioPubs {
		if pubUser == userID || pub == nil || pub.track == nil {
			continue
		}
		tracks = append(tracks, pub.track)
	}
	s.mu.Unlock()

	for _, tr := range tracks {
		if _, err := pc.AddTrack(tr); err != nil {
			return err
		}
	}
	return nil
}

func (s *SFU) negotiateOffer(roomID, userID string) error {
	s.mu.Lock()
	p := s.peerLocked(roomID, userID)
	if p == nil || p.pc == nil || p.signal == nil {
		s.mu.Unlock()
		return fmt.Errorf("no peer connection")
	}
	if p.negotiating {
		s.mu.Unlock()
		return nil
	}
	p.negotiating = true
	pc := p.pc
	signal := p.signal
	s.mu.Unlock()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		s.mu.Lock()
		if pe := s.peerLocked(roomID, userID); pe != nil {
			pe.negotiating = false
		}
		s.mu.Unlock()
		return err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		s.mu.Lock()
		if pe := s.peerLocked(roomID, userID); pe != nil {
			pe.negotiating = false
		}
		s.mu.Unlock()
		return err
	}
	local := pc.LocalDescription()
	if local == nil {
		s.mu.Lock()
		if pe := s.peerLocked(roomID, userID); pe != nil {
			pe.negotiating = false
		}
		s.mu.Unlock()
		return fmt.Errorf("missing local description")
	}
	return signal.WriteJSON(map[string]any{
		"type": "offer",
		"sdp":  local.SDP,
	})
}

func (s *SFU) unpublishHubAudioLocked(r *room, userID string) {
	if r == nil || r.audioPubs == nil {
		return
	}
	pub := r.audioPubs[userID]
	if pub == nil {
		return
	}
	s.stopHubAudioPubLocked(pub)
	delete(r.audioPubs, userID)
}

func (s *SFU) stopHubAudioPubLocked(pub *hubAudioPub) {
	if pub == nil {
		return
	}
	select {
	case <-pub.stop:
	default:
		close(pub.stop)
	}
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
	p.negotiating = false
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
