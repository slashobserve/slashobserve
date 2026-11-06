package public

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Mode string

const (
	ModeFrozen Mode = "frozen"
	ModeFree   Mode = "free"
)

const allValue = "$__all"

var ErrSelectionNotAllowed = errors.New("selection not allowed")

type OptionsFunc func(name string, earlier map[string][]string) ([]string, error)

func (m Mode) valid() bool {
	return m == ModeFrozen || m == ModeFree
}

func EffectiveSelections(mode Mode, d *spec.Dashboard, stored, requested map[string][]string, options OptionsFunc) (map[string][]string, error) {
	out := make(map[string][]string, len(d.Variables))
	for _, v := range d.Variables {
		if values, ok := stored[v.Name]; ok {
			out[v.Name] = slices.Clone(values)
		}
	}
	if mode != ModeFree {
		return out, nil
	}
	for i, v := range d.Variables {
		values, ok := requested[v.Name]
		if !ok || !viewerMayChoose(v.Type) {
			continue
		}
		if err := checkChoice(v, values, func() ([]string, error) { return options(v.Name, earlierOf(d, i, out)) }); err != nil {
			return nil, err
		}
		out[v.Name] = slices.Clone(values)
	}
	return out, nil
}

func viewerMayChoose(t spec.VariableType) bool {
	switch t {
	case spec.QueryVariable, spec.CustomVariable, spec.IntervalVariable, spec.DatasourceVariable:
		return true
	}
	return false
}

func checkChoice(v spec.Variable, values []string, options func() ([]string, error)) error {
	if len(values) == 0 {
		return fmt.Errorf("%w: variable %q has no value", ErrSelectionNotAllowed, v.Name)
	}
	if slices.Equal(values, []string{allValue}) && v.IncludeAll && (v.Type == spec.QueryVariable || v.Type == spec.CustomVariable) {
		return nil
	}
	allowed, err := options()
	if err != nil {
		return err
	}
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("%w: variable %q has a value that is not one of its options", ErrSelectionNotAllowed, v.Name)
		}
	}
	return nil
}

func earlierOf(d *spec.Dashboard, upto int, selections map[string][]string) map[string][]string {
	names := map[string]bool{}
	for _, v := range d.Variables[:upto] {
		names[v.Name] = true
	}
	out := maps.Clone(selections)
	maps.DeleteFunc(out, func(name string, _ []string) bool { return !names[name] })
	return out
}
