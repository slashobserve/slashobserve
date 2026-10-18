package logattrs

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

var sqlStatePattern = regexp.MustCompile(`^[0-9A-Z]{5}$`)

// Safe describes err without its text, which can echo queries, tool results or credentials:
// the innermost error's Go type and, for a Postgres error, its SQLSTATE.
func Safe(err error) []any {
	attrs := []any{slog.String("error_type", innermostType(err))}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && sqlStatePattern.MatchString(pgErr.Code) {
		attrs = append(attrs, slog.String("sqlstate", pgErr.Code))
	}
	return attrs
}

func innermostType(err error) string {
	for {
		if next := errors.Unwrap(err); next != nil {
			err = next
			continue
		}
		var joined interface{ Unwrap() []error }
		if errors.As(err, &joined) {
			if wrapped := joined.Unwrap(); len(wrapped) > 0 {
				err = wrapped[len(wrapped)-1]
				continue
			}
		}
		return fmt.Sprintf("%T", err)
	}
}
