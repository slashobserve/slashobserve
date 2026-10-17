package spec_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestScanQuery(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []spec.Reference
	}{
		{"bare variable", `up{pod="$pod"}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Start: 8, End: 12},
		}},
		{"braced variable", `up{pod="${pod}"}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Start: 8, End: 14},
		}},
		{"braced variable with format", `x IN (${pod:regex})`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "pod", Format: "regex", Start: 6, End: 18},
		}},
		{"rate interval", `rate(x[$__rate_interval])`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__rate_interval", Start: 7, End: 23},
		}},
		{"function macro", `WHERE $__timeFilter(Timestamp)`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "Timestamp", HasArgs: true, Start: 6, End: 30},
		}},
		{"nested parens", `$__timeFilter(toDateTime(ts))`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Args: "toDateTime(ts)", HasArgs: true, Start: 0, End: 29},
		}},
		{"macro without args", `$__timeFilter`, []spec.Reference{
			{Kind: spec.MacroRef, Name: "__timeFilter", Start: 0, End: 13},
		}},
		{"label_replace group", `label_replace(x, "a", "$1", "b", "(.*)")`, nil},
		{"trailing dollar", `SELECT 1 $`, nil},
		{"regex anchor", `up{pod=~"x$"}`, nil},
		{"two references", `a $x b ${y:csv}`, []spec.Reference{
			{Kind: spec.VariableRef, Name: "x", Start: 2, End: 4},
			{Kind: spec.VariableRef, Name: "y", Format: "csv", Start: 7, End: 15},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := spec.ScanQuery(c.query)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestScanQueryErrors(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		message string
	}{
		{"unterminated brace", `up{pod="${pod`, `unterminated "${" at offset 8`},
		{"empty format", `${pod:}`, `invalid reference "${pod:}" at offset 0`},
		{"unclosed paren", `$__timeFilter(ts`, `unclosed "(" at offset 13`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := spec.ScanQuery(c.query)
			if err == nil || !strings.Contains(err.Error(), c.message) {
				t.Fatalf("got error %v, want it to contain %q", err, c.message)
			}
		})
	}
}

func TestMacrosAndFormats(t *testing.T) {
	names := func(ms []spec.Macro) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Name)
		}
		return out
	}
	if got, want := names(spec.Macros(spec.Prometheus)), []string{"__interval", "__range", "__rate_interval"}; !slices.Equal(got, want) {
		t.Errorf("prometheus macros = %v, want %v", got, want)
	}
	if got, want := names(spec.Macros(spec.ClickHouse)), []string{"__interval", "__timeFilter", "__timeGroup"}; !slices.Equal(got, want) {
		t.Errorf("clickhouse macros = %v, want %v", got, want)
	}
	if formats := spec.VariableFormats(); !slices.IsSorted(formats) || len(formats) != 7 {
		t.Errorf("VariableFormats() = %v", formats)
	}
}
