package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

const canonicalUUIDLength = 36

var errInvalidUUID = errors.New("not a canonical UUID")

// ParseUUID is stricter than pgtype.UUID.Scan, which skips the characters at the dash positions
// and accepts 32 digits without dashes.
func ParseUUID(s string) (pgtype.UUID, error) {
	if len(s) != canonicalUUIDLength {
		return pgtype.UUID{}, errInvalidUUID
	}
	for i := 0; i < len(s); i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return pgtype.UUID{}, errInvalidUUID
			}
		default:
			if !isHexDigit(s[i]) {
				return pgtype.UUID{}, errInvalidUUID
			}
		}
	}
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func isHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
