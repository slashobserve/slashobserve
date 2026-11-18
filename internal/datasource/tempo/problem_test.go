package tempo

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTraceQLProblemSaysWhereAndWhat(t *testing.T) {
	cases := []struct {
		name, message, query, sent, want string
	}{
		{
			"the added hint is not blamed", "parse error at line 1, col 10: syntax error: unexpected with", "{ name = ", "{ name = with (most_recent=true)",
			`Invalid TraceQL at line 1, column 9 after "{ name =": unexpected end of the query.`,
		},
		{
			"an unclosed second line", "parse error at line 2, col 12: syntax error: unexpected with", "{ a = 1 &&\n  b = \"x\"", "{ a = 1 &&\n  b = \"x\" with (most_recent=true)",
			"Invalid TraceQL at line 2, column 10 after \"{ a = 1 &&\n  b = \"x\"\": unexpected end of the query.",
		},
		{
			"end of query", "invalid TraceQL query: parse error at line 1, col 9: syntax error: unexpected $end", "{ name =", "",
			`Invalid TraceQL at line 1, column 9 after "{ name =": unexpected end of the query.`,
		},
		{
			"token", "parse error at line 1, col 3: syntax error: unexpected IDENTIFIER", "{ foo }", "",
			`Invalid TraceQL at line 1, column 3 after "{": unexpected IDENTIFIER.`,
		},
		{
			"second line", "parse error at line 2, col 5: syntax error: unexpected }", "{ name = \"a\"\n  && }", "",
			"Invalid TraceQL at line 2, column 5 after \"{ name = \"a\"\n  &&\": unexpected }.",
		},
		{
			"long query keeps the tail", "parse error at line 1, col 65: syntax error: unexpected }", `{ resource.service.name = "checkout" && span.http.status_code = }`, "",
			`Invalid TraceQL at line 1, column 65 after "...ut" && span.http.status_code =": unexpected }.`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := traceQLProblem(datasource.Errorf(datasource.ErrInvalidQuery, "%s", tc.message), tc.query, tc.sent)
			if !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("kind lost: %v", err)
			}
			var ue *datasource.UpstreamError
			if !errors.As(err, &ue) || ue.Message != tc.want {
				t.Fatalf("message = %q, want %q", ue.Message, tc.want)
			}
			if strings.Contains(err.Error(), "$end") || strings.Count(err.Error(), "invalid") > 1 {
				t.Fatalf("raw parser text left in %q", err)
			}
		})
	}
}

func TestTraceQLProblemLeavesOtherErrorsAlone(t *testing.T) {
	other := datasource.Errorf(datasource.ErrTimeout, "query timed out")
	if got := traceQLProblem(other, "{}", "{}"); !errors.Is(got, other) {
		t.Fatalf("got %v", got)
	}
	plain := datasource.Errorf(datasource.ErrInvalidQuery, "query rejected by the datasource")
	if got := traceQLProblem(plain, "{}", "{}"); !errors.Is(got, plain) {
		t.Fatalf("got %v", got)
	}
	if got := traceQLProblem(nil, "{}", "{}"); got != nil {
		t.Fatalf("got %v", got)
	}
}
