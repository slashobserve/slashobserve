package datasource_test

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestUpstreamErrorStatesItsKindOnce(t *testing.T) {
	cases := []struct {
		kind    error
		message string
		want    string
	}{
		{datasource.ErrTimeout, "query timed out", "query timed out"},
		{datasource.ErrTimeout, "query timed out after 30s", "query timed out after 30s"},
		{datasource.ErrInvalidQuery, "parse error at char 5", "invalid query: parse error at char 5"},
		{nil, "no kind", "no kind"},
	}
	for _, tc := range cases {
		err := &datasource.UpstreamError{Kind: tc.kind, Message: tc.message}
		if got := err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
		if tc.kind != nil && !errors.Is(err, tc.kind) {
			t.Errorf("errors.Is(%q, kind) = false", tc.want)
		}
	}
}
