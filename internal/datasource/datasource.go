package datasource

import (
	"context"
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

type Variable struct {
	Name   string
	Values []string
	All    bool
}

type QueryUser struct {
	ID    string
	Login string
	Email string
	Name  string
}

type Query struct {
	Text      string
	Variables []Variable
	Limits    Limits
	Dashboard string
	User      QueryUser
}

type DiscoverKind string

const (
	DiscoverMetrics     DiscoverKind = "metrics"
	DiscoverLabels      DiscoverKind = "labels"
	DiscoverLabelValues DiscoverKind = "label_values"
	DiscoverTables      DiscoverKind = "tables"
	DiscoverColumns     DiscoverKind = "columns"
	DiscoverSampleRows  DiscoverKind = "sample_rows"
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
	Limit  int
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
