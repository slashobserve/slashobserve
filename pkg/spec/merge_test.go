package spec_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type mergeDoc struct {
	title     string
	time      string
	variables []string
	members   string
	panels    []string
}

func mk(panels ...string) mergeDoc {
	return mergeDoc{title: "t", time: `{"from":"now-6h","to":"now"}`, panels: panels}
}

func (d mergeDoc) titled(title string) mergeDoc { d.title = title; return d }

func (d mergeDoc) timed(time string) mergeDoc { d.time = time; return d }

func (d mergeDoc) with(members string) mergeDoc { d.members = members; return d }

func (d mergeDoc) vars(variables ...string) mergeDoc { d.variables = variables; return d }

func (d mergeDoc) build(t *testing.T) *spec.Dashboard {
	t.Helper()
	variables := ""
	if len(d.variables) > 0 {
		variables = `"variables":[` + strings.Join(d.variables, ",") + `],`
	}
	members := ""
	if d.members != "" {
		members = d.members + ","
	}
	data := fmt.Sprintf(`{"specVersion":%q,"title":%q,"time":%s,%s%s"panels":[%s]}`, spec.Version, d.title, d.time, members, variables, strings.Join(d.panels, ","))
	dashboard, err := spec.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return dashboard
}

func timeseries(id, title, query string, x, y, w int) string {
	return fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":8},"datasource":"prom","queries":[{"query":%q}]}`, id, title, x, y, w, query)
}

func row(id string, y int, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, id, y, strings.Join(children, ","))
}

func variable(name string, options ...string) string {
	quoted := make([]string, len(options))
	for i, o := range options {
		quoted[i] = fmt.Sprintf("%q", o)
	}
	return fmt.Sprintf(`{"name":%q,"type":"custom","options":[%s]}`, name, strings.Join(quoted, ","))
}

func marshalled(t *testing.T, d *spec.Dashboard) string {
	t.Helper()
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func conflictKeys(conflicts []spec.Conflict) []string {
	keys := make([]string, len(conflicts))
	for i, c := range conflicts {
		keys[i] = fmt.Sprintf("%s/%s/%s", c.Kind, c.ID, c.Reason)
	}
	return keys
}

func TestMerge3(t *testing.T) {
	p1 := timeseries("p1", "p1", "up", 0, 0, 12)
	p2 := timeseries("p2", "p2", "up", 12, 0, 12)
	p3 := timeseries("p3", "p3", "up", 0, 9, 12)
	base := mk(p1, p2, row("r1", 8, p3))

	a := timeseries("a", "a", "up", 0, 0, 24)
	b := timeseries("b", "b", "up", 0, 8, 24)
	c := timeseries("c", "c", "up", 0, 16, 24)
	narrow := mk(timeseries("p1", "p1", "up", 0, 0, 6), p2)

	tests := []struct {
		name      string
		base      mergeDoc
		ours      mergeDoc
		theirs    mergeDoc
		choices   []spec.Choice
		want      *mergeDoc
		conflicts []string
	}{
		{
			name:   "disjoint edits keep both",
			base:   base,
			ours:   mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)),
			want:   ptr(mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3))),
		},
		{
			name:   "a panel theirs added to a row lands in that row at its place",
			base:   base,
			ours:   mk(timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			theirs: mk(p1, p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))),
			want:   ptr(mk(timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12)))),
		},
		{
			name:   "a panel theirs deleted stays deleted",
			base:   base,
			ours:   mk(p1, p2, row("r1", 8, timeseries("p3", "renamed", "up", 0, 9, 12))),
			theirs: mk(p1, row("r1", 8, p3)),
			want:   ptr(mk(p1, row("r1", 8, timeseries("p3", "renamed", "up", 0, 9, 12)))),
		},
		{
			name:   "a resize by theirs is carried",
			base:   base,
			ours:   mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "p1", "up", 0, 0, 6), p2, row("r1", 8, p3)),
			want:   ptr(mk(timeseries("p1", "p1", "up", 0, 0, 6), timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3))),
		},
		{
			name:   "a newer move of a panel theirs edited keeps the newer place",
			base:   base,
			ours:   mk(timeseries("p1", "p1", "up", 0, 20, 12), p2, row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)),
			want:   ptr(mk(timeseries("p1", "p1", "rate(x[5m])", 0, 20, 12), p2, row("r1", 8, p3))),
		},
		{
			name:   "an unchanged theirs takes ours",
			base:   base,
			ours:   mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs: base,
			want:   ptr(mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3))),
		},
		{
			name:      "both changing one panel conflicts",
			base:      base,
			ours:      mk(timeseries("p1", "renamed", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			theirs:    mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)),
			conflicts: []string{"panel/p1/both_changed"},
		},
		{
			name:      "theirs deleting what ours edited conflicts",
			base:      base,
			ours:      mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs:    mk(p1, row("r1", 8, p3)),
			conflicts: []string{"panel/p2/changed_and_removed"},
		},
		{
			name:      "both adding the same id with different content conflicts",
			base:      base,
			ours:      mk(p1, p2, row("r1", 8, p3), timeseries("p9", "theirs", "up", 0, 30, 12)),
			theirs:    mk(p1, p2, row("r1", 8, p3), timeseries("p9", "mine", "up", 0, 30, 12)),
			conflicts: []string{"panel/p9/both_changed"},
		},
		{
			name:   "theirs changing a dashboard field merges with ours",
			base:   base,
			ours:   mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)).titled("retitled"),
			want:   ptr(mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), timeseries("p2", "renamed", "up", 12, 0, 12), row("r1", 8, p3)).titled("retitled")),
		},
		{
			name:      "a panel of theirs that overlaps a newer panel is invalid on theirs",
			base:      base,
			ours:      mk(p1, p2, row("r1", 8, p3), timeseries("p5", "p5", "up", 0, 30, 12)),
			theirs:    mk(p1, p2, row("r1", 8, p3), timeseries("p4", "p4", "up", 0, 30, 12)),
			conflicts: []string{"panel/p4/invalid"},
		},
		{
			name:   "each side moving a different field of one panel keeps both",
			base:   narrow,
			ours:   mk(timeseries("p1", "p1", "up", 2, 0, 6), p2),
			theirs: mk(timeseries("p1", "p1", "up", 0, 0, 8), p2),
			want:   ptr(mk(timeseries("p1", "p1", "up", 2, 0, 8), p2)),
		},
		{
			name:   "both sides making the same content change is no conflict",
			base:   base,
			ours:   mk(timeseries("p1", "same", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "same", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			want:   ptr(mk(timeseries("p1", "same", "up", 0, 0, 12), p2, row("r1", 8, p3))),
		},
		{
			name:   "theirs and ours changing different variables keeps both",
			base:   mk(p1).vars(variable("env", "a", "b"), variable("region", "x", "y")),
			ours:   mk(p1).vars(variable("env", "a", "b"), variable("region", "x", "z")),
			theirs: mk(p1).vars(variable("env", "a", "c"), variable("region", "x", "y")),
			want:   ptr(mk(p1).vars(variable("env", "a", "c"), variable("region", "x", "z"))),
		},
		{
			name:      "both changing one variable conflicts on its name",
			base:      mk(p1).vars(variable("env", "a", "b")),
			ours:      mk(p1).vars(variable("env", "a", "z")),
			theirs:    mk(p1).vars(variable("env", "a", "c")),
			conflicts: []string{"variable/env/both_changed"},
		},
		{
			name:   "theirs changing time and ours changing tags keeps both",
			base:   mk(p1).with(`"tags":["x"]`),
			ours:   mk(p1).with(`"tags":["x","y"]`),
			theirs: mk(p1).with(`"tags":["x"]`).timed(`{"from":"now-1h","to":"now"}`),
			want:   ptr(mk(p1).with(`"tags":["x","y"]`).timed(`{"from":"now-1h","to":"now"}`)),
		},
		{
			name:      "both changing the title differently conflicts on the setting",
			base:      mk(p1),
			ours:      mk(p1).titled("ours"),
			theirs:    mk(p1).titled("theirs"),
			conflicts: []string{"setting/title/both_changed"},
		},
		{
			name:      "ours removing a row blames the panel theirs added in it",
			base:      base,
			ours:      mk(p1, p2),
			theirs:    mk(p1, p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))),
			conflicts: []string{"panel/p4/changed_and_removed"},
		},
		{
			name:   "theirs reordering with ours keeping the order takes theirs",
			base:   mk(a, b, c),
			ours:   mk(a, b, c),
			theirs: mk(c, a, b),
			want:   ptr(mk(c, a, b)),
		},
		{
			name:   "both reordering takes ours",
			base:   mk(a, b, c),
			ours:   mk(b, a, c),
			theirs: mk(c, a, b),
			want:   ptr(mk(b, a, c)),
		},
		{
			name:    "choosing theirs resolves a both-changed panel with its content and placement",
			base:    mk(p1, p2),
			ours:    mk(timeseries("p1", "ours", "up", 0, 0, 12), p2),
			theirs:  mk(timeseries("p1", "theirs", "up", 0, 0, 6), p2),
			choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "p1", Take: spec.SideTheirs}},
			want:    ptr(mk(timeseries("p1", "theirs", "up", 0, 0, 6), p2)),
		},
		{
			name:    "choosing ours resolves a both-changed panel with ours",
			base:    mk(p1, p2),
			ours:    mk(timeseries("p1", "ours", "up", 0, 0, 12), p2),
			theirs:  mk(timeseries("p1", "theirs", "up", 0, 0, 6), p2),
			choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "p1", Take: spec.SideOurs}},
			want:    ptr(mk(timeseries("p1", "ours", "up", 0, 0, 12), p2)),
		},
		{
			name:    "choosing the side that lacks a panel removes it",
			base:    mk(p1, p2),
			ours:    mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12)),
			theirs:  mk(p1),
			choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "p2", Take: spec.SideTheirs}},
			want:    ptr(mk(p1)),
		},
		{
			name:    "a choice for a unit that would not conflict is applied anyway",
			base:    mk(p1, p2),
			ours:    mk(p1, timeseries("p2", "renamed", "up", 12, 0, 12)),
			theirs:  mk(p1, p2),
			choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "p2", Take: spec.SideTheirs}},
			want:    ptr(mk(p1, p2)),
		},
		{
			name:    "choices resolve a variable and a setting",
			base:    mk(p1).vars(variable("env", "a", "b")),
			ours:    mk(p1).vars(variable("env", "a", "z")).titled("ours"),
			theirs:  mk(p1).vars(variable("env", "a", "c")).titled("theirs"),
			choices: []spec.Choice{{Kind: spec.UnitVariable, ID: "env", Take: spec.SideTheirs}, {Kind: spec.UnitSetting, ID: "title", Take: spec.SideOurs}},
			want:    ptr(mk(p1).vars(variable("env", "a", "c")).titled("ours")),
		},
		{
			name:      "conflicts list panels before variables before settings in ours order",
			base:      mk(p1, p2).vars(variable("env", "a", "b")),
			ours:      mk(timeseries("p1", "o1", "up", 0, 0, 12), timeseries("p2", "o2", "up", 12, 0, 12)).vars(variable("env", "a", "z")).titled("ours"),
			theirs:    mk(timeseries("p1", "t1", "up", 0, 0, 12), timeseries("p2", "t2", "up", 12, 0, 12)).vars(variable("env", "a", "c")).titled("theirs"),
			conflicts: []string{"panel/p1/both_changed", "panel/p2/both_changed", "variable/env/both_changed", "setting/title/both_changed"},
		},
		{
			name:      "a merged variable that reads a variable theirs removed is invalid on the reader",
			base:      mk(p1).vars(variable("env", "a", "b")),
			ours:      mk(p1).vars(variable("env", "a", "b"), `{"name":"svc","type":"query","datasource":"prom","query":"label_values(up{env=\"${env}\"}, job)"}`),
			theirs:    mk(p1),
			conflicts: []string{"variable/svc/invalid"},
		},
		{
			name:      "a merged document over the panel limit is blamed on the panels setting",
			base:      mk(p1),
			ours:      mk(append([]string{p1}, gridOf("o", 0, 120)...)...),
			theirs:    mk(append([]string{p1}, gridOf("t", 400, 120)...)...),
			conflicts: []string{"setting/panels/invalid"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spec.Merge3(tt.base.build(t), tt.ours.build(t), tt.theirs.build(t), tt.choices...)
			if err != nil {
				t.Fatal(err)
			}
			if got.Conflicts == nil {
				t.Fatal("Conflicts is nil, want a non-nil slice")
			}
			if tt.want == nil {
				if got.Dashboard != nil {
					t.Fatal("a merge with conflicts returned a dashboard")
				}
				if keys := conflictKeys(got.Conflicts); !slices.Equal(keys, tt.conflicts) {
					t.Fatalf("conflicts = %v, want %v", keys, tt.conflicts)
				}
				return
			}
			if len(got.Conflicts) > 0 || got.Dashboard == nil {
				t.Fatalf("conflicts = %v, dashboard nil = %v, want a clean merge", conflictKeys(got.Conflicts), got.Dashboard == nil)
			}
			if gotJSON, wantJSON := marshalled(t, got.Dashboard), marshalled(t, tt.want.build(t)); gotJSON != wantJSON {
				t.Fatalf("merged\n%s\nwant\n%s", gotJSON, wantJSON)
			}
		})
	}
}

func TestMerge3Messages(t *testing.T) {
	p1 := timeseries("p1", "p1", "up", 0, 0, 12)
	p2 := timeseries("p2", "p2", "up", 12, 0, 12)
	p3 := timeseries("p3", "p3", "up", 0, 9, 12)
	base := mk(p1, p2, row("r1", 8, p3))

	tests := []struct {
		name   string
		ours   mergeDoc
		theirs mergeDoc
		want   string
	}{
		{
			name:   "both changed",
			ours:   mk(timeseries("p1", "ours", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			theirs: mk(timeseries("p1", "theirs", "up", 0, 0, 12), p2, row("r1", 8, p3)),
			want:   `panel "p1" was changed on both sides`,
		},
		{
			name:   "changed and removed",
			ours:   mk(p1, timeseries("p2", "ours", "up", 12, 0, 12), row("r1", 8, p3)),
			theirs: mk(p1, row("r1", 8, p3)),
			want:   `panel "p2" was removed on one side and changed on the other`,
		},
		{
			name:   "in a removed row",
			ours:   mk(p1, p2),
			theirs: mk(p1, p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))),
			want:   `panel "p4" is in row "r1", which was removed`,
		},
		{
			name:   "invalid prefixes the unit",
			ours:   mk(p1, p2, row("r1", 8, p3), timeseries("p5", "p5", "up", 0, 30, 12)),
			theirs: mk(p1, p2, row("r1", 8, p3), timeseries("p4", "p4", "up", 0, 30, 12)),
			want:   `panel "p4": `,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spec.Merge3(base.build(t), tt.ours.build(t), tt.theirs.build(t))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Conflicts) != 1 || !strings.HasPrefix(got.Conflicts[0].Message, tt.want) {
				t.Fatalf("conflicts = %+v, want one starting %q", got.Conflicts, tt.want)
			}
		})
	}
}

func TestMerge3RefusesAChoiceForAnUnknownUnit(t *testing.T) {
	d := mk(timeseries("p1", "p1", "up", 0, 0, 12)).build(t)
	for _, kind := range []spec.UnitKind{spec.UnitPanel, spec.UnitVariable, spec.UnitSetting} {
		_, err := spec.Merge3(d, d, d, spec.Choice{Kind: kind, ID: "missing", Take: spec.SideOurs})
		if !errors.Is(err, spec.ErrUnknownChoice) {
			t.Fatalf("kind %s: err = %v, want ErrUnknownChoice", kind, err)
		}
	}
	if _, err := spec.Merge3(d, d, d, spec.Choice{Kind: spec.UnitPanel, ID: "p1", Take: "both"}); err == nil {
		t.Fatal("a choice taking an unknown side was accepted")
	}
}

func TestMerge3DoesNotMutateItsInputs(t *testing.T) {
	p1 := timeseries("p1", "p1", "up", 0, 0, 12)
	p2 := timeseries("p2", "p2", "up", 12, 0, 12)
	p3 := timeseries("p3", "p3", "up", 0, 9, 12)
	base := mk(p1, p2, row("r1", 8, p3)).vars(variable("env", "a", "b")).build(t)
	ours := mk(timeseries("p1", "ours", "up", 0, 0, 12), p2, row("r1", 8, p3)).vars(variable("env", "a", "b")).build(t)
	theirs := mk(timeseries("p1", "theirs", "up", 0, 0, 6), p2, row("r1", 8, p3, timeseries("p4", "p4", "up", 12, 9, 12))).build(t)
	before := []string{marshalled(t, base), marshalled(t, ours), marshalled(t, theirs)}

	for _, choices := range [][]spec.Choice{nil, {{Kind: spec.UnitPanel, ID: "p1", Take: spec.SideTheirs}}} {
		if _, err := spec.Merge3(base, ours, theirs, choices...); err != nil {
			t.Fatal(err)
		}
	}
	after := []string{marshalled(t, base), marshalled(t, ours), marshalled(t, theirs)}
	if !slices.Equal(before, after) {
		t.Fatalf("inputs changed\nbefore %v\nafter  %v", before, after)
	}
}

func TestChanges(t *testing.T) {
	p1 := timeseries("p1", "p1", "up", 0, 0, 12)
	p2 := timeseries("p2", "p2", "up", 12, 0, 12)
	p3 := timeseries("p3", "p3", "up", 0, 9, 12)
	from := mk(p1, p2, row("r1", 8, p3)).vars(variable("env", "a"), variable("region", "x"))

	tests := []struct {
		name string
		to   mergeDoc
		want []spec.UnitChange
	}{
		{
			name: "nothing changed",
			to:   from,
			want: []spec.UnitChange{},
		},
		{
			name: "a panel added",
			to:   mk(p1, p2, row("r1", 8, p3), timeseries("p4", "p4", "up", 0, 30, 12)).vars(from.variables...),
			want: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p4", Change: spec.ChangeAdded}},
		},
		{
			name: "a panel removed",
			to:   mk(p1, row("r1", 8, p3)).vars(from.variables...),
			want: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p2", Change: spec.ChangeRemoved}},
		},
		{
			name: "a panel changed",
			to:   mk(timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), p2, row("r1", 8, p3)).vars(from.variables...),
			want: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p1", Change: spec.ChangeChanged}},
		},
		{
			name: "a panel that only changed y is moved",
			to:   mk(timeseries("p1", "p1", "up", 0, 20, 12), p2, row("r1", 8, p3)).vars(from.variables...),
			want: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p1", Change: spec.ChangeMoved}},
		},
		{
			name: "a panel moved into a row is moved",
			to:   mk(p1, row("r1", 8, p3, timeseries("p2", "p2", "up", 12, 0, 12))).vars(from.variables...),
			want: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p2", Change: spec.ChangeMoved}},
		},
		{
			name: "a variable added and variables moved",
			to:   mk(p1, p2, row("r1", 8, p3)).vars(variable("region", "x"), variable("env", "a"), variable("zone", "z")),
			want: []spec.UnitChange{
				{Kind: spec.UnitVariable, ID: "region", Change: spec.ChangeMoved},
				{Kind: spec.UnitVariable, ID: "env", Change: spec.ChangeMoved},
				{Kind: spec.UnitVariable, ID: "zone", Change: spec.ChangeAdded},
			},
		},
		{
			name: "a variable that kept its place is not moved by an addition",
			to:   mk(p1, p2, row("r1", 8, p3)).vars(variable("env", "a"), variable("zone", "z"), variable("region", "x")),
			want: []spec.UnitChange{{Kind: spec.UnitVariable, ID: "zone", Change: spec.ChangeAdded}},
		},
		{
			name: "a setting changed",
			to:   mk(p1, p2, row("r1", 8, p3)).vars(from.variables...).titled("retitled"),
			want: []spec.UnitChange{{Kind: spec.UnitSetting, ID: "title", Change: spec.ChangeChanged}},
		},
		{
			name: "a setting added",
			to:   mk(p1, p2, row("r1", 8, p3)).vars(from.variables...).with(`"tags":["x"]`),
			want: []spec.UnitChange{{Kind: spec.UnitSetting, ID: "tags", Change: spec.ChangeAdded}},
		},
		{
			name: "panels in to order, then removed panels, then variables, then settings by name",
			to: mk(timeseries("p4", "p4", "up", 0, 30, 12), timeseries("p1", "p1", "rate(x[5m])", 0, 0, 12), row("r1", 8, p3)).
				vars(variable("env", "a", "b")).with(`"description":"d"`).titled("retitled"),
			want: []spec.UnitChange{
				{Kind: spec.UnitPanel, ID: "p4", Change: spec.ChangeAdded},
				{Kind: spec.UnitPanel, ID: "p1", Change: spec.ChangeChanged},
				{Kind: spec.UnitPanel, ID: "p2", Change: spec.ChangeRemoved},
				{Kind: spec.UnitVariable, ID: "env", Change: spec.ChangeChanged},
				{Kind: spec.UnitVariable, ID: "region", Change: spec.ChangeRemoved},
				{Kind: spec.UnitSetting, ID: "description", Change: spec.ChangeAdded},
				{Kind: spec.UnitSetting, ID: "title", Change: spec.ChangeChanged},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spec.Changes(from.build(t), tt.to.build(t))
			if got == nil || !slices.Equal(got, tt.want) {
				t.Fatalf("changes = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func gridOf(prefix string, y, n int) []string {
	panels := make([]string, n)
	for i := range panels {
		id := fmt.Sprintf("%s%d", prefix, i)
		panels[i] = timeseries(id, id, "up", (i%4)*6, y+8+(i/4)*8, 6)
	}
	return panels
}

func ptr[T any](v T) *T { return &v }
