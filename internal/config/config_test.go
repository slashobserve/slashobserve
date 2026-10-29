package config

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestAdminEmailsSetting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"unset", "", nil},
		{"blank", "   ", nil},
		{"one", "boss@example.com", []string{"boss@example.com"}},
		{"trimmed and lowercased", " Boss@Example.COM , ops@example.com", []string{"boss@example.com", "ops@example.com"}},
		{"duplicates fold", "a@example.com,A@EXAMPLE.com", []string{"a@example.com"}},
		{"empty items are skipped", ",a@example.com,, ", []string{"a@example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{envAdminEmails: tt.value})))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !slices.Equal(cfg.AdminEmails, tt.want) {
				t.Fatalf("AdminEmails = %v, want %v", cfg.AdminEmails, tt.want)
			}
		})
	}
}

func TestAdminEmailsRefuseMalformedEntriesWithoutEchoingThem(t *testing.T) {
	for _, value := range []string{"secret-name", "a@b@c", "@example.com", "secret@"} {
		t.Run(value, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{envAdminEmails: value})))
			if err == nil || !strings.Contains(err.Error(), envAdminEmails+": entry 1 is not an email address") {
				t.Fatalf("Load err = %v, want one naming %s and entry 1", err, envAdminEmails)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "example.com") {
				t.Fatalf("Load err = %v, echoes the value", err)
			}
		})
	}
}

func TestAdminEmailsAreCappedAtOneHundred(t *testing.T) {
	list := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf("user%d@example.com", i)
		}
		return strings.Join(items, ",")
	}
	if _, err := Load(envOf(llmEnv(map[string]string{envAdminEmails: list(100)}))); err != nil {
		t.Fatalf("100 entries: %v", err)
	}
	_, err := Load(envOf(llmEnv(map[string]string{envAdminEmails: list(101)})))
	if err == nil || !strings.Contains(err.Error(), envAdminEmails+": at most 100 entries") {
		t.Fatalf("101 entries: err = %v", err)
	}
}

func TestOIDCGroupRolesSetting(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []GroupRoleSetting
	}{
		{"unset", "", nil},
		{"one", "eng=editor", []GroupRoleSetting{{"eng", "editor"}}},
		{
			"several keep their order",
			"ops=admin, support = Commenter ,guests=viewer",
			[]GroupRoleSetting{{"ops", "admin"}, {"support", "commenter"}, {"guests", "viewer"}},
		},
		{"split on the last equals sign", "team=a=b=viewer", []GroupRoleSetting{{"team=a=b", "viewer"}}},
		{"empty items are skipped", ",eng=editor,,", []GroupRoleSetting{{"eng", "editor"}}},
		{"same group twice keeps both", "eng=viewer,eng=editor", []GroupRoleSetting{{"eng", "viewer"}, {"eng", "editor"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(envOf(llmEnv(map[string]string{envOIDCGroupRoles: tt.value})))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !slices.Equal(cfg.OIDCGroupRoles, tt.want) {
				t.Fatalf("OIDCGroupRoles = %v, want %v", cfg.OIDCGroupRoles, tt.want)
			}
		})
	}
}

func TestOIDCGroupRolesRefuseBadEntriesByName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"no equals sign", "eng", "entry 1 must be group=role"},
		{"empty group", "=editor", "entry 1 needs a group name"},
		{"empty role", "eng=", `entry 1 has role ""`},
		{"unknown role", "eng=owner", `entry 1 has role "owner"`},
		{"control character in group", "a\x01b=editor", "entry 1 needs a group name"},
		{"group of 257 bytes", strings.Repeat("a", 257) + "=editor", "entry 1 needs a group name"},
		{"second entry is bad", "eng=editor,ops=boss", `entry 2 has role "boss"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envOf(llmEnv(map[string]string{envOIDCGroupRoles: tt.value})))
			if err == nil || !strings.Contains(err.Error(), envOIDCGroupRoles+": "+tt.want) {
				t.Fatalf("Load err = %v, want one naming %s and %q", err, envOIDCGroupRoles, tt.want)
			}
		})
	}
}

func TestOIDCGroupRolesAcceptA256ByteGroupName(t *testing.T) {
	group := strings.Repeat("a", 256)
	cfg, err := Load(envOf(llmEnv(map[string]string{envOIDCGroupRoles: group + "=viewer"})))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.OIDCGroupRoles) != 1 || cfg.OIDCGroupRoles[0].Group != group {
		t.Fatalf("OIDCGroupRoles = %v", cfg.OIDCGroupRoles)
	}
}

func TestOIDCGroupRolesAreCappedAtOneHundred(t *testing.T) {
	list := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = fmt.Sprintf("g%d=viewer", i)
		}
		return strings.Join(items, ",")
	}
	if _, err := Load(envOf(llmEnv(map[string]string{envOIDCGroupRoles: list(100)}))); err != nil {
		t.Fatalf("100 entries: %v", err)
	}
	_, err := Load(envOf(llmEnv(map[string]string{envOIDCGroupRoles: list(101)})))
	if err == nil || !strings.Contains(err.Error(), envOIDCGroupRoles+": at most 100 entries") {
		t.Fatalf("101 entries: err = %v", err)
	}
}
