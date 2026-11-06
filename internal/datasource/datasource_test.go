package datasource_test

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTimeRangeValidate(t *testing.T) {
	at := func(year int) time.Time { return time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC) }
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		r       datasource.TimeRange
		invalid bool
	}{
		{name: "valid", r: datasource.TimeRange{From: now.Add(-time.Hour), To: now}},
		{name: "pre-1970", r: datasource.TimeRange{From: at(1960), To: at(1961)}},
		{name: "year 1 to year 9999", r: datasource.TimeRange{From: at(1), To: at(9999)}},
		{name: "zero from", r: datasource.TimeRange{To: now}, invalid: true},
		{name: "zero to", r: datasource.TimeRange{From: now}, invalid: true},
		{name: "both zero", invalid: true},
		{name: "year 0 from", r: datasource.TimeRange{From: at(0), To: now}, invalid: true},
		{name: "year 10000 to", r: datasource.TimeRange{From: now, To: at(10000)}, invalid: true},
		{name: "year 10000 both", r: datasource.TimeRange{From: at(10000), To: at(10001)}, invalid: true},
		{name: "from equals to", r: datasource.TimeRange{From: now, To: now}, invalid: true},
		{name: "from after to", r: datasource.TimeRange{From: now, To: now.Add(-time.Second)}, invalid: true},
		{name: "offset zone in year 0", r: datasource.TimeRange{From: time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("x", 3600)), To: now}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			switch {
			case tt.invalid && !errors.Is(err, datasource.ErrInvalidQuery):
				t.Fatalf("want ErrInvalidQuery, got %v", err)
			case !tt.invalid && err != nil:
				t.Fatalf("want valid, got %v", err)
			}
		})
	}
}

func TestMergeMeta(t *testing.T) {
	frame := func(meta *datasource.FrameMeta) datasource.Frame { return datasource.Frame{Meta: meta} }
	tests := []struct {
		name   string
		frames []datasource.Frame
		want   datasource.FrameMeta
	}{
		{name: "nil metas", frames: []datasource.Frame{frame(nil), frame(nil)}},
		{name: "one truncated", frames: []datasource.Frame{frame(nil), frame(&datasource.FrameMeta{Truncated: true}), frame(nil)}, want: datasource.FrameMeta{Truncated: true}},
		{name: "rows total from the second frame", frames: []datasource.Frame{frame(&datasource.FrameMeta{Partial: true}), frame(&datasource.FrameMeta{RowsTotal: 42}), frame(&datasource.FrameMeta{RowsTotal: 7})}, want: datasource.FrameMeta{Partial: true, RowsTotal: 42}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datasource.MergeMeta(tt.frames); got != tt.want {
				t.Fatalf("MergeMeta = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFrameIsSeries(t *testing.T) {
	field := func(typ datasource.FieldType) datasource.Field { return datasource.Field{Type: typ} }
	tests := []struct {
		name   string
		fields []datasource.Field
		want   bool
	}{
		{name: "time and number", fields: []datasource.Field{field(datasource.FieldTime), field(datasource.FieldNumber)}, want: true},
		{name: "time and two numbers", fields: []datasource.Field{field(datasource.FieldTime), field(datasource.FieldNumber), field(datasource.FieldNumber)}, want: true},
		{name: "time and string", fields: []datasource.Field{field(datasource.FieldTime), field(datasource.FieldString)}},
		{name: "number first", fields: []datasource.Field{field(datasource.FieldNumber), field(datasource.FieldNumber)}},
		{name: "time only", fields: []datasource.Field{field(datasource.FieldTime)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (datasource.Frame{Fields: tt.fields}).IsSeries(); got != tt.want {
				t.Fatalf("IsSeries = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortFacetsOrdersExactLowCardinalityFieldsAndTopValuesFirst(t *testing.T) {
	f := datasource.Facets{Fields: []datasource.FacetField{
		{Name: "pod", Kind: "field", Cardinality: 2},
		{Name: "route", Kind: "field", Cardinality: 2},
		{Name: "service_name", Kind: "label", Exact: true, Cardinality: 3, Values: []datasource.FacetValue{
			{Value: "checkout", Count: 9931}, {Value: "frontend", Count: 48210}, {Value: "cart", Count: 9931},
		}},
		{Name: "level", Kind: "label", Exact: true, Cardinality: 1},
		{Name: "http_request_method", Kind: "field", Cardinality: 1},
	}}
	datasource.SortFacets(&f)
	var names []string
	for _, field := range f.Fields {
		names = append(names, field.Name)
	}
	if want := []string{"level", "service_name", "http_request_method", "pod", "route"}; !slices.Equal(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	if want := []datasource.FacetValue{{Value: "frontend", Count: 48210}, {Value: "cart", Count: 9931}, {Value: "checkout", Count: 9931}}; !slices.Equal(f.Fields[1].Values, want) {
		t.Fatalf("values = %v, want %v", f.Fields[1].Values, want)
	}
}

func TestTrimFacetsCapsFieldsAndValuesAndSaysSo(t *testing.T) {
	values := func(n int) []datasource.FacetValue {
		out := make([]datasource.FacetValue, n)
		for i := range out {
			out[i] = datasource.FacetValue{Value: strconv.Itoa(i), Count: int64(n - i)}
		}
		return out
	}
	f := datasource.Facets{Fields: []datasource.FacetField{{Name: "a", Values: values(3)}, {Name: "b", Values: values(2)}, {Name: "c"}}}
	datasource.TrimFacets(&f, 2, 2)
	if len(f.Fields) != 2 || !f.Truncated {
		t.Fatalf("fields = %+v, truncated %v", f.Fields, f.Truncated)
	}
	if a := f.Fields[0]; len(a.Values) != 2 || !a.Truncated {
		t.Fatalf("field a = %+v", a)
	}
	if b := f.Fields[1]; len(b.Values) != 2 || b.Truncated {
		t.Fatalf("field b = %+v", b)
	}
	same := datasource.Facets{Fields: []datasource.FacetField{{Name: "a", Values: values(2)}}}
	datasource.TrimFacets(&same, 1, 2)
	if same.Truncated || same.Fields[0].Truncated {
		t.Fatalf("a facet within the caps was marked truncated: %+v", same)
	}
}
