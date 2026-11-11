package search

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("cursor is not valid")

type cursor struct {
	AsOfMillis int64 `json:"a"`
	Offset     int   `json:"o"`
}

func encodeCursor(asOf time.Time, offset int) string {
	raw, err := json.Marshal(cursor{AsOfMillis: asOf.UnixMilli(), Offset: offset})
	if err != nil {
		panic("search: cursor does not marshal")
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (asOf time.Time, offset int, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, 0, ErrInvalidCursor
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return time.Time{}, 0, ErrInvalidCursor
	}
	if c.AsOfMillis <= 0 || c.Offset < 0 || c.Offset >= MaxResults {
		return time.Time{}, 0, ErrInvalidCursor
	}
	return time.UnixMilli(c.AsOfMillis).UTC(), c.Offset, nil
}
