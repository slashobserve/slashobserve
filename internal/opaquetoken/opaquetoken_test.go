package opaquetoken

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewHasPrefixAndRandomBody(t *testing.T) {
	plaintext, hash, err := New("soa_")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plaintext, "soa_") || len(plaintext) != len("soa_")+43 {
		t.Fatalf("token = %q, want soa_ plus 43 characters", plaintext)
	}
	if len(hash) != 32 || !bytes.Equal(hash, Hash(plaintext)) {
		t.Fatalf("hash does not match Hash(plaintext)")
	}
	other, _, err := New("soa_")
	if err != nil {
		t.Fatal(err)
	}
	if other == plaintext {
		t.Fatal("two tokens are equal")
	}
}

func TestShaped(t *testing.T) {
	plaintext, _, err := New("sok_")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		token  string
		prefix string
		want   bool
	}{
		{"new token", plaintext, "sok_", true},
		{"wrong prefix", plaintext, "soa_", false},
		{"one short", plaintext[:len(plaintext)-1], "sok_", false},
		{"one long", plaintext + "a", "sok_", false},
		{"empty", "", "sok_", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Shaped(c.token, c.prefix); got != c.want {
				t.Fatalf("Shaped(%q, %q) = %v, want %v", c.token, c.prefix, got, c.want)
			}
		})
	}
}
