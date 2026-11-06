package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
)

func toolInputLines(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(logs, "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "agent tool input" {
			out = append(out, entry)
		}
	}
	return out
}

func shortHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])[:12]
}

func TestToolInputsAreLoggedByShapeNeverByText(t *testing.T) {
	query := `{"datasource":"prom","query":"rate(UNIQUE-QUERY-TEXT[5m])"}`
	rejected := `{"spec":{"title":"UNIQUE-TITLE","panels":[` +
		`{"id":"p1","title":"UNIQUE-TITLE","queries":[{"expr":"UNIQUE-QUERY-TEXT"}]},` +
		`{"id":"r1","type":"row","title":"UNIQUE-TITLE","panels":[{"id":"p2"}]},` +
		`{"id":"has a space UNIQUE-TITLE"}]}}`
	accepted := `{"upsert":[{"id":"p3","title":"UNIQUE-TITLE"}],"delete":["p1"],"summary":"UNIQUE-SUMMARY"}`
	r := newRig(t, session(
		toolTurn(call("c1", "query_range", query)),
		toolTurn(call("c2", "create_dashboard", rejected)),
		toolTurn(call("c3", "update_panels", accepted)),
	))
	r.agent = agent.New(agent.Config{
		Provider:      r.provider,
		Tools:         r.tools,
		Dashboards:    r.dash,
		Builds:        r.builds,
		Datasources:   fakeHealth{},
		Logger:        slog.New(slog.NewJSONHandler(r.logs, nil)),
		LogToolInputs: true,
	})
	r.tools.onSave("create_dashboard", invalidSpecResult("query_failed", "schema"))
	r.tools.onSave("update_panels", savedResult(dashID, 1))
	task := r.task()
	task.Prompt = "UNIQUE-PROMPT checkout health"

	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	lines := toolInputLines(t, r.logs.String())
	if len(lines) != 3 {
		t.Fatalf("tool input lines = %d, want one per call: %s", len(lines), r.logs.String())
	}
	want := []map[string]any{
		{"tool": "query_range", "input_sha256": shortHash(query), "input_bytes": float64(len(query))},
		{"tool": "create_dashboard", "input_sha256": shortHash(rejected), "input_bytes": float64(len(rejected)),
			"panel_ids": "p1,r1,p2", "panel_count": float64(3), "issue_paths": []any{"/panels/0", "/panels/0"}},
		{"tool": "update_panels", "input_sha256": shortHash(accepted), "input_bytes": float64(len(accepted)),
			"panel_ids": "p3,p1", "panel_count": float64(2)},
	}
	for i, line := range lines {
		for _, key := range []string{"time", "level", "msg"} {
			delete(line, key)
		}
		if line["agent_request_id"] != requestID {
			t.Fatalf("line %d agent_request_id = %v", i, line["agent_request_id"])
		}
		delete(line, "agent_request_id")
		if !reflect.DeepEqual(line, want[i]) {
			t.Errorf("line %d = %v, want %v", i, line, want[i])
		}
		for key, value := range line {
			text := fmt.Sprint(value)
			for _, leaked := range []string{"UNIQUE-QUERY-TEXT", "UNIQUE-TITLE", "UNIQUE-PROMPT", "UNIQUE-SUMMARY", "SECRET"} {
				if strings.Contains(text, leaked) {
					t.Errorf("line %d %s carries %q: %v", i, key, leaked, value)
				}
			}
		}
	}
}

func TestToolInputsAreNotLoggedByDefault(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "create_dashboard", `{"spec":{"panels":[{"id":"p1"}]}}`))))
	r.tools.onSave("create_dashboard", savedResult(dashID, 1))

	if _, err := r.agent.Run(context.Background(), r.task()); err != nil {
		t.Fatal(err)
	}
	if lines := toolInputLines(t, r.logs.String()); len(lines) != 0 {
		t.Fatalf("tool input lines = %v, want none", lines)
	}
}
