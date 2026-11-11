package sealed_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/sealed"
)

const purpose = "test purpose v1"

var (
	key = sealed.Key("0123456789abcdef0123456789abcdef")
	id  = pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
)

func TestSealedValueOpensWithTheSameKeyPurposeAndID(t *testing.T) {
	ciphertext, err := key.Seal(purpose, id, []byte("glsa_token"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("glsa_token")) {
		t.Fatal("ciphertext holds the plain value")
	}
	plain, err := key.Open(purpose, id, ciphertext)
	if err != nil || string(plain) != "glsa_token" {
		t.Fatalf("open = %q, %v", plain, err)
	}
}

func TestSealingTwiceGivesDifferentCiphertexts(t *testing.T) {
	a, _ := key.Seal(purpose, id, []byte("x"))
	b, _ := key.Seal(purpose, id, []byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("two seals of one value are identical, the nonce is not random")
	}
}

func TestAnotherPurposeIDOrKeyCannotOpen(t *testing.T) {
	ciphertext, err := key.Seal(purpose, id, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	otherID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	cases := map[string]func() ([]byte, error){
		"purpose": func() ([]byte, error) { return key.Open("other purpose v1", id, ciphertext) },
		"id":      func() ([]byte, error) { return key.Open(purpose, otherID, ciphertext) },
		"key": func() ([]byte, error) {
			return sealed.Key("ffffffffffffffffffffffffffffffff").Open(purpose, id, ciphertext)
		},
	}
	for name, open := range cases {
		if _, err := open(); !errors.Is(err, sealed.ErrUnreadable) {
			t.Errorf("%s: error %v, want ErrUnreadable", name, err)
		}
	}
}

func TestEmptyKeyIsRefused(t *testing.T) {
	var none sealed.Key
	if none.Set() {
		t.Fatal("an empty key reports Set")
	}
	if _, err := none.Seal(purpose, id, []byte("x")); !errors.Is(err, sealed.ErrNoKey) {
		t.Fatalf("seal error %v, want ErrNoKey", err)
	}
	if _, err := none.Open(purpose, id, []byte("x")); !errors.Is(err, sealed.ErrNoKey) {
		t.Fatalf("open error %v, want ErrNoKey", err)
	}
}

func TestTruncatedInputIsUnreadable(t *testing.T) {
	ciphertext, err := key.Seal(purpose, id, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 5, 11, len(ciphertext) - 1} {
		if _, err := key.Open(purpose, id, ciphertext[:n]); !errors.Is(err, sealed.ErrUnreadable) {
			t.Errorf("length %d: error %v, want ErrUnreadable", n, err)
		}
	}
}

func TestStoredGrafanaTokenFormatStillOpens(t *testing.T) {
	const grafanaLabel = "slashobserve grafana connection token v1"
	derived, err := hkdf.Key(sha256.New, key, nil, grafanaLabel, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := []byte("fixed-nonce!")
	stored := gcm.Seal(append([]byte(nil), nonce...), nonce, []byte("glsa_stored_token"), id.Bytes[:])

	plain, err := key.Open(grafanaLabel, id, stored)
	if err != nil || string(plain) != "glsa_stored_token" {
		t.Fatalf("open = %q, %v", plain, err)
	}
}
