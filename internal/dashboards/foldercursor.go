package dashboards

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

type folderCursor struct {
	General   bool
	Title     string
	FolderID  pgtype.UUID
	UpdatedAt time.Time
	ID        pgtype.UUID
}

type folderCursorWire struct {
	General   *bool   `json:"g"`
	Title     *string `json:"t"`
	FolderID  *string `json:"f"`
	UpdatedAt *int64  `json:"u"`
	ID        *string `json:"i"`
}

func encodeFolderCursor(general bool, title string, folderID pgtype.UUID, updatedAt time.Time, id string) string {
	micros := updatedAt.UnixMicro()
	folder := folderID.String()
	raw, _ := json.Marshal(folderCursorWire{General: &general, Title: &title, FolderID: &folder, UpdatedAt: &micros, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeFolderCursor(cursor string) (folderCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return folderCursor{}, ErrInvalidCursor
	}
	var wire folderCursorWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return folderCursor{}, ErrInvalidCursor
	}
	if wire.General == nil || wire.Title == nil || wire.FolderID == nil || wire.UpdatedAt == nil || wire.ID == nil {
		return folderCursor{}, ErrInvalidCursor
	}
	folderID, err := db.ParseUUID(*wire.FolderID)
	if err != nil {
		return folderCursor{}, ErrInvalidCursor
	}
	id, err := db.ParseUUID(*wire.ID)
	if err != nil {
		return folderCursor{}, ErrInvalidCursor
	}
	return folderCursor{General: *wire.General, Title: *wire.Title, FolderID: folderID, UpdatedAt: time.UnixMicro(*wire.UpdatedAt).UTC(), ID: id}, nil
}
