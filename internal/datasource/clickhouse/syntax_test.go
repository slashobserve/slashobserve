package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

func syntaxMessage(statement string, at int) string {
	line := strings.Count(statement[:at], "\n") + 1
	col := utf8.RuneCountInString(statement[strings.LastIndex(statement[:at], "\n")+1:at]) + 1
	token, _, _ := strings.Cut(statement[at:], " ")
	return fmt.Sprintf("Syntax error: failed at position %d (%s) (line %d, col %d): %s. Expected one of: BY, token", at+1, token, line, col, statement[at:])
}

type syntaxConn struct {
	*pageConn
	fails func(statement string) (int, bool)
}

func (c *syntaxConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if at, ok := c.fails(query); ok {
		c.mu.Lock()
		c.statements = append(c.statements, query)
		c.mu.Unlock()
		return nil, &ch.Exception{Code: syntaxErrorCode, Message: syntaxMessage(query, at)}
	}
	return c.pageConn.Query(ctx, query, args...)
}

func syntaxDatasource(fails func(string) (int, bool)) *Datasource {
	conn := &syntaxConn{pageConn: &pageConn{fakeConn: &fakeConn{columns: stringColumns("ServiceName")}}, fails: fails}
	d := pageDatasource(conn.pageConn)
	d.conn = conn
	return d
}

const unfinishedGroup = "SELECT ServiceName, count() FROM otel_traces\nWHERE $__timeFilter(Timestamp) GROUP"

func failOnPage(at func(statement string) int) func(string) (int, bool) {
	return func(statement string) (int, bool) {
		if strings.HasSuffix(statement, "LIMIT 0") {
			return 0, false
		}
		return at(statement), true
	}
}

func TestASyntaxErrorCountsItsPositionInThePersonsText(t *testing.T) {
	d := syntaxDatasource(failOnPage(func(s string) int { return strings.LastIndex(s, "GROUP") }))
	_, _, err := d.QueryPage(context.Background(), traceSQLRequest(unfinishedGroup, 50))
	want := `Syntax error: failed at line 2, column 32 of the query, near "GROUP". Expected one of: BY, token`
	if err == nil || datasource.MessageOf(err) != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestASyntaxErrorInTheWrapperPointsAtTheEndOfThePersonsText(t *testing.T) {
	d := syntaxDatasource(failOnPage(func(s string) int { return strings.LastIndex(s, "GROUP") + len("GROUP\n") }))
	_, _, err := d.QueryPage(context.Background(), traceSQLRequest(unfinishedGroup, 50))
	want := "Syntax error: failed at the end of the query (line 2, column 37). Expected one of: BY, token"
	if err == nil || datasource.MessageOf(err) != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

const strayParenthesis = "SELECT TraceId\nFROM otel_traces WHERE ) AND $__timeFilter(Timestamp)"

func failWrappedAtItsEnd(statement string) (int, bool) {
	if strings.HasPrefix(statement, "EXPLAIN AST ") {
		return strings.Index(statement, ") AND"), true
	}
	return strings.LastIndex(statement, "LIMIT 0"), true
}

func TestAStrayParenthesisInAPagedQueryIsLocatedByParsingTheQueryAlone(t *testing.T) {
	d := syntaxDatasource(failWrappedAtItsEnd)
	_, _, err := d.QueryPage(context.Background(), traceSQLRequest(strayParenthesis, 50))
	want := `Syntax error: failed at line 2, column 24 of the query, near ")". Expected one of: BY, token`
	if err == nil || datasource.MessageOf(err) != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestAWrapperOnlySyntaxErrorKeepsItsEndOfQueryPosition(t *testing.T) {
	d := syntaxDatasource(func(statement string) (int, bool) {
		if strings.HasPrefix(statement, "EXPLAIN AST ") {
			return 0, false
		}
		return strings.LastIndex(statement, "LIMIT 0"), true
	})
	_, _, err := d.QueryPage(context.Background(), traceSQLRequest(unfinishedGroup, 50))
	want := "Syntax error: failed at the end of the query (line 2, column 37). Expected one of: BY, token"
	if err == nil || datasource.MessageOf(err) != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestASyntaxErrorOfAnExploreQueryIsCountedBeforeTheMacrosExpand(t *testing.T) {
	d := syntaxDatasource(func(s string) (int, bool) { return strings.LastIndex(s, "GROUP"), true })
	_, err := d.QueryInstant(context.Background(), exploreQuery(unfinishedGroup), datasource.TimeRange{From: linesStart, To: linesEnd})
	want := `Syntax error: failed at line 2, column 32 of the query, near "GROUP". Expected one of: BY, token`
	if err == nil || datasource.MessageOf(err) != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestASyntaxErrorWithoutAPositionIsLeftAsTheDatabaseSaidIt(t *testing.T) {
	message := "Syntax error: unexpected end of query"
	if got := locateSyntax(message, "SELECT", "SELECT", macros.Expansion{Text: "SELECT"}); got != message {
		t.Fatalf("got %q, want the message unchanged", got)
	}
}

func TestLintShowsOnlyThePersonsTextAndPositionOfASyntaxError(t *testing.T) {
	query := "SELECT TraceId\nFROM otel_traces WHERE ) AND $__timeFilter(Timestamp)"
	d := syntaxDatasource(func(s string) (int, bool) {
		return strings.Index(s, ") AND"), strings.HasPrefix(s, explainSyntax)
	})
	diags, err := d.Lint(context.Background(), query)
	if err != nil || len(diags) != 1 {
		t.Fatalf("Lint = %+v, %v", diags, err)
	}
	want := `Syntax error: failed at line 2, column 24 of the query, near ")". Expected one of: BY, token`
	if diags[0].Message != want {
		t.Fatalf("message %q, want %q", diags[0].Message, want)
	}
	if got := query[diags[0].Start:diags[0].End]; !strings.HasPrefix(got, ")") {
		t.Fatalf("range %d-%d covers %q", diags[0].Start, diags[0].End, got)
	}
}

func TestASyntaxErrorThatCannotBeLocatedDoesNotEchoTheExpandedStatement(t *testing.T) {
	message := "Syntax error: failed at position 60 () (line 2, col 22): ) AND (Timestamp >= fromUnixTimestamp64Milli(toInt64(1795163735263), 'UTC')\nLIMIT 1. Expected one of: BY, token"
	got := locateSyntax(message, "unrelated", "SELECT 1", macros.Expansion{Text: "SELECT 1"})
	want := "Syntax error: failed to parse the query. Expected one of: BY, token"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = datasource.MessageOf(mapException(&ch.Exception{Code: syntaxErrorCode, Message: message}))
	if got != want {
		t.Fatalf("mapped %q, want %q", got, want)
	}
}
