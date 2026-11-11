package webhooks

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/sealed"
)

const (
	secretPurpose = "slashobserve webhook signing secret v1"
	secretPrefix  = "whsec_"
	secretBytes   = 32
)

func newSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("webhook secret: %w", err)
	}
	return secretPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func sealSecret(key sealed.Key, id pgtype.UUID, secret string) ([]byte, error) {
	ciphertext, err := key.Seal(secretPurpose, id, []byte(secret))
	if errors.Is(err, sealed.ErrNoKey) {
		return nil, ErrNoSecretKey
	}
	return ciphertext, err
}

func openSecret(key sealed.Key, id pgtype.UUID, ciphertext []byte) (string, error) {
	plain, err := key.Open(secretPurpose, id, ciphertext)
	switch {
	case errors.Is(err, sealed.ErrNoKey):
		return "", ErrNoSecretKey
	case errors.Is(err, sealed.ErrUnreadable):
		return "", ErrSecretUnreadable
	}
	return string(plain), err
}
