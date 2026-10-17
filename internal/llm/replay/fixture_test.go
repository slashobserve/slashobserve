package replay

import (
	"strings"
	"testing"
)

func TestFixtureRejectsTurnsAfterATurnWithoutToolCalls(t *testing.T) {
	_, err := parseFixture("f.json", []byte(`{"prompt":"p","turns":[{"text":"done"},{"text":"never reached"}]}`))
	if err == nil || !strings.Contains(err.Error(), "turn 1: unreachable") {
		t.Fatalf("expected an unreachable turn error, got %v", err)
	}
}

func TestFixtureAllowsToolCallsOnTheLastTurn(t *testing.T) {
	content := `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]},{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]},` +
		`{"tool_calls":[{"name":"t"}],"expect":[{"is_error":false}]}]}`
	if _, err := parseFixture("f.json", []byte(content)); err != nil {
		t.Fatalf("parseFixture: %v", err)
	}
}
