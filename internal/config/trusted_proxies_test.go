package config

import (
	"slices"
	"strings"
	"testing"
)

func proxyStrings(cfg Config) []string {
	out := []string{}
	for _, p := range cfg.TrustedProxies {
		out = append(out, p.String())
	}
	return out
}

func TestTrustedProxiesSetting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"unset", "", []string{}},
		{"blank", "   ", []string{}},
		{"an address", "10.0.0.5", []string{"10.0.0.5/32"}},
		{"a range", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"a range keeps only its network bits", "10.1.2.3/16", []string{"10.1.0.0/16"}},
		{"an IPv6 address", "2001:db8::1", []string{"2001:db8::1/128"}},
		{"an IPv6 range", "2001:db8::/32", []string{"2001:db8::/32"}},
		{"a mapped range becomes IPv4", "::ffff:10.0.0.0/104", []string{"10.0.0.0/8"}},
		{"a mapped address becomes IPv4", "::ffff:10.0.0.5", []string{"10.0.0.5/32"}},
		{"a list with spaces and empty items", " 10.0.0.0/8, ,192.0.2.1,, fd00::/8 ", []string{"10.0.0.0/8", "192.0.2.1/32", "fd00::/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{}
			if tt.value != "" {
				env[envTrustedProxies] = tt.value
			}
			cfg, err := Load(envOf(llmEnv(env)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := proxyStrings(cfg); !slices.Equal(got, tt.want) {
				t.Fatalf("TrustedProxies = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrustedProxiesRefuseBadEntriesByName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"a host name", "proxy.example.com", `"proxy.example.com"`},
		{"a bad address among good ones", "10.0.0.1,10.0.0.300", `"10.0.0.300"`},
		{"a range with a bad length", "10.0.0.0/33", `"10.0.0.0/33"`},
		{"a range with no length", "10.0.0.0/", `"10.0.0.0/"`},
		{"every IPv4 address", "0.0.0.0/0", `"0.0.0.0/0"`},
		{"every IPv6 address", "::/0", `"::/0"`},
		{"every IPv4 address as a mapped range", "::ffff:0:0/96", `"::ffff:0:0/96"`},
		{"a mapped range wider than IPv4", "::ffff:0:0/80", `"::ffff:0:0/80"`},
		{"a zone", "fe80::1%eth0", `"fe80::1%eth0"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{envTrustedProxies: tt.value})))
			if err == nil {
				t.Fatalf("Load accepted %q", tt.value)
			}
			if !strings.Contains(err.Error(), envTrustedProxies) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to name %s and %s", err, envTrustedProxies, tt.want)
			}
		})
	}
}
