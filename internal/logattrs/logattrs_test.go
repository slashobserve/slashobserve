package logattrs

import (
	"errors"
	"fmt"
	"log/slog"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func attrMap(attrs []any) map[string]string {
	out := map[string]string{}
	for _, a := range attrs {
		attr := a.(slog.Attr)
		out[attr.Key] = attr.Value.String()
	}
	return out
}

func TestSafe(t *testing.T) {
	leak := "host db.internal user bob"
	tests := []struct {
		name string
		err  error
		want map[string]string
	}{
		{"plain", errors.New(leak), map[string]string{"error_type": "*errors.errorString"}},
		{"multi wrap reports the innermost", fmt.Errorf("a: %w", fmt.Errorf("b: %w", syscall.ECONNREFUSED)), map[string]string{"error_type": "syscall.Errno", "errno": "connection refused"}},
		{"joined reports the last", errors.Join(errors.New(leak), &pgconn.PgError{Code: "28P01", Message: leak}), map[string]string{"error_type": "*pgconn.PgError", "sqlstate": "28P01"}},
		{"valid sqlstate", fmt.Errorf("x: %w", &pgconn.PgError{Code: "23505", Message: leak}), map[string]string{"error_type": "*pgconn.PgError", "sqlstate": "23505"}},
		{"invalid sqlstate is dropped", &pgconn.PgError{Code: "bad code with " + leak}, map[string]string{"error_type": "*pgconn.PgError"}},
		{"errno beside a wrapped message", fmt.Errorf("dial %s: %w", leak, syscall.ETIMEDOUT), map[string]string{"error_type": "syscall.Errno", "errno": syscall.ETIMEDOUT.Error()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attrMap(Safe(tt.err))
			if len(got) != len(tt.want) {
				t.Fatalf("Safe = %v, want %v", got, tt.want)
			}
			for key, want := range tt.want {
				if got[key] != want {
					t.Errorf("%s = %q, want %q", key, got[key], want)
				}
			}
			for key, value := range got {
				if value == leak {
					t.Errorf("%s carries the error text", key)
				}
			}
		})
	}
}

func TestFailurePutsTheCodeFirstThenExtrasThenSafe(t *testing.T) {
	attrs := Failure("some_code", syscall.ECONNREFUSED, slog.String("dashboard_id", "d1"))
	var keys []string
	for _, a := range attrs {
		keys = append(keys, a.(slog.Attr).Key)
	}
	want := []string{"error_code", "dashboard_id", "error_type", "errno"}
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}
