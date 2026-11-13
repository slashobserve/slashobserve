package cli

import (
	"net/http"
	"strings"
	"testing"
)

const (
	traceA = "4bf92f3577b34da6a3ce929d0e0e4736"
	traceB = "5c1d7a2e9f8b4a3d"
	traceC = "6D2E8B3FA09C4B4E9F7A1C2D3E4F5061"

	traceComparePath = "/api/v1/datasources/tempo/trace-compare"

	traceCompareAnswer = `{"selected":{"requested":2,"read":1},"baseline":{"requested":1,"read":1},"attributes":9,"partial":true,"truncated":true,"differences":[
		{"attribute":"resource.k8s.pod","value":"checkout-1","selected_share":1,"baseline_share":0.125,"selected_count":2,"baseline_count":1},
		{"attribute":"status","value":"error","selected_share":0.6667,"baseline_share":0,"selected_count":2,"baseline_count":0}]}`
)

func TestTracesCompare(t *testing.T) {
	t.Run("sends both id sets with the range and prints the shares as percentages", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, traceComparePath, 200, traceCompareAnswer})
		stdout, stderr, code := f.run("traces", "--datasource", "tempo", "--from", "now-6h", "--compare", traceA+","+traceB, "--baseline", traceC)
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, traceComparePath, "", `{"selected":["` + traceA + `","` + traceB + `"],"baseline":["` + strings.ToLower(traceC) + `"],"from":"now-6h","to":"now","limit":20}`})
		for _, want := range []string{
			"ATTRIBUTE VALUE SELECTED BASELINE",
			"resource.k8s.pod checkout-1 100.0% 12.5%",
			"status error 66.7% 0.0%",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout lacks %q:\n%s", want, stdout)
			}
		}
		for _, notice := range []string{"Read 1 of 2 selected traces and 1 of 1 baseline traces", "More differences exist"} {
			if !strings.Contains(stderr, notice) {
				t.Fatalf("stderr %q lacks %q", stderr, notice)
			}
		}
	})
	t.Run("accepts repeated flags and a limit", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, traceComparePath, 200, traceCompareAnswer})
		if _, stderr, code := f.run("traces", "--datasource", "tempo", "--compare", traceA, "--compare", traceB, "--baseline", traceC, "--limit", "5"); code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		f.wantRequests(t, recordedRequest{http.MethodPost, traceComparePath, "", `{"selected":["` + traceA + `","` + traceB + `"],"baseline":["` + strings.ToLower(traceC) + `"],"from":"now-24h","to":"now","limit":5}`})
	})
	t.Run("prints the response body with --json", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, traceComparePath, 200, traceCompareAnswer})
		stdout, _, code := f.run("--json", "traces", "--datasource", "tempo", "--compare", traceA, "--baseline", traceB)
		if code != 0 || !sameJSON(stdout, traceCompareAnswer) {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
	t.Run("says when no value differs", func(t *testing.T) {
		f := newManageFake(t, manageReply{http.MethodPost, traceComparePath, 200, `{"selected":{"requested":1,"read":1},"baseline":{"requested":1,"read":1},"attributes":3,"partial":false,"truncated":false,"differences":[]}`})
		stdout, stderr, code := f.run("traces", "--datasource", "tempo", "--compare", traceA, "--baseline", traceB)
		if code != 0 || stdout != "" || !strings.Contains(stderr, "No attribute value differs") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}

func TestTracesCompareUsageErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no baseline", []string{"--compare", traceA}, "--compare needs --baseline"},
		{"a positional id", []string{"--compare", traceA, "--baseline", traceB, traceC}, "not as arguments"},
		{"ask", []string{"--compare", traceA, "--baseline", traceB, "--ask", "slow checkouts"}, "--compare cannot be used with --ask"},
		{"a compare id that is not a trace id", []string{"--compare", "checkout", "--baseline", traceB}, `"checkout" in --compare is not a trace id`},
		{"a baseline id that is not a trace id", []string{"--compare", traceA, "--baseline", traceB + ",12345"}, `"12345" in --baseline is not a trace id`},
		{"an empty compare list", []string{"--compare", "", "--baseline", traceB}, "--compare needs at least one trace id"},
		{"a limit above 100", []string{"--compare", traceA, "--baseline", traceB, "--limit", "101"}, "--limit must be 1 to 100"},
		{"a limit of zero", []string{"--compare", traceA, "--baseline", traceB, "--limit", "0"}, "--limit must be 1 to 100"},
		{"baseline without compare", []string{traceA, "--baseline", traceB}, "--baseline needs --compare"},
		{"limit without compare", []string{traceA, "--limit", "5"}, "--limit needs --compare"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newManageFake(t)
			_, stderr, code := f.run(append([]string{"traces", "--datasource", "tempo"}, tc.args...)...)
			if code != 2 || !strings.Contains(stderr, tc.wantErr) || len(f.got) != 0 {
				t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
			}
		})
	}
	t.Run("a missing datasource", func(t *testing.T) {
		f := newManageFake(t)
		_, stderr, code := f.run("traces", "--compare", traceA, "--baseline", traceB)
		if code != 2 || !strings.Contains(stderr, "needs --datasource") || len(f.got) != 0 {
			t.Fatalf("exit %d, stderr %q, requests %v", code, stderr, f.got)
		}
	})
}
