package public

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func selectionsDashboard() *spec.Dashboard {
	return &spec.Dashboard{Variables: []spec.Variable{
		{Name: "env", Type: spec.CustomVariable, Options: []spec.VariableOption{{Value: "prod"}, {Value: "staging"}}, IncludeAll: true, Multi: true},
		{Name: "pod", Type: spec.QueryVariable},
		{Name: "step", Type: spec.IntervalVariable},
		{Name: "ds", Type: spec.DatasourceVariable},
		{Name: "note", Type: spec.TextboxVariable},
		{Name: "filters", Type: spec.AdhocVariable},
		{Name: "by", Type: spec.GroupByVariable},
		{Name: "fixed", Type: spec.ConstantVariable, Value: "x"},
		{Name: "region", Type: spec.QueryVariable},
	}}
}

type optionCalls struct {
	earlier map[string]map[string][]string
}

func (o *optionCalls) options(name string, earlier map[string][]string) ([]string, error) {
	if o.earlier == nil {
		o.earlier = map[string]map[string][]string{}
	}
	o.earlier[name] = earlier
	switch name {
	case "env":
		return []string{"prod", "staging"}, nil
	case "pod":
		if reflect.DeepEqual(earlier["env"], []string{"staging"}) {
			return []string{"web-s1"}, nil
		}
		return []string{"web-1", "web-2"}, nil
	case "step":
		return []string{"$__auto", "1m", "5m"}, nil
	case "ds":
		return []string{"prom", "prom2"}, nil
	case "region":
		return nil, errors.New("datasource down")
	}
	return nil, nil
}

func TestEffectiveSelectionsFrozenIgnoresEveryRequest(t *testing.T) {
	d := selectionsDashboard()
	stored := map[string][]string{"env": {"prod"}, "note": {"hi"}, "gone": {"old"}}
	requested := map[string][]string{"env": {"staging"}, "pod": {"web-1"}, "note": {"evil"}}
	calls := &optionCalls{}
	got, err := EffectiveSelections(ModeFrozen, d, stored, requested, calls.options)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"env": {"prod"}, "note": {"hi"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if len(calls.earlier) != 0 {
		t.Fatalf("frozen mode computed options for %v", calls.earlier)
	}
}

func TestEffectiveSelectionsFree(t *testing.T) {
	stored := map[string][]string{"env": {"prod"}, "note": {"stored note"}, "by": {"job"}}
	cases := []struct {
		name      string
		requested map[string][]string
		want      map[string][]string
		refused   bool
	}{
		{"nothing requested keeps the stored selections", nil, stored, false},
		{"a custom option", map[string][]string{"env": {"staging"}}, map[string][]string{"env": {"staging"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"several options of a multi variable", map[string][]string{"env": {"prod", "staging"}}, map[string][]string{"env": {"prod", "staging"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"all when the variable includes all", map[string][]string{"env": {"$__all"}}, map[string][]string{"env": {"$__all"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"a custom value outside the options", map[string][]string{"env": {"dev"}}, nil, true},
		{"all mixed with a value", map[string][]string{"env": {"$__all", "prod"}}, nil, true},
		{"an empty choice", map[string][]string{"env": {}}, nil, true},
		{"a query option", map[string][]string{"pod": {"web-2"}}, map[string][]string{"env": {"prod"}, "pod": {"web-2"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"a query value outside the options", map[string][]string{"pod": {"secret-pod"}}, nil, true},
		{"all on a query variable without includeAll", map[string][]string{"pod": {"$__all"}}, nil, true},
		{"an interval option", map[string][]string{"step": {"5m"}}, map[string][]string{"env": {"prod"}, "step": {"5m"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"auto interval", map[string][]string{"step": {"$__auto"}}, map[string][]string{"env": {"prod"}, "step": {"$__auto"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"an interval outside the options", map[string][]string{"step": {"1s"}}, nil, true},
		{"a datasource option", map[string][]string{"ds": {"prom2"}}, map[string][]string{"env": {"prod"}, "ds": {"prom2"}, "note": {"stored note"}, "by": {"job"}}, false},
		{"a datasource outside the options", map[string][]string{"ds": {"other"}}, nil, true},
		{"textbox, adhoc, group by and constant keep the stored or default", map[string][]string{"note": {"x"}, "filters": {"job|=|api"}, "by": {"pod"}, "fixed": {"y"}}, stored, false},
		{"an unknown variable is ignored", map[string][]string{"nope": {"x"}}, stored, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := &optionCalls{}
			got, err := EffectiveSelections(ModeFree, selectionsDashboard(), stored, tc.requested, calls.options)
			if tc.refused {
				if !errors.Is(err, ErrSelectionNotAllowed) {
					t.Fatalf("err = %v, want ErrSelectionNotAllowed", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEffectiveSelectionsCheckChainedOptionsAgainstEarlierChoices(t *testing.T) {
	calls := &optionCalls{}
	requested := map[string][]string{"env": {"staging"}, "pod": {"web-s1"}}
	got, err := EffectiveSelections(ModeFree, selectionsDashboard(), map[string][]string{"note": {"n"}}, requested, calls.options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["pod"], []string{"web-s1"}) {
		t.Fatalf("pod = %v", got["pod"])
	}
	if want := map[string][]string{"env": {"staging"}}; !reflect.DeepEqual(calls.earlier["pod"], want) {
		t.Fatalf("pod options were computed with %v, want %v", calls.earlier["pod"], want)
	}
	if _, err := EffectiveSelections(ModeFree, selectionsDashboard(), nil, map[string][]string{"env": {"prod"}, "pod": {"web-s1"}}, calls.options); !errors.Is(err, ErrSelectionNotAllowed) {
		t.Fatalf("a pod of another env was accepted: %v", err)
	}
}

func TestEffectiveSelectionsPassesOptionErrorsThrough(t *testing.T) {
	calls := &optionCalls{}
	_, err := EffectiveSelections(ModeFree, selectionsDashboard(), nil, map[string][]string{"region": {"eu"}}, calls.options)
	if err == nil || errors.Is(err, ErrSelectionNotAllowed) {
		t.Fatalf("err = %v, want the options error", err)
	}
}

func TestUsable(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	cases := []struct {
		name       string
		expiresAt  *time.Time
		usesUser   bool
		wantOK     bool
		wantReason string
	}{
		{"no expiry", nil, false, true, ""},
		{"expires later", &future, false, true, ""},
		{"expired", &past, false, false, "expired"},
		{"expires right now", &now, false, false, "expired"},
		{"user scoped", nil, true, false, "user_scoped"},
		{"expired and user scoped", &past, true, false, "expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := usable(Link{ExpiresAt: tc.expiresAt}, tc.usesUser, now)
			if ok != tc.wantOK || reason != tc.wantReason {
				t.Fatalf("usable = %v, %q; want %v, %q", ok, reason, tc.wantOK, tc.wantReason)
			}
		})
	}
}
