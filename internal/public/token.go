package public

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

const (
	tokenPrefix       = "pub_"
	tokenRandomBytes  = 32
	displayPrefixSize = 12
)

var tokenBodyLength = base64.RawURLEncoding.EncodedLen(tokenRandomBytes)

func newToken() (plaintext string, hash []byte, err error) {
	b := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plaintext = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, hashToken(plaintext), nil
}

func hashToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}

func displayPrefix(plaintext string) string {
	return plaintext[:displayPrefixSize]
}

func validTokenShape(s string) bool {
	body, ok := strings.CutPrefix(s, tokenPrefix)
	if !ok || len(body) != tokenBodyLength {
		return false
	}
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
