package webrtc

import (
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestPresenceAPIRegistersVP8AndOpusOnly(t *testing.T) {
	api, err := presenceAPI()
	if err != nil {
		t.Fatal(err)
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()

	// Offer from a "client" that lists H264 + VP8; SFU answer must not keep H264.
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video",
		"test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AddTrack(track); err != nil {
		t.Fatal(err)
	}
	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	sdp := strings.ToLower(answer.SDP)
	if !strings.Contains(sdp, "vp8") {
		t.Fatalf("answer missing VP8: %s", answer.SDP)
	}
	if strings.Contains(sdp, "h264") {
		t.Fatalf("VP8-only MediaEngine answered with H264: %s", answer.SDP)
	}
}
