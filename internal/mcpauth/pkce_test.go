package mcpauth

import (
	"strings"
	"testing"
)

func TestPKCEMatchesTheRFCVector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !pkceMatches(verifier, challenge) {
		t.Fatal("the RFC 7636 appendix B vector does not match")
	}
	if !validChallenge(challenge) {
		t.Fatal("the RFC vector's challenge is refused")
	}
}

func TestPKCERefusesBadVerifiers(t *testing.T) {
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	cases := map[string]string{
		"wrong":         "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXX",
		"short":         "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjX",
		"too long":      strings.Repeat("a", 129),
		"bad character": "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEj+k",
		"empty":         "",
	}
	for name, verifier := range cases {
		if pkceMatches(verifier, challenge) {
			t.Errorf("%s verifier matched", name)
		}
	}
}

func TestValidChallenge(t *testing.T) {
	good := strings.Repeat("A", 43)
	for _, bad := range []string{"", good[:42], good + "A", good[:42] + "=", good[:42] + "+", good[:42] + "/"} {
		if validChallenge(bad) {
			t.Errorf("%q accepted as a challenge", bad)
		}
	}
	if !validChallenge(good) || !validChallenge(good[:41]+"-_") {
		t.Error("a 43 character base64url challenge was refused")
	}
}
