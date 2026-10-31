package datasource

import (
	"regexp"
	"slices"
)

const AdhocOr = "or"

const AdhocNumberPattern = `^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]+)?$`

var adhocNumber = regexp.MustCompile(AdhocNumberPattern)

func IsAdhocNumber(text string) bool { return adhocNumber.MatchString(text) }

func IsAdhocOrdering(op string) bool { return op == "<" || op == ">" }

type AdhocGroups [][]AdhocFilter

func AdhocScopes(filters []AdhocFilter) []AdhocGroups {
	var order []int
	byScope := map[int]AdhocGroups{}
	for _, f := range filters {
		groups, seen := byScope[f.Scope]
		if !seen {
			order = append(order, f.Scope)
		}
		if f.Condition == AdhocOr || len(groups) == 0 {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], f)
		byScope[f.Scope] = groups
	}
	scopes := make([]AdhocGroups, len(order))
	for i, scope := range order {
		scopes[i] = byScope[scope]
	}
	return scopes
}

func AdhocUsesOr(filters []AdhocFilter) bool {
	return slices.ContainsFunc(filters, func(f AdhocFilter) bool { return f.Condition == AdhocOr })
}

func AndAdhoc(lists ...[]AdhocFilter) []AdhocFilter {
	renumber := slices.ContainsFunc(lists, AdhocUsesOr)
	var out []AdhocFilter
	next := 0
	for _, list := range lists {
		top := next
		for _, f := range list {
			if renumber {
				f.Scope += next
				top = max(top, f.Scope+1)
			}
			out = append(out, f)
		}
		next = top
	}
	return out
}
