package filter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/kql"
)

type operatorStyle int

const (
	kqlOperators operatorStyle = iota
	luceneOperators
)

func (s operatorStyle) spelled(operator string) string {
	if s == luceneOperators {
		return strings.ToUpper(operator)
	}
	return operator
}

var misspelledOperators = map[string]string{
	"andd": "and", "adn": "and", "nad": "and", "annd": "and",
	"orr": "or",
	"nto": "not", "nott": "not",
}

var operatorWords = []string{"and", "or", "not"}

func operatorTypo(word string, style operatorStyle) (string, bool) {
	lower := strings.ToLower(word)
	operator, misspelled := misspelledOperators[lower]
	if !misspelled {
		wrongCase := style == luceneOperators && slices.Contains(operatorWords, lower) && word != style.spelled(lower)
		if !wrongCase {
			return "", false
		}
		operator = lower
	}
	return fmt.Sprintf("%q is searched as text; did you mean %q?", word, style.spelled(operator)), true
}

func kqlWarnings(body string) []string {
	root, _ := kql.Parse(body)
	var warnings []string
	var walk func(*kql.Node)
	walk = func(n *kql.Node) {
		if n.Kind == kql.And {
			for i := 1; i < len(n.Children)-1; i++ {
				if word, ok := bareWord(n.Children[i]); ok {
					if warning, typo := operatorTypo(word, kqlOperators); typo {
						warnings = append(warnings, warning)
					}
				}
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	if root != nil {
		walk(root)
	}
	return warnings
}

func bareWord(n *kql.Node) (string, bool) {
	return n.Value, n.Kind == kql.Free && !n.Quoted && !n.Wildcard
}
