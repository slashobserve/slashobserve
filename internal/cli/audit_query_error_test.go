package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const unknownFieldReply = `{"error":"unknown field \"nofield\"","code":"query_invalid","start":6,"end":17}`

func TestAuditCommandsUnderlineWhereTheQueryIsWrong(t *testing.T) {
	for _, args := range [][]string{
		{"audit", "search", "--query", "x and nofield:foo"},
		{"audit", "search", "--follow", "--query", "x and nofield:foo"},
		{"audit", "facets", "origin", "--query", "x and nofield:foo"},
		{"audit", "histogram", "--query", "x and nofield:foo"},
		{"audit", "export", "--query", "x and nofield:foo"},
	} {
		_, stdout, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, unknownFieldReply, args...)
		want := "unknown field \"nofield\" (query_invalid)\nx and nofield:foo\n      ^^^^^^^^^^^"
		if code != 1 || stdout != "" || strings.TrimSpace(stderr) != want {
			t.Errorf("%v: exit %d, stdout %q, stderr %q, want %q", args, code, stdout, stderr, want)
		}
	}
}

func TestAuditAggregateUnderlinesTheLineOfAMultiLineBody(t *testing.T) {
	reply := `{"error":"unknown field \"nofield\"","code":"query_invalid","start":11,"end":18}`
	_, _, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, reply, "audit", "aggregate", "--query", "{\n\"aggs\":{\"nofield\":1}}")
	want := "unknown field \"nofield\" (query_invalid)\n\"aggs\":{\"nofield\":1}}\n         ^^^^^^^"
	if code != 1 || strings.TrimSpace(stderr) != want {
		t.Fatalf("exit %d, stderr %q, want %q", code, stderr, want)
	}
}

func TestTheUnderlineCountsCharactersNotBytes(t *testing.T) {
	reply := `{"error":"unknown field \"nofield\"","code":"query_invalid","start":14,"end":25}`
	_, _, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, reply, "audit", "search", "--query", "é and 日本 nofield:foo")
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if code != 1 || len(lines) != 3 || lines[1] != "é and 日本 nofield:foo" || lines[2] != "           ^^^^^^^^^^^" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAnUnderlineThatDoesNotFitTheQueryIsLeftOut(t *testing.T) {
	reply := `{"error":"bad","code":"query_invalid","start":90,"end":99}`
	_, _, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, reply, "audit", "search", "--query", "short")
	if code != 1 || strings.TrimSpace(stderr) != "bad (query_invalid)" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAuditJSONErrorsKeepTheOffsets(t *testing.T) {
	_, _, stderr, code := runAudit(t, http.StatusUnprocessableEntity, nil, unknownFieldReply, "--json", "audit", "search", "--query", "x and nofield:foo")
	var got struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		Start *int   `json:"start"`
		End   *int   `json:"end"`
	}
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("stderr %q: %v", stderr, err)
	}
	if code != 1 || got.Code != "query_invalid" || got.Start == nil || *got.Start != 6 || got.End == nil || *got.End != 17 || strings.Contains(got.Error, "^") {
		t.Fatalf("exit %d, error %+v", code, got)
	}
}
