package mcp

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const (
	nonce          = "00112233445566778899aabbccddeeff"
	zeroWidthSpace = "\xe2\x80\x8b"
	zeroWidthJoin  = "\xe2\x80\x8d"
	bidiOverride   = "\xe2\x80\xae"
	noBreakSpace   = "\xc2\xa0"
	eAcute         = "\xc3\xa9"
)

func labelFor(raw string) string {
	return labelPrefix + "." + nonce + "." + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func TestLabelClient(t *testing.T) {
	long := labelFor(strings.Repeat("a", maxLabelBytes))
	exact := labelFor(strings.Repeat("a", 356))
	if len(exact) != maxLabelBytes {
		t.Fatalf("exact label is %d bytes, want %d", len(exact), maxLabelBytes)
	}
	if len(long) <= maxLabelBytes {
		t.Fatalf("long label is %d bytes, not over the limit", len(long))
	}
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"plain name", labelFor("claude-code"), "claude-code"},
		{"round trip of clientLabel", clientLabel("cursor"), "cursor"},
		{"empty header", "", ""},
		{"name over 64 runes is cut", labelFor(strings.Repeat(eAcute, 80)), strings.Repeat(eAcute, 64)},
		{"zero-width characters are removed", labelFor("cl" + zeroWidthSpace + "aude" + zeroWidthJoin + "-code"), "claude-code"},
		{"bidi override is removed", labelFor(bidiOverride + "gnp.exe"), "gnp.exe"},
		{"NUL and control characters are removed", labelFor("a\x00b\x1b[31mc\n"), "ab[31mc"},
		{"invalid UTF-8 is dropped", labelFor("a\xff\xfeb"), "ab"},
		{"empty name", labelFor(""), ""},
		{"whitespace only name", labelFor(" \t" + noBreakSpace + " "), ""},
		{"uppercase nonce hex", labelPrefix + ".00112233445566778899AABBCCDDEEFF." + base64.RawURLEncoding.EncodeToString([]byte("x")), ""},
		{"short nonce", labelPrefix + ".0011." + base64.RawURLEncoding.EncodeToString([]byte("x")), ""},
		{"non-hex nonce", labelPrefix + ".zz112233445566778899aabbccddeeff." + base64.RawURLEncoding.EncodeToString([]byte("x")), ""},
		{"wrong prefix", "so2." + nonce + ".eA", ""},
		{"missing name part", labelPrefix + "." + nonce, ""},
		{"bad base64 characters", labelPrefix + "." + nonce + ".!!!!", ""},
		{"padded base64", labelPrefix + "." + nonce + ".eA==", ""},
		{"standard alphabet base64", labelPrefix + "." + nonce + ".+/+/", ""},
		{"label over 512 bytes", long + "A", ""},
		{"label at 512 bytes", exact, strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelClient(tt.header); got != tt.want {
				t.Errorf("labelClient(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestInitializeClient(t *testing.T) {
	initialize := func(name any) string {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{"clientInfo": map[string]any{"name": name}},
		})
		return string(body)
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"named client", initialize("claude-code"), "claude-code"},
		{"name is cleaned", initialize(bidiOverride + "cursor" + zeroWidthSpace + "\x00"), "cursor"},
		{"name is cut to 64 runes", initialize(strings.Repeat(eAcute, 80)), strings.Repeat(eAcute, 64)},
		{"empty name", initialize(""), ""},
		{"whitespace name", initialize("  \t "), ""},
		{"zero-width only name", initialize(zeroWidthSpace + zeroWidthJoin), ""},
		{"non-string name", initialize(42), ""},
		{"no client info", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, ""},
		{"another method", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"initialize","clientInfo":{"name":"x"}}}`, ""},
		{"initialize only in arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{"q":"initialize"}}}`, ""},
		{"batch body", "[" + initialize("claude-code") + "]", ""},
		{"not JSON", "initialize", ""},
		{"empty body", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initializeClient([]byte(tt.body)); got != tt.want {
				t.Errorf("initializeClient = %q, want %q", got, tt.want)
			}
		})
	}
}
