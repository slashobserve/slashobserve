package contextsources

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestCheckAddressAcceptsAnHTTPSDomainAndReturnsItsHost(t *testing.T) {
	cases := map[string]string{
		"https://mcp.example.com/mcp":      "mcp.example.com",
		"https://MCP.Example.com:8443/sse": "mcp.example.com",
		"https://example.com":              "example.com",
	}
	for raw, want := range cases {
		got, err := checkAddress(raw)
		if err != nil || got != want {
			t.Errorf("checkAddress(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestCheckAddressRefusesWhatTheEgressWouldRefuse(t *testing.T) {
	for _, raw := range []string{
		"", "mcp.example.com/mcp", "http://mcp.example.com/mcp", "ftp://mcp.example.com",
		"https://127.0.0.1/mcp", "https://[::1]/mcp", "https://10.0.0.5/mcp", "https://localhost/mcp",
		"https://user:pass@mcp.example.com/mcp", "https://mcp.example.com/mcp?key=abc", "https://mcp.example.com/mcp#frag",
		"https://mcp.example.com/mcp?", "https:///mcp", "https://" + strings.Repeat("a", maxURLBytes) + ".com",
	} {
		_, err := checkAddress(raw)
		var bad *InvalidError
		if !errors.As(err, &bad) || bad.Fields[0].Field != "url" {
			t.Errorf("checkAddress(%q) = %v, want an invalid url", raw, err)
		}
	}
}

func TestCheckUIDAndName(t *testing.T) {
	for _, uid := range []string{"docs", "Docs_1", "a-b", strings.Repeat("a", 40)} {
		if err := checkUID(uid); err != nil {
			t.Errorf("uid %q: %v", uid, err)
		}
	}
	for _, uid := range []string{"", "-docs", "a b", "a/b", strings.Repeat("a", 41)} {
		if checkUID(uid) == nil {
			t.Errorf("uid %q should be refused", uid)
		}
	}
	if checkName("Docs") != nil || checkName("") == nil || checkName("a\nb") == nil || checkName("a\u200bb") == nil || checkName(strings.Repeat("é", 101)) == nil || checkName(strings.Repeat("é", 100)) != nil {
		t.Error("name checks")
	}
}

func TestCheckAudienceAcceptsOnlyEveryoneAndAdmins(t *testing.T) {
	for _, a := range []datasource.Audience{datasource.AudienceEveryone, datasource.AudienceAdmins} {
		if err := checkAudience(a); err != nil {
			t.Errorf("%q: %v", a, err)
		}
	}
	if checkAudience("members") == nil || checkAudience("") == nil {
		t.Error("an unknown audience was accepted")
	}
}

func TestCheckHeaderRefusesNamesTheConnectionOwns(t *testing.T) {
	for _, h := range []string{"Authorization", "X-Api-Key", "x-token"} {
		if err := checkHeader(h); err != nil {
			t.Errorf("%q: %v", h, err)
		}
	}
	for _, h := range []string{"", "Host", "content-type", "Cookie", "Mcp-Session-Id", "bad header", "bad:header", "a\r\nb", strings.Repeat("a", 101)} {
		if checkHeader(h) == nil {
			t.Errorf("%q should be refused", h)
		}
	}
}

func TestCheckSecretBoundsAndControlCharacters(t *testing.T) {
	if checkSecret("Bearer abc") != nil || checkSecret(strings.Repeat("a", maxSecretBytes)) != nil {
		t.Error("a valid secret was refused")
	}
	for _, s := range []string{"", strings.Repeat("a", maxSecretBytes+1), "a\nb", "a\x00b"} {
		if checkSecret(s) == nil {
			t.Errorf("%q should be refused", s)
		}
	}
}

func TestInvalidErrorNamesTheField(t *testing.T) {
	err := invalid("url", "must start with https://")
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "https://") {
		t.Errorf("error = %q", err)
	}
}
