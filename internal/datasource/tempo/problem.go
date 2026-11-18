package tempo

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const excerptLength = 30

var traceQLParseError = regexp.MustCompile(`(?s)parse error at line (\d+), col (\d+): (?:syntax error: )?(.*)$`)

func traceQLProblem(err error, query, sent string) error {
	var ue *datasource.UpstreamError
	if !errors.As(err, &ue) || !errors.Is(ue.Kind, datasource.ErrInvalidQuery) {
		return err
	}
	match := traceQLParseError.FindStringSubmatch(ue.Message)
	if match == nil {
		return err
	}
	line, _ := strconv.Atoi(match[1])
	column, _ := strconv.Atoi(match[2])
	detail := strings.TrimRight(strings.ReplaceAll(match[3], "$end", "end of the query"), ". ")
	if offsetOf(sent, line, column) >= len(query) && len(sent) > len(query) {
		line, column = endOf(query)
		detail = "unexpected end of the query"
	}
	after := ""
	if text := excerptBefore(query, line, column); text != "" {
		after = ` after "` + text + `"`
	}
	return datasource.Errorf(ue.Kind, "Invalid TraceQL at line %d, column %d%s: %s.", line, column, after, detail)
}

func offsetOf(text string, line, column int) int {
	offset := 0
	for i, l := range strings.Split(text, "\n") {
		if i == line-1 {
			return offset + column - 1
		}
		offset += len(l) + 1
	}
	return offset
}

func endOf(text string) (line, column int) {
	lines := strings.Split(strings.TrimRight(text, " \t\r\n"), "\n")
	return len(lines), len(lines[len(lines)-1]) + 1
}

func excerptBefore(query string, line, column int) string {
	lines := strings.Split(query, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	prefix := lines[line-1]
	prefix = prefix[:min(max(column-1, 0), len(prefix))]
	text := strings.TrimSpace(strings.Join(append(lines[:line-1:line-1], prefix), "\n"))
	runes := []rune(text)
	if len(runes) > excerptLength {
		return "..." + string(runes[len(runes)-excerptLength:])
	}
	return text
}
