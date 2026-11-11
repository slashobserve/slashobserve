package sealed

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

var (
	ErrNoKey      = errors.New("no secret key is set")
	ErrUnreadable = errors.New("the sealed value cannot be read with the current secret key")
)

type Key []byte

func (k Key) Set() bool {
	return len(k) > 0
}

func (k Key) aead(purpose string) (cipher.AEAD, error) {
	if !k.Set() {
		return nil, ErrNoKey
	}
	key, err := hkdf.Key(sha256.New, k, nil, purpose, 32)
	if err != nil {
		return nil, fmt.Errorf("derive secret key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func (k Key) Seal(purpose string, id pgtype.UUID, plain []byte) ([]byte, error) {
	aead, err := k.aead(purpose)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secret nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plain, id.Bytes[:]), nil
}

func (k Key) Open(purpose string, id pgtype.UUID, sealed []byte) ([]byte, error) {
	aead, err := k.aead(purpose)
	if err != nil {
		return nil, err
	}
	size := aead.NonceSize()
	if len(sealed) < size {
		return nil, ErrUnreadable
	}
	plain, err := aead.Open(nil, sealed[:size], sealed[size:], id.Bytes[:])
	if err != nil {
		return nil, ErrUnreadable
	}
	return plain, nil
}
