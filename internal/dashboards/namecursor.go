package dashboards

import (
	"encoding/base64"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

type nameCursor struct {
	Title string
	ID    pgtype.UUID
}

type nameCursorWire struct {
	Title *string `json:"n"`
	ID    *string `json:"i"`
}

func encodeNameCursor(title, id string) string {
	raw, _ := json.Marshal(nameCursorWire{Title: &title, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeNameCursor(cursor string) (nameCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nameCursor{}, ErrInvalidCursor
	}
	var wire nameCursorWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nameCursor{}, ErrInvalidCursor
	}
	if wire.Title == nil || wire.ID == nil {
		return nameCursor{}, ErrInvalidCursor
	}
	id, err := db.ParseUUID(*wire.ID)
	if err != nil {
		return nameCursor{}, ErrInvalidCursor
	}
	return nameCursor{Title: *wire.Title, ID: id}, nil
}
