package webrtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestParseOfferCodecCaps(t *testing.T) {
	sdp := "" +
		"v=0\r\n" +
		"m=audio 9 UDP/TLS/RTP/SAVPF 111 0\r\n" +
		"a=rtpmap:111 opus/48000/2\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96 97\r\n" +
		"a=rtpmap:96 VP8/90000\r\n" +
		"a=rtpmap:97 H264/90000\r\n" +
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n"

	caps := parseOfferCodecCaps(sdp)
	if _, ok := caps.video["vp8"]; !ok {
		t.Fatal("want vp8")
	}
	if _, ok := caps.video["h264"]; !ok {
		t.Fatal("want h264")
	}
	if _, ok := caps.audio["opus"]; !ok {
		t.Fatal("want opus")
	}
	if _, ok := caps.audio["pcmu"]; !ok {
		t.Fatal("want pcmu")
	}
}

func TestParseOfferCodecCapsVP8Only(t *testing.T) {
	sdp := "" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"

	caps := parseOfferCodecCaps(sdp)
	if _, ok := caps.video["vp8"]; !ok {
		t.Fatal("want vp8")
	}
	if _, ok := caps.video["h264"]; ok {
		t.Fatal("did not want h264")
	}
}

func TestSupportsTrack(t *testing.T) {
	caps := remoteCodecCaps{
		video: map[string]struct{}{"vp8": {}},
		audio: map[string]struct{}{"opus": {}},
	}
	vp8, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	h264, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !caps.supportsTrack(vp8) {
		t.Fatal("vp8 should be supported")
	}
	if caps.supportsTrack(h264) {
		t.Fatal("h264 should be skipped")
	}
}

func TestMimeSubtype(t *testing.T) {
	if got := mimeSubtype("video/H264"); got != "h264" {
		t.Fatalf("got %s", got)
	}
	if got := mimeSubtype(webrtc.MimeTypeVP8); got != "vp8" {
		t.Fatalf("got %s", got)
	}
}
