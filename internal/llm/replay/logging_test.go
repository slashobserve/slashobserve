package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

const loggingFixture = `{
  "prompt": "show checkout health",
  "turns": [
    {"tool_calls": [{"name": "run_query", "input": {"q": "up"}}]},
    {"text": "done", "expect": [{"is_error": false, "contains": "ok"}]}
  ]
}`

func newLoggedProvider(t *testing.T) (*Provider, *logtest.Capture) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "checkout.json"), []byte(loggingFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	logger, logs := logtest.New()
	p, err := New(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	return p, logs
}

func TestFixtureMissLogsLengthAndHashNotThePrompt(t *testing.T) {
	p, logs := newLoggedProvider(t)
	const prompt = "secret prompt about db.internal"
	if _, err := p.Start(t.Context(), llm.Request{Prompt: prompt}); err == nil {
		t.Fatal("Start found a fixture for an unknown prompt")
	}
	record := logs.Find(t, "replay found no fixture for the prompt")
	if record["prompt_length"] != float64(len(prompt)) {
		t.Errorf("prompt_length = %v, want %d", record["prompt_length"], len(prompt))
	}
	if hash, _ := record["prompt_key_sha256"].(string); len(hash) != promptHashLength {
		t.Errorf("prompt_key_sha256 = %v", record["prompt_key_sha256"])
	}
	logs.AssertAbsent(t, "secret", "db.internal")
}

func TestFailedExpectationLogsShapeNotTheResult(t *testing.T) {
	p, logs := newLoggedProvider(t)
	session, err := p.Start(t.Context(), llm.Request{Prompt: "Show checkout health"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := session.Next(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := llm.ToolResult{CallID: turn.ToolCalls[0].ID, IsError: true, Content: `{"code":"query_invalid","message":"leaked-detail up{pod=\"x\"}"}`}
	if _, err := session.Next(t.Context(), []llm.ToolResult{result}); err == nil {
		t.Fatal("Next accepted a result that breaks the expectation")
	}
	record := logs.Find(t, "replay expectation failed")
	want := map[string]any{
		"fixture": "checkout.json", "turn": float64(1), "call": float64(0), "tool": "run_query",
		"is_error": true, "expected_is_error": false, "result_length": float64(len(result.Content)), "tool_error_code": "query_invalid",
	}
	for key, value := range want {
		if record[key] != value {
			t.Errorf("%s = %v, want %v", key, record[key], value)
		}
	}
	logs.AssertAbsent(t, "leaked-detail", "pod=")
}

func TestLoadedFixturesLogTheHashThatAMissIsMatchedAgainst(t *testing.T) {
	_, logs := newLoggedProvider(t)
	record := logs.Find(t, "replay fixture loaded")
	if record["fixture"] != "checkout.json" || record["prompt_key_sha256"] != promptKeyHash("Show   checkout HEALTH") {
		t.Errorf("record = %v, want the hash of the normalized prompt key", record)
	}
}
