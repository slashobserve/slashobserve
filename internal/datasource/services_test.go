package datasource_test

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestResolveServiceSignal(t *testing.T) {
	tests := []struct {
		dsType, requested, want string
		invalid                 bool
	}{
		{"loki", "", "logs", false},
		{"loki", "logs", "logs", false},
		{"loki", "traces", "", true},
		{"tempo", "", "traces", false},
		{"tempo", "logs", "", true},
		{"clickhouse", "", "logs", false},
		{"clickhouse", "traces", "traces", false},
		{"elasticsearch", "traces", "", true},
		{"opensearch", "", "logs", false},
		{"prometheus", "", "metrics", false},
		{"prometheus", "metrics", "metrics", false},
		{"prometheus", "logs", "", true},
		{"mysql", "", "", true},
	}
	for _, tt := range tests {
		got, err := datasource.ResolveServiceSignal(tt.dsType, tt.requested)
		if tt.invalid {
			if !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Errorf("%s %q: err = %v, want ErrInvalidQuery", tt.dsType, tt.requested, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s %q = %q, %v; want %q", tt.dsType, tt.requested, got, err, tt.want)
		}
	}
}
