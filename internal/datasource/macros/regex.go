package macros

import (
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func ExpandRegex(text string, vars []datasource.Variable) (string, error) {
	refs, err := spec.ScanQuery(text)
	if err != nil {
		return "", scanFailure(err)
	}
	var b strings.Builder
	last := 0
	for _, ref := range refs {
		idx := slices.IndexFunc(vars, func(v datasource.Variable) bool { return v.Name == ref.Name })
		if ref.Kind != spec.VariableRef || idx < 0 {
			continue
		}
		out, err := expandRegexVariable(ref, vars[idx])
		if err != nil {
			return "", err
		}
		b.WriteString(text[last:ref.Start])
		b.WriteString(out)
		last = ref.End
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

func expandRegexVariable(ref spec.Reference, v datasource.Variable) (string, error) {
	switch {
	case ref.Format != "" && ref.Format != "regex":
		return "", refErrorf(ref, "format %q is not available in a variable regex for %s; use regex", ref.Format, refLabel(ref))
	case v.Adhoc:
		return "", refErrorf(ref, "adhoc variable %s cannot be referenced in a variable regex", refLabel(ref))
	case v.AllValue:
		if len(v.Values) != 1 || hasControlByte(v.Values[0]) {
			return "", refErrorf(ref, "variable %s has an invalid allValue", refLabel(ref))
		}
		return v.Values[0], nil
	case !v.All && len(v.Values) == 0:
		return "", refErrorf(ref, "no value selected for %s", refLabel(ref))
	case slices.ContainsFunc(v.Values, hasControlByte):
		return "", refErrorf(ref, "variable %s has a value with a control character", refLabel(ref))
	}
	text, err := applyFormat(ref, "regex", v, false)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(text, "/", `\/`), nil
}
