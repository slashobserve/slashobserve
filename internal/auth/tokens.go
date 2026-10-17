package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func newToken(prefix string) (plaintext string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, hashToken(plaintext), nil
}

func hashToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}
