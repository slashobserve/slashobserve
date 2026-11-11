package mcpauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

const (
	challengeLength   = 43
	minVerifierLength = 43
	maxVerifierLength = 128
)

func pkceMatches(verifier, challenge string) bool {
	if len(verifier) < minVerifierLength || len(verifier) > maxVerifierLength {
		return false
	}
	for i := range len(verifier) {
		if !isVerifierChar(verifier[i]) {
			return false
		}
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(want), []byte(challenge)) == 1
}

func validChallenge(challenge string) bool {
	if len(challenge) != challengeLength {
		return false
	}
	for i := range len(challenge) {
		c := challenge[i]
		if !isAlnum(c) && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func isVerifierChar(c byte) bool {
	return isAlnum(c) || c == '-' || c == '.' || c == '_' || c == '~'
}

func isAlnum(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9'
}
