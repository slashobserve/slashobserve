package audit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestOnlyAUniqueViolationCountsAsAlreadyStored(t *testing.T) {
	if !alreadyStored(fmt.Errorf("copy: %w", &pgconn.PgError{Code: "23505"})) {
		t.Error("a unique violation was not read as already stored")
	}
	for _, err := range []error{&pgconn.PgError{Code: "23514"}, context.DeadlineExceeded, errors.New("unexpected")} {
		if alreadyStored(err) {
			t.Errorf("alreadyStored(%v) = true", err)
		}
	}
}

func TestWhichWriteFailuresAreRetried(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{context.DeadlineExceeded, true},
		{fmt.Errorf("copy: %w", context.DeadlineExceeded), true},
		{&pgconn.PgError{Code: "55P03"}, true},
		{&pgconn.PgError{Code: "08006"}, true},
		{&pgconn.PgError{Code: "57P01"}, true},
		{&pgconn.PgError{Code: "53300"}, true},
		{&pgconn.PgError{Code: "23505"}, false},
		{&pgconn.PgError{Code: "23514"}, false},
		{errors.New("unexpected"), false},
	}
	for _, c := range cases {
		if got := retryable(c.err); got != c.want {
			t.Errorf("retryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
