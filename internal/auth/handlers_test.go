package auth

import (
	"strings"
	"testing"
)

func TestEmailFailure(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		wantCode   string
		wantReason string
	}{
		{"valid", "admin@example.com", "", ""},
		{"missing", "", "email_missing", ""},
		{"too long", strings.Repeat("a", maxEmailBytes) + "@example.com", "email_not_allowed", "email_too_long"},
		{"nul", "a\x00@example.com", "email_not_allowed", "email_invalid"},
		{"newline", "a\n@example.com", "email_not_allowed", "email_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := emailFailure(tt.email)
			if tt.wantCode == "" {
				if got != nil {
					t.Fatalf("emailFailure(%q) = %+v, want nil", tt.email, got)
				}
				return
			}
			if got == nil || got.code != tt.wantCode || got.reason != tt.wantReason {
				t.Fatalf("emailFailure(%q) = %+v, want %s/%s", tt.email, got, tt.wantCode, tt.wantReason)
			}
		})
	}
}
