package clickhouse

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	nearRunes     = 30
	statementLead = 64
)

var (
	failedAtPosition = regexp.MustCompile(`failed at position (\d+)`)
	echoedStatement  = regexp.MustCompile(`(?s)failed at position \d+ \(.*?\) \(line \d+, col \d+\): .*?(\. Expected|$)`)
	echoReplacement  = "failed to parse the query$1"
)

func withoutEchoedStatement(message string) string {
	return echoedStatement.ReplaceAllString(message, echoReplacement)
}

type sentError struct {
	err       error
	statement string
}

func (e *sentError) Error() string { return e.err.Error() }

func (e *sentError) Unwrap() error { return e.err }

func syntaxFailure(err error) (*sentError, *ch.Exception, bool) {
	var sent *sentError
	var ex *ch.Exception
	ok := errors.As(err, &sent) && errors.As(err, &ex) && ex.Code == syntaxErrorCode
	return sent, ex, ok
}

func inPersonsText(err error, original string, exp macros.Expansion) error {
	sent, ex, ok := syntaxFailure(err)
	if !ok {
		return err
	}
	located := *ex
	located.Message = locateSyntax(ex.Message, sent.statement, original, exp)
	return &located
}

func locateSyntax(message, statement, original string, exp macros.Expansion) string {
	m := failedAtPosition.FindStringSubmatchIndex(message)
	if m == nil {
		return message
	}
	position, err := strconv.Atoi(message[m[2]:m[3]])
	if err != nil {
		return withoutEchoedStatement(message)
	}
	expanded := strings.TrimRightFunc(exp.Text, unicode.IsSpace)
	body := strings.TrimLeftFunc(expanded, unicode.IsSpace)
	start := strings.Index(statement, body[:min(len(body), statementLead)])
	if start < 0 || body == "" {
		return withoutEchoedStatement(message)
	}
	offset := position - 1 - start + len(expanded) - len(body)
	head, tail := message[:m[0]], ""
	if i := strings.Index(message[m[1]:], ". Expected"); i >= 0 {
		tail = message[m[1]+i:]
	}
	if offset < 0 || offset >= len(expanded) {
		end := len(strings.TrimRightFunc(original, unicode.IsSpace))
		line, column := lineAndColumn(original, end)
		return fmt.Sprintf("%sfailed at the end of the query (line %d, column %d)%s", head, line, column, tail)
	}
	at, _, _ := exp.Locate(offset, offset+1)
	line, column := lineAndColumn(original, at)
	return fmt.Sprintf("%sfailed at line %d, column %d of the query, near %q%s", head, line, column, near(original[at:]), tail)
}

func lineAndColumn(text string, offset int) (line, column int) {
	before := text[:offset]
	return strings.Count(before, "\n") + 1, utf8.RuneCountInString(before[strings.LastIndex(before, "\n")+1:]) + 1
}

func near(rest string) string {
	word, _, _ := strings.Cut(strings.TrimLeftFunc(rest, unicode.IsSpace), " ")
	word, _, _ = strings.Cut(word, "\n")
	if utf8.RuneCountInString(word) <= nearRunes {
		return word
	}
	return string([]rune(word)[:nearRunes])
}
