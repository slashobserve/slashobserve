package opaquetoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

const randomBytes = 32

var bodyLength = base64.RawURLEncoding.EncodedLen(randomBytes)

func New(prefix string) (plaintext string, hash []byte, err error) {
	b := make([]byte, randomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, Hash(plaintext), nil
}

func Hash(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}

func Shaped(token, prefix string) bool {
	return strings.HasPrefix(token, prefix) && len(token) == len(prefix)+bodyLength
}
