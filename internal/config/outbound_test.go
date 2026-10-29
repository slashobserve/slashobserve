package config

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func patternStrings(cfg Config) []string {
	out := []string{}
	for _, p := range cfg.OutboundDomains {
		out = append(out, p.String())
	}
	return out
}

func TestOutboundDomainsSetting(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantLocked bool
		want       []string
	}{
		{"unset", "", false, []string{}},
		{"blank", "   ", false, []string{}},
		{"none", "none", true, []string{}},
		{"none in capitals", "NONE", true, []string{}},
		{"a list with a wildcard", "example.com, *.example.org", true, []string{"example.com", "*.example.org"}},
		{"duplicates fold", "Example.com,example.com, *.a.io,*.A.io", true, []string{"example.com", "*.a.io"}},
		{"empty items are skipped", ",example.com,, ", true, []string{"example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			if tt.value != "" {
				env[envOutboundDomains] = tt.value
			}
			cfg, err := Load(envOf(llmEnv(env)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.OutboundDomainsLocked != tt.wantLocked {
				t.Fatalf("OutboundDomainsLocked = %v, want %v", cfg.OutboundDomainsLocked, tt.wantLocked)
			}
			if got := patternStrings(cfg); !slices.Equal(got, tt.want) {
				t.Fatalf("OutboundDomains = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutboundDomainsRefuseBadEntriesByName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"a url", "http://x.com", `"http://x.com"`},
		{"a single label host", "localhost", `"localhost"`},
		{"an address", "10.0.0.1", `"10.0.0.1"`},
		{"one bad entry among good ones", "example.com,bad host", `"bad host"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{envOutboundDomains: tt.value})))
			if err == nil || !strings.Contains(err.Error(), envOutboundDomains+": "+tt.want) {
				t.Fatalf("Load err = %v, want one naming %s and %s", err, envOutboundDomains, tt.want)
			}
		})
	}
}

func TestOutboundDomainsAreCappedAtOneHundred(t *testing.T) {
	list := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf("host%d.example.com", i)
		}
		return strings.Join(items, ",")
	}
	if _, err := Load(envOf(llmEnv(map[string]string{envOutboundDomains: list(100)}))); err != nil {
		t.Fatalf("Load with 100 entries: %v", err)
	}
	_, err := Load(envOf(llmEnv(map[string]string{envOutboundDomains: list(101)})))
	if err == nil || !strings.Contains(err.Error(), envOutboundDomains+": at most 100 entries") {
		t.Fatalf("Load with 101 entries: err = %v, want the cap error", err)
	}
}
