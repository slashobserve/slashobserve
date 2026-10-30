package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsForeignKeyViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"foreign key violation", &pgconn.PgError{Code: "23503"}, true},
		{"wrapped foreign key violation", fmt.Errorf("x: %w", &pgconn.PgError{Code: "23503"}), true},
		{"unique violation", &pgconn.PgError{Code: "23505"}, false},
		{"plain error", errors.New("x"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsForeignKeyViolation(tt.err); got != tt.want {
				t.Fatalf("IsForeignKeyViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}
