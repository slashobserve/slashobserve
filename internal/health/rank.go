package health

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	lowestRank        = 3
	maxCursorMicros   = 253402300799999999
	maxCursorTitleLen = 1000
)

type rankCursor struct {
	Rank  int
	Seen  time.Time
	Title string
	ID    pgtype.UUID
}

type rankCursorWire struct {
	R *int    `json:"r"`
	S *int64  `json:"s"`
	T *string `json:"t"`
	I *string `json:"i"`
}

func encodeRankCursor(rank int, seen time.Time, title, id string) string {
	micros := seen.UnixMicro()
	raw, _ := json.Marshal(rankCursorWire{R: &rank, S: &micros, T: &title, I: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeRankCursor(raw string) (*rankCursor, bool) {
	if raw == "" {
		return nil, true
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(decoded))
	dec.DisallowUnknownFields()
	var wire rankCursorWire
	if err := dec.Decode(&wire); err != nil || dec.More() {
		return nil, false
	}
	if wire.R == nil || wire.S == nil || wire.T == nil || wire.I == nil {
		return nil, false
	}
	if *wire.R < 0 || *wire.R > lowestRank || *wire.S < 0 || *wire.S > maxCursorMicros || len(*wire.T) > maxCursorTitleLen {
		return nil, false
	}
	id, err := db.ParseUUID(*wire.I)
	if err != nil {
		return nil, false
	}
	return &rankCursor{Rank: *wire.R, Seen: time.UnixMicro(*wire.S).UTC(), Title: *wire.T, ID: id}, true
}
