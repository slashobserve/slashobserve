package public

import (
	"bytes"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func TestNewTokenShape(t *testing.T) {
	plaintext, hash, err := opaquetoken.New(tokenPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(plaintext) != 47 || !strings.HasPrefix(plaintext, "pub_") {
		t.Fatalf("token %q is not pub_ plus 43 characters", plaintext)
	}
	if !validTokenShape(plaintext) {
		t.Fatalf("a new token fails the shape check")
	}
	if !bytes.Equal(hash, opaquetoken.Hash(plaintext)) || len(hash) != 32 {
		t.Fatalf("the stored hash is not the SHA-256 of the token")
	}
	if got := displayPrefix(plaintext); got != plaintext[:12] || len(got) != 12 {
		t.Fatalf("display prefix %q", got)
	}
	other, _, err := opaquetoken.New(tokenPrefix)
	if err != nil || other == plaintext {
		t.Fatalf("two tokens are equal or failed: %v", err)
	}
}

func TestValidTokenShape(t *testing.T) {
	body := strings.Repeat("A", 43)
	cases := []struct {
		name  string
		token string
		want  bool
	}{
		{"prefix and 43 base64url characters", "pub_" + body, true},
		{"dash and underscore", "pub_" + strings.Repeat("-_", 21) + "z", true},
		{"one character short", "pub_" + body[:42], false},
		{"one character long", "pub_" + body + "A", false},
		{"padding", "pub_" + body[:42] + "=", false},
		{"standard base64 plus", "pub_" + body[:42] + "+", false},
		{"slash", "pub_" + body[:42] + "/", false},
		{"another prefix", "key_" + body, false},
		{"no prefix", body + "abcd", false},
		{"empty", "", false},
		{"non-ascii", "pub_" + body[:41] + "é", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validTokenShape(tc.token); got != tc.want {
				t.Fatalf("validTokenShape(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}
