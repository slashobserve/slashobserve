package filter

import (
	"fmt"
	"strings"

	"github.com/slashobserve/slashobserve/internal/kql"
)

const minOperatorTypoLength = 3

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

func operatorTypo(word string, style operatorStyle) (string, bool) {
	if len(word) < minOperatorTypoLength {
		return "", false
	}
	lower := strings.ToLower(word)
	for _, operator := range []string{"and", "or", "not"} {
		if withinOneEdit(lower, operator) {
			return fmt.Sprintf("%q is searched as text; did you mean %q?", word, style.spelled(operator)), true
		}
	}
	return "", false
}

func withinOneEdit(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	switch len(a) - len(b) {
	case 0:
		return oneSubstitutionOrSwap(a, b)
	case 1:
		return oneInsertion(a, b)
	}
	return false
}

func oneSubstitutionOrSwap(a, b string) bool {
	i := 0
	for i < len(a) && a[i] == b[i] {
		i++
	}
	if i == len(a) {
		return true
	}
	if a[i+1:] == b[i+1:] {
		return true
	}
	return i+1 < len(a) && a[i] == b[i+1] && a[i+1] == b[i] && a[i+2:] == b[i+2:]
}

func oneInsertion(longer, shorter string) bool {
	i := 0
	for i < len(shorter) && longer[i] == shorter[i] {
		i++
	}
	return longer[i+1:] == shorter[i:]
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
