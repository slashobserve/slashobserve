package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const foreignKeyViolationCode = "23503"

func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode
}

func ViolatedForeignKey(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return pgErr.ConstraintName
	}
	return ""
}
