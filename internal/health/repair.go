package health

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const maxTitleRunes = 80

func repairPrompt(problems []Problem) (string, bool) {
	var lines []string
	seen := map[string]bool{}
	for _, p := range problems {
		line, ok := repairLine(p)
		if !ok || seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", false
	}
	reserve := utf8.RuneCountInString(moreLine(len(lines)))
	var b strings.Builder
	b.WriteString(repairprompt.Prefix)
	size := utf8.RuneCountInString(repairprompt.Prefix)
	for i, line := range lines {
		need := size + 1 + utf8.RuneCountInString(line)
		if i < len(lines)-1 {
			need += reserve
		}
		if need > revisions.MaxPromptRunes {
			b.WriteString(moreLine(len(lines) - i))
			break
		}
		b.WriteString("\n")
		b.WriteString(line)
		size += 1 + utf8.RuneCountInString(line)
	}
	return b.String(), true
}

func moreLine(n int) string {
	return fmt.Sprintf("\n- and %d more", n)
}

func repairLine(p Problem) (string, bool) {
	var what string
	switch p.Kind {
	case KindNoData:
		what = "returns no data"
	case KindQueryFailed:
		what = "fails"
	case KindVariableFailed:
		what = "resolves to nothing"
	default:
		return "", false
	}
	switch {
	case p.Variable != "":
		return fmt.Sprintf("- variable %s %s", printable(p.Variable), what), true
	case p.PanelID == "":
		return "", false
	}
	return repairprompt.PanelLine(printable(p.PanelID), printable(p.PanelTitle), what), true
}

func printable(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	if r := []rune(s); len(r) > maxTitleRunes {
		s = string(r[:maxTitleRunes])
	}
	return s
}
