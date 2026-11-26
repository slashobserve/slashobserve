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

const appsFlag = "ui"

func clientLabel(name string, apps bool) string {
	nonce := make([]byte, nonceBytes)
	_, _ = rand.Read(nonce)
	label := labelPrefix + "." + hex.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString([]byte(name))
	if apps {
		label += "." + appsFlag
	}
	return label
}

type parsedLabel struct {
	client string
	apps   bool
}

func parseLabel(header string) parsedLabel {
	if len(header) > maxLabelBytes {
		return parsedLabel{}
	}
	parts := strings.Split(header, ".")
	if len(parts) < 3 || len(parts) > 4 || parts[0] != labelPrefix || !isLowerHex(parts[1], nonceBytes*2) {
		return parsedLabel{}
	}
	if len(parts) == 4 && parts[3] != appsFlag {
		return parsedLabel{}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return parsedLabel{}
	}
	return parsedLabel{client: origin.CleanClient(string(decoded)), apps: len(parts) == 4}
}

func labelClient(header string) string { return parseLabel(header).client }

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
