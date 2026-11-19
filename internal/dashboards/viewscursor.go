package dashboards

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type viewsCursor struct {
	Views     int32
	UpdatedAt time.Time
	ID        pgtype.UUID
}

type viewsCursorWire struct {
	Views     *int32  `json:"v"`
	UpdatedAt *int64  `json:"u"`
	ID        *string `json:"i"`
}

func encodeViewsCursor(views int, updatedAt time.Time, id string) string {
	count := httpapi.Int32(views)
	micros := updatedAt.UnixMicro()
	raw, _ := json.Marshal(viewsCursorWire{Views: &count, UpdatedAt: &micros, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeViewsCursor(cursor string) (viewsCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return viewsCursor{}, ErrInvalidCursor
	}
	var wire viewsCursorWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return viewsCursor{}, ErrInvalidCursor
	}
	if wire.Views == nil || *wire.Views < 0 || wire.UpdatedAt == nil || wire.ID == nil {
		return viewsCursor{}, ErrInvalidCursor
	}
	id, err := db.ParseUUID(*wire.ID)
	if err != nil {
		return viewsCursor{}, ErrInvalidCursor
	}
	return viewsCursor{Views: *wire.Views, UpdatedAt: time.UnixMicro(*wire.UpdatedAt).UTC(), ID: id}, nil
}
