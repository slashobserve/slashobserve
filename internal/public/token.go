package public

import (
	"strings"

	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const (
	tokenPrefix       = "pub_"
	displayPrefixSize = 12
)

func displayPrefix(plaintext string) string {
	return plaintext[:displayPrefixSize]
}

func validTokenShape(s string) bool {
	if !opaquetoken.Shaped(s, tokenPrefix) {
		return false
	}
	body := strings.TrimPrefix(s, tokenPrefix)
	for i := range len(body) {
		if !isBase64URL(body[i]) {
			return false
		}
	}
	return true
}

func isBase64URL(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_'
}
