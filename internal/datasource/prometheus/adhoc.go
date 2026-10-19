package prometheus

import (
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func applyFilters(expr string, filters []datasource.AdhocFilter) (string, error) {
	tree, err := promParser.ParseExpr(expr)
	if err != nil {
		return "", err
	}
	var failure error
	parser.Inspect(tree, func(node parser.Node, _ []parser.Node) error {
		selector, ok := node.(*parser.VectorSelector)
		if !ok || failure != nil {
			return nil
		}
		for _, f := range filters {
			matchType, err := matchTypeOf(f.Op)
			if err != nil {
				failure = err
				return nil
			}
			matcher, err := labels.NewMatcher(matchType, f.Key, f.Value)
			if err != nil {
				failure = fmt.Errorf("adhoc filter on %q: %w", f.Key, err)
				return nil
			}
			selector.LabelMatchers = append(selector.LabelMatchers, matcher)
		}
		return nil
	})
	if failure != nil {
		return "", failure
	}
	return tree.String(), nil
}

func matchTypeOf(op string) (labels.MatchType, error) {
	switch op {
	case "=":
		return labels.MatchEqual, nil
	case "!=":
		return labels.MatchNotEqual, nil
	case "=~":
		return labels.MatchRegexp, nil
	case "!~":
		return labels.MatchNotRegexp, nil
	}
	return 0, fmt.Errorf("adhoc filter operator %q is not supported", op)
}
