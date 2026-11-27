package grounding

import (
	"encoding/json"
	"strings"
)

const (
	kindAnswers = "answers"

	missingAnswersFix = "the markdown states a figure but answers is empty: list each quantity the question asks for in answers with the one fact that answers it"
)

func MissingAnswers(input json.RawMessage, question string) (Problem, bool) {
	var in reviewed
	if json.Unmarshal(input, &in) != nil || len(in.Answers) > 0 {
		return Problem{}, false
	}
	for _, text := range []string{in.Markdown, in.Impact} {
		if len(citations(text)) > 0 || len(UncitedFigures(text, question)) > 0 {
			return Problem{Kind: kindAnswers, Text: strings.TrimSpace(firstLine(in.Markdown)), Fix: missingAnswersFix}, true
		}
	}
	return Problem{}, false
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return clip(line)
}
