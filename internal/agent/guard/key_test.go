package guard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKeyIgnoresWhitespaceAndKeepsTheToolName(t *testing.T) {
	compact := Key("update_panels", json.RawMessage(`{"a":1,"b":[2,3]}`))
	if got := Key("update_panels", json.RawMessage("{ \"a\": 1,\n \"b\": [2, 3] }")); got != compact {
		t.Fatalf("whitespace changed the key: %q vs %q", got, compact)
	}
	if got := Key("create_dashboard", json.RawMessage(`{"a":1,"b":[2,3]}`)); got == compact {
		t.Fatal("two tools share a key")
	}
	if got := Key("update_panels", json.RawMessage(`{"a":2,"b":[2,3]}`)); got == compact {
		t.Fatal("different input shares a key")
	}
	if !strings.HasPrefix(compact, "update_panels:") {
		t.Fatalf("key = %q, want the tool name and a colon first", compact)
	}
}

func TestKeyHashesInputThatIsNotJSONAsIs(t *testing.T) {
	broken := Key("lint_query", json.RawMessage(`{"half`))
	if broken == Key("lint_query", json.RawMessage(`{"other`)) {
		t.Fatal("different broken inputs share a key")
	}
	if broken != Key("lint_query", json.RawMessage(`{"half`)) {
		t.Fatal("the same broken input changed key")
	}
}
