package table

import (
	"crypto/rand"
	"strings"
)

// Crockford Base32 without I/L/O/U — Phase 20 private lobby invite codes.
const inviteAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const inviteCodeLen = 8

// generateInviteCode returns an 8-character Crockford Base32 code.
func generateInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, inviteCodeLen)
	for i := range buf {
		out[i] = inviteAlphabet[int(buf[i])%len(inviteAlphabet)]
	}
	return string(out), nil
}

// normalizeInviteCode uppercases and trims; empty if not exactly 8 Crockford chars.
func normalizeInviteCode(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if len(s) != inviteCodeLen {
		return ""
	}
	for _, r := range s {
		if !strings.ContainsRune(inviteAlphabet, r) {
			return ""
		}
	}
	return s
}
