package cli

import (
	"net/http"
	"strings"
	"testing"
)

const andWarning = `"andd" is searched as text; did you mean "and"?`

func TestAuditCommandsPrintTheQueryWarningsOnStandardError(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		reply string
	}{
		{[]string{"audit", "search", "--query", "x andd y"}, `{"events":[],"next_cursor":null,"warnings":["` + strings.ReplaceAll(andWarning, `"`, `\"`) + `"]}`},
		{[]string{"audit", "facets", "origin", "--query", "x andd y"}, `{"field":"origin","values":[],"warnings":["` + strings.ReplaceAll(andWarning, `"`, `\"`) + `"]}`},
		{[]string{"audit", "histogram", "--query", "x andd y"}, `{"step_ms":1000,"total":0,"buckets":[],"warnings":["` + strings.ReplaceAll(andWarning, `"`, `\"`) + `"]}`},
		{[]string{"audit", "aggregate", "--query", `{"aggs":{}}`}, `{"levels":[],"metrics":[],"rows":[],"truncated":false,"warnings":["` + strings.ReplaceAll(andWarning, `"`, `\"`) + `"]}`},
	} {
		_, stdout, stderr, code := runAudit(t, http.StatusOK, nil, tc.reply, tc.args...)
		if code != 0 || strings.TrimSpace(strings.SplitN(stderr, "\n", 2)[0]) != "warning: "+andWarning || strings.Contains(stdout, "warning") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", tc.args, code, stdout, stderr)
		}
	}
}

func TestAuditJSONKeepsTheWarningsInTheBody(t *testing.T) {
	reply := `{"events":[],"next_cursor":null,"warnings":["` + strings.ReplaceAll(andWarning, `"`, `\"`) + `"]}`
	call, stdout, _, code := runAudit(t, http.StatusOK, nil, reply, "--json", "audit", "search")
	if code != 0 || strings.TrimSpace(stdout) != reply || call.query.Has("q") || call.query.Has("since") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, call, stdout)
	}
}

func TestReadingEveryPageKeepsTheWarningsOnce(t *testing.T) {
	merged, err := appendPage([]byte(`{"events":[1],"next_cursor":"c","warnings":["w"]}`), []byte(`{"events":[2],"next_cursor":null,"warnings":["w"]}`))
	if err != nil || !strings.Contains(string(merged), `"warnings":["w"]`) || !strings.Contains(string(merged), `"events":[1,2]`) {
		t.Fatalf("%s, %v", merged, err)
	}
}
