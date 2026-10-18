package logtest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type Capture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func New() (*slog.Logger, *Capture) {
	c := &Capture{}
	return slog.New(slog.NewJSONHandler(c, &slog.HandlerOptions{Level: slog.LevelDebug})), c
}

func (c *Capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *Capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *Capture) Records(t testing.TB) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(c.String()), "\n") {
		if line == "" {
			continue
		}
		record := map[string]any{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, record)
	}
	return out
}

// Find returns the one record logged with msg and fails the test when there is none or several.
func (c *Capture) Find(t testing.TB, msg string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, record := range c.Records(t) {
		if record["msg"] == msg {
			found = append(found, record)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one %q record, got %d in: %s", msg, len(found), c.String())
	}
	return found[0]
}

// AssertSafeFailure checks that record carries an error_code and the error_type from logattrs.Safe, has no
// "error" or "err" attribute at any group depth, and that none of the forbidden texts appears anywhere in the captured output.
func (c *Capture) AssertSafeFailure(t testing.TB, record map[string]any, code string, forbidden ...string) {
	t.Helper()
	if record["error_code"] != code {
		t.Errorf("error_code = %v, want %q in: %v", record["error_code"], code, record)
	}
	if kind, _ := record["error_type"].(string); kind == "" {
		t.Errorf("record has no error_type: %v", record)
	}
	for _, key := range []string{"error", "err"} {
		if hasKey(record, key) {
			t.Errorf("record has a %q attribute: %v", key, record)
		}
	}
	c.AssertAbsent(t, forbidden...)
}

func hasKey(record map[string]any, key string) bool {
	for name, value := range record {
		if name == key {
			return true
		}
		if group, ok := value.(map[string]any); ok && hasKey(group, key) {
			return true
		}
	}
	return false
}

func (c *Capture) AssertAbsent(t testing.TB, forbidden ...string) {
	t.Helper()
	out := c.String()
	for _, text := range forbidden {
		if strings.Contains(out, text) {
			t.Errorf("log contains %q: %s", text, out)
		}
	}
}
