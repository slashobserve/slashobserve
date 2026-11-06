package datasource

import (
	"context"
	"slices"
	"strings"
	"time"
)

type Datasource interface {
	Type() string
	CheckHealth(ctx context.Context) error
	Discover(ctx context.Context, req DiscoverRequest) (*Catalog, error)
	Lint(ctx context.Context, query string) ([]Diagnostic, error)
	VariableValues(ctx context.Context, q Query, r TimeRange) ([]string, error)
	QueryRange(ctx context.Context, q Query, r TimeRange, step time.Duration) ([]Frame, error)
	QueryInstant(ctx context.Context, q Query, r TimeRange) ([]Frame, error)
	Close() error
}

type ExemplarSource interface {
	QueryExemplars(ctx context.Context, q Query, r TimeRange, step time.Duration) ([]Frame, error)
}

type IndexSource interface {
	Index() (pattern, timeField string)
}

type TimeRange struct{ From, To time.Time }

func (r TimeRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() {
		return Errorf(ErrInvalidQuery, "time range requires from and to")
	}
	if !validYear(r.From) || !validYear(r.To) {
		return Errorf(ErrInvalidQuery, "time range years must be between 1 and 9999")
	}
	if !r.From.Before(r.To) {
		return Errorf(ErrInvalidQuery, "time range from must be before to")
	}
	return nil
}

func validYear(t time.Time) bool {
	y := t.UTC().Year()
	return y >= 1 && y <= 9999
}

type AdhocFilter struct {
	Key, Op, Value string
	Condition      string
	Scope          int
}

type Variable struct {
	Name     string
	Values   []string
	All      bool
	AllValue bool
	Adhoc    bool
	Filters  []AdhocFilter
}

type QueryUser struct {
	ID    string
	Login string
	Email string
	Name  string
}

type Query struct {
	Text      string
	Format    string
	Variables []Variable
	Filters   []AdhocFilter
	GroupBy   []string
	Limits    Limits
	Dashboard string
	User      QueryUser

	MinInterval time.Duration
}

type DiscoverKind string

const (
	DiscoverMetrics     DiscoverKind = "metrics"
	DiscoverLabels      DiscoverKind = "labels"
	DiscoverLabelValues DiscoverKind = "label_values"
	DiscoverTables      DiscoverKind = "tables"
	DiscoverColumns     DiscoverKind = "columns"
	DiscoverSampleRows  DiscoverKind = "sample_rows"
	DiscoverInventory   DiscoverKind = "inventory"
)

const (
	DefaultDiscoverLimit = 200
	MaxDiscoverLimit     = 1000
)

type DiscoverRequest struct {
	Kind   DiscoverKind
	Search string
	Metric string
	Table  string
	Label  string
	Scope  string
	Limit  int
	Range  TimeRange
}

func (r DiscoverRequest) EffectiveLimit() int {
	switch {
	case r.Limit <= 0:
		return DefaultDiscoverLimit
	case r.Limit > MaxDiscoverLimit:
		return MaxDiscoverLimit
	default:
		return r.Limit
	}
}

type Catalog struct {
	Metrics []Metric `json:"metrics,omitempty"`
	Labels  []string `json:"labels,omitempty"`
	Values  []string `json:"values,omitempty"`
	Tables  []Table  `json:"tables,omitempty"`
	Columns []Column `json:"columns,omitempty"`
	Sample  *Frame   `json:"sample,omitempty"`

	Families        []Family `json:"families,omitempty"`
	FamiliesTotal   int      `json:"families_total,omitempty"`
	RuntimeFamilies int      `json:"runtime_families,omitempty"`
	Truncated       bool     `json:"truncated,omitempty"`
}

type Family struct {
	Name   string   `json:"name"`
	Type   string   `json:"type,omitempty"`
	Help   string   `json:"help,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Series int      `json:"series"`
	Table  string   `json:"table,omitempty"`
}

var familySeriesSuffixes = map[string][]string{
	"counter":        {"_total", "_created"},
	"histogram":      {"_bucket", "_sum", "_count", "_created"},
	"gaugehistogram": {"_bucket", "_gsum", "_gcount"},
	"summary":        {"_sum", "_count", "_created"},
}

var runtimeFamilyPrefixes = []string{"go_", "process_", "promhttp_", "scrape_"}

func IsRuntimeFamily(name string) bool {
	return name == "up" || slices.ContainsFunc(runtimeFamilyPrefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) })
}

func (f Family) SeriesNames() []string {
	names := []string{f.Name}
	for _, suffix := range familySeriesSuffixes[f.Type] {
		names = append(names, f.Name+suffix)
	}
	return names
}

type Metric struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Help  string `json:"help,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Table string `json:"table,omitempty"`
}

type Table struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Start    int      `json:"start"`
	End      int      `json:"end"`
}
