package cli

import "testing"

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
			"forbidden role explains the key's role",
			apiError{status: 403, message: "your role cannot do this", code: "forbidden_role"},
			"your role cannot do this (forbidden_role)\nThis key's service account role does not allow it; an admin can change the role on the API keys page.",
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
