package cli

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func TestAPIErrorText(t *testing.T) {
	tests := []struct {
		name string
		err  apiError
		want string
	}{
		{"no message", apiError{status: 502}, "the server answered HTTP 502"},
		{"message without a code", apiError{status: 400, message: "bad input"}, "bad input"},
		{"message with a code", apiError{status: 404, message: "not found", code: "not_found"}, "not found (not_found)"},
		{
			"role message explains the key's role",
			apiError{status: 403, message: forbidden.RoleMessage, code: forbidden.Code},
			"your role does not allow this (forbidden_role)\nThis key's service account role does not allow it; an admin can change the role on the API keys page.",
		},
		{
			"access message explains who can see the folder",
			apiError{status: 403, message: forbidden.AccessMessage, code: forbidden.Code},
			"you do not have access to this dashboard (forbidden_role)\nThis key cannot see that dashboard or its folder. A folder Admin or an instance Admin decides who sees a folder, but access lists name people and groups, which a key never matches, so use a key of an Admin service account or of the service account that created the folder.",
		},
		{
			"another forbidden_role message gets no hint",
			apiError{status: 403, message: "something else", code: forbidden.Code},
			"something else (forbidden_role)",
		},
		{
			"the role message under another code gets no hint",
			apiError{status: 403, message: forbidden.RoleMessage, code: "other"},
			forbidden.RoleMessage + " (other)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
