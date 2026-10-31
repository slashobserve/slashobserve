package cli

import (
	"net/url"
	"strings"
	"testing"
)

func TestLoginHintAndHelpNameTheAPIKeysPage(t *testing.T) {
	for _, tc := range []struct{ server, want string }{
		{"https://obs.example", "https://obs.example/keys"},
		{"https://obs.example/", "https://obs.example/keys"},
		{"https://corp.example/observe", "https://corp.example/observe/keys"},
	} {
		base, err := url.Parse(tc.server)
		if err != nil {
			t.Fatal(err)
		}
		if got := keysPageURL(base); got != tc.want {
			t.Errorf("keysPageURL(%s) = %s, want %s", tc.server, got, tc.want)
		}
	}
	if help := (&app{}).loginCommand().Long; !strings.Contains(help, "(<server>/keys)") {
		t.Errorf("login help = %q, want it to point at <server>/keys", help)
	}
}
