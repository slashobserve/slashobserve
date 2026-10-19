package mcp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	labelPrefix   = "so1"
	maxLabelBytes = 512
	nonceBytes    = 16
)

func clientLabel(name string) string {
	nonce := make([]byte, nonceBytes)
	_, _ = rand.Read(nonce)
	return labelPrefix + "." + hex.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString([]byte(name))
}

func labelClient(header string) string {
	if len(header) > maxLabelBytes {
		return ""
	}
	parts := strings.SplitN(header, ".", 3)
	if len(parts) != 3 || parts[0] != labelPrefix || !isLowerHex(parts[1], nonceBytes*2) {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ""
	}
	return origin.CleanClient(string(decoded))
}

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
