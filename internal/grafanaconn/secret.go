package grafanaconn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

const tokenKeyLabel = "slashobserve grafana connection token v1"

var (
	ErrNoSecretKey     = errors.New("SLASHOBSERVE_SECRET_KEY must be set to store or use a Grafana connection token")
	ErrUnreadableToken = errors.New("the stored token cannot be read with the current SLASHOBSERVE_SECRET_KEY; set the connection's token again")
)

type secretKey []byte

func (k secretKey) aead() (cipher.AEAD, error) {
	if len(k) == 0 {
		return nil, ErrNoSecretKey
	}
	key, err := hkdf.Key(sha256.New, k, nil, tokenKeyLabel, 32)
	if err != nil {
		return nil, fmt.Errorf("derive token key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("token cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func (k secretKey) seal(id pgtype.UUID, token string) ([]byte, error) {
	aead, err := k.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("token nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, []byte(token), id.Bytes[:]), nil
}

func (k secretKey) open(id pgtype.UUID, sealed []byte) (string, error) {
	aead, err := k.aead()
	if err != nil {
		return "", err
	}
	size := aead.NonceSize()
	if len(sealed) < size {
		return "", ErrUnreadableToken
	}
	plain, err := aead.Open(nil, sealed[:size], sealed[size:], id.Bytes[:])
	if err != nil {
		return "", ErrUnreadableToken
	}
	return string(plain), nil
}
