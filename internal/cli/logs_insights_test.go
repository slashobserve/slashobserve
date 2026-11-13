package cli

import (
	"net/http"
	"strings"
	"testing"
)

const (
	lokiPath = "/api/v1/datasources/loki"
	anchorNs = "1793527200123456789"

	facetsAnswer = `{"sampled":true,"lines_sampled":1000,"truncated":true,"fields":[
		{"name":"app","kind":"label","exact":true,"unit":"bytes","values":[{"value":"shop","count":4096},{"value":"cart","count":512}],"truncated":false},
		{"name":"status","kind":"field","exact":false,"cardinality":2,"values":[{"value":"200","count":900},{"value":"500","count":100}],"truncated":false}]}`

	patternsAnswer = `{"source":"server","lines_sampled":2000,"lines_matched":1500,"truncated":true,"patterns":[
		{"pattern":"payment <*> declined","count":1200,"sample":"payment 42 declined","first":"","last":"","trend":[],"level":"error"},
		{"pattern":"cart <*> loaded","count":300,"sample":"cart 7 loaded","first":"","last":"","trend":[],"level":""}]}`

	contextAnswer = `{"anchor":"` + anchorNs + `","before":{"truncated":true,"frames":[{"ref":"A","labels":{"app":"shop"},"fields":[
			{"name":"Time","type":"time","values":[1793527199500,1793527199500,1793527198500]},
			{"name":"Body","type":"string","values":["newer tie","older tie","first before"]}]}]},
		"after":{"truncated":false,"frames":[{"ref":"A","labels":{"app":"shop"},"fields":[
			{"name":"Time","type":"time","values":[1793527201000]},
			{"name":"Body","type":"string","values":["after line"]}]}]}}`
)

func TestLogsFacets(t *testing.T) {
	path := lokiPath + "/log-facets"
	t.Run("sends the query, range and fields and prints each field's values", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, facetsAnswer})
		stdout, stderr, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--from", "now-15m", "--facets", "--field", "app", "--field", "status")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","from":"now-15m","to":"now","fields":["app","status"]}`})
		want := "app (label, exact, bytes)\n 4096 shop\n 512 cart\n\nstatus (field, sampled)\n 900 200\n 100 500\n"
		if stdout != want {
			t.Fatalf("stdout %q, want %q", stdout, want)
		}
		for _, notice := range []string{"newest 1000 lines", "name some with --field"} {
			if !strings.Contains(stderr, notice) {
				t.Fatalf("stderr %q lacks %q", stderr, notice)
			}
		}
	})
	t.Run("omits fields when none are named", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, facetsAnswer})
		if _, _, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--facets"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","from":"now-1h","to":"now"}`})
	})
	t.Run("prints the response body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, facetsAnswer})
		stdout, _, code := f.run("--json", "logs", `{app="shop"}`, "--datasource", "loki", "--facets")
		if code != 0 || !sameJSON(stdout, facetsAnswer) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
}

func TestLogsPatterns(t *testing.T) {
	path := lokiPath + "/log-patterns"
	t.Run("sends the query and range and prints counts and patterns", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, patternsAnswer})
		stdout, stderr, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--patterns", "--limit", "20")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","from":"now-1h","to":"now","limit":20}`})
		for _, want := range []string{"COUNT PATTERN", "1200 payment <*> declined", "300 cart <*> loaded"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
		for _, notice := range []string{"1500 lines matched, 2000 lines sampled", "More patterns exist"} {
			if !strings.Contains(stderr, notice) {
				t.Fatalf("stderr %q lacks %q", stderr, notice)
			}
		}
	})
	t.Run("caps the limit at the most patterns the server returns", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, patternsAnswer})
		if _, _, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--patterns", "--limit", "1000"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","from":"now-1h","to":"now","limit":100}`})
	})
}

func TestLogsContext(t *testing.T) {
	path := lokiPath + "/log-context"
	cases := []struct {
		name   string
		anchor string
	}{
		{"an RFC 3339 time becomes its nanosecond string", "2026-11-01T10:00:00.123456789Z"},
		{"a nanosecond time is sent as given", anchorNs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newManageFake(t, manageReply{http.MethodPost, path, 200, contextAnswer})
			stdout, stderr, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--context", tc.anchor, "--before", "2", "--after", "0")
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","anchor":"` + anchorNs + `","before":2,"after":0}`})
			want := "2026-11-01T09:59:58.500Z first before\n" +
				"2026-11-01T09:59:59.500Z older tie\n" +
				"2026-11-01T09:59:59.500Z newer tie\n" +
				"-- 2026-11-01T10:00:00.123456789Z --\n" +
				"2026-11-01T10:00:01.000Z after line\n"
			if stdout != want {
				t.Fatalf("stdout %q, want %q", stdout, want)
			}
			if !strings.Contains(stderr, "More lines exist before it") || strings.Contains(stderr, "after it") {
				t.Fatalf("stderr %q", stderr)
			}
		})
	}
	t.Run("sends 50 lines on each side by default", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, contextAnswer})
		if _, _, code := f.run("logs", `{app="shop"}`, "--datasource", "loki", "--context", anchorNs); code != 0 {
			t.Fatalf("exit %d", code)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, path, "", `{"query":"{app=\"shop\"}","anchor":"` + anchorNs + `","before":50,"after":50}`})
	})
	t.Run("prints the response body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, path, 200, contextAnswer})
		stdout, _, code := f.run("--json", "logs", `{app="shop"}`, "--datasource", "loki", "--context", anchorNs)
		if code != 0 || !sameJSON(stdout, contextAnswer) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
}

func TestLogsInsightUsageErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"facets with tail", []string{"--facets", "--tail"}, "--facets and --tail cannot be used together"},
		{"facets with patterns", []string{"--facets", "--patterns"}, "--facets and --patterns cannot be used together"},
		{"patterns with context", []string{"--patterns", "--context", anchorNs}, "--patterns and --context cannot be used together"},
		{"context with tail", []string{"--context", anchorNs, "--tail"}, "--context and --tail cannot be used together"},
		{"field without facets", []string{"--field", "app"}, "--field needs --facets"},
		{"before without context", []string{"--before", "5"}, "--before needs --context"},
		{"after without context", []string{"--after", "5"}, "--after needs --context"},
		{"too many lines before", []string{"--context", anchorNs, "--before", "501"}, "--before and --after must be 0 to 500"},
		{"negative lines after", []string{"--context", anchorNs, "--after", "-1"}, "--before and --after must be 0 to 500"},
		{"context that is not a time", []string{"--context", "yesterday"}, `--context "yesterday" is not a time`},
		{"context with a time that has no zone", []string{"--context", "2026-11-01T10:00:00"}, "is not a time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newManageFake(t)
			_, stderr, code := f.run(append([]string{"logs", `{app="shop"}`, "--datasource", "loki"}, tc.args...)...)
			if code != 2 || !strings.Contains(stderr, tc.wantErr) || len(f.got) != 0 {
				t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
			}
		})
	}
}
