package webrtc

import (
	"strings"

	"github.com/pion/webrtc/v4"
)

// remoteCodecCaps is the set of RTP codec names a peer advertised in their SDP
// offer (a=rtpmap), keyed by media kind. Names are lowercase subtypes
// (e.g. "vp8", "h264", "opus") without the "video/" / "audio/" prefix.
type remoteCodecCaps struct {
	video map[string]struct{}
	audio map[string]struct{}
}

// parseOfferCodecCaps extracts codec names from video/audio m-lines in an SDP offer.
func parseOfferCodecCaps(sdp string) remoteCodecCaps {
	caps := remoteCodecCaps{
		video: make(map[string]struct{}),
		audio: make(map[string]struct{}),
	}
	section := ""
	for _, raw := range strings.Split(sdp, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "m=") {
			media := strings.TrimPrefix(line, "m=")
			if i := strings.IndexByte(media, ' '); i >= 0 {
				media = media[:i]
			}
			switch media {
			case "video", "audio":
				section = media
			default:
				section = ""
			}
			continue
		}
		if section == "" || !strings.HasPrefix(line, "a=rtpmap:") {
			continue
		}
		rest := strings.TrimPrefix(line, "a=rtpmap:")
		space := strings.IndexByte(rest, ' ')
		if space < 0 {
			continue
		}
		codecPart := rest[space+1:]
		name := codecPart
		if slash := strings.IndexByte(codecPart, '/'); slash >= 0 {
			name = codecPart[:slash]
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if section == "video" {
			caps.video[name] = struct{}{}
		} else {
			caps.audio[name] = struct{}{}
		}
	}
	return caps
}

func mimeSubtype(mimeType string) string {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.LastIndexByte(mt, '/'); i >= 0 {
		return mt[i+1:]
	}
	return mt
}

// supportsTrack reports whether the remote offer listed this track's codec.
// Empty caps for a kind mean the peer has no compatible m-line — skip relay.
// Board video is VP8-only on the SFU MediaEngine; non-VP8 video is never relayed.
func (c remoteCodecCaps) supportsTrack(tr *webrtc.TrackLocalStaticRTP) bool {
	if tr == nil {
		return false
	}
	codec := tr.Codec()
	sub := mimeSubtype(codec.MimeType)
	if sub == "" {
		return false
	}
	mt := strings.ToLower(codec.MimeType)
	switch {
	case strings.HasPrefix(mt, "video/"):
		if sub != "vp8" {
			return false
		}
		_, ok := c.video[sub]
		return ok
	case strings.HasPrefix(mt, "audio/"):
		_, ok := c.audio[sub]
		return ok
	default:
		return false
	}
}
