package datasource_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestNamesAddress(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{"Syntax error: failed at position 8", false},
		{"1:5: parse error: unexpected end of input", false},
		{"Unknown table expression identifier 'default.logs'", false},
		{"", false},
		{"connect to 10.1.2.3 failed", true},
		{"dial tcp 10.1.2.3:9095: refused", true},
		{"dial tcp [fd00::5]:9095: refused", true},
		{"peer fd00::5 is down", true},
		{"store-gateway-0.mimir.svc:9095 exceeded the limit", true},
		{"clickhouse:9000 closed the connection.", true},
	}
	for _, tt := range tests {
		if got := datasource.NamesAddress(tt.message); got != tt.want {
			t.Errorf("NamesAddress(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}
