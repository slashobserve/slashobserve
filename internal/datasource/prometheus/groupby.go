package prometheus

import (
	"slices"

	"github.com/prometheus/prometheus/promql/parser"
)

func applyGroupBy(expr string, keys []string) (string, error) {
	if len(keys) == 0 {
		return expr, nil
	}
	tree, err := promParser.ParseExpr(expr)
	if err != nil {
		return "", err
	}
	parser.Inspect(tree, func(node parser.Node, _ []parser.Node) error {
		aggregation, ok := node.(*parser.AggregateExpr)
		if !ok || aggregation.Without {
			return nil
		}
		for _, key := range keys {
			if !slices.Contains(aggregation.Grouping, key) {
				aggregation.Grouping = append(aggregation.Grouping, key)
			}
		}
		return nil
	})
	return tree.String(), nil
}
