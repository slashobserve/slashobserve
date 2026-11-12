package observations

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type Observation struct {
	ID             string
	FolderID       string
	Title          string
	State          State
	Summary        string
	From, To       *time.Time
	ThreadID       string
	RunID          string
	Actor          actor.Actor
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ResolvedAt     *time.Time
	Folder         folders.Folder
	Abilities      folders.Abilities
	AllowedActions []Action
	Blocks         []Block
}

type Block struct {
	ID             string
	ObservationID  string
	Kind           Kind
	State          BlockState
	Position       int
	Width          Width
	Version        int
	Body           json.RawMessage
	SnapshotID     string
	RunID          string
	Actor          actor.Actor
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AllowedActions []BlockAction
}

type Snapshot struct {
	ID            string
	ObservationID string
	Panel         json.RawMessage
	Query         json.RawMessage
	Data          json.RawMessage
	UserScoped    bool
	From, To      time.Time
	Actor         actor.Actor
	CreatedAt     time.Time
}

type Placement struct {
	Folder    pgtype.UUID
	Personal  bool
	Abilities folders.Abilities
	Activity  activity.Owner
}

type CreateInput struct {
	FolderID   pgtype.UUID
	Title      string
	From, To   *time.Time
	FirstBlock *BlockInput
	RunID      pgtype.UUID
}

type UpdateInput struct {
	Title      *string
	From, To   *time.Time
	ClearRange bool
	RunID      pgtype.UUID
}

type ListFilter struct {
	FolderID pgtype.UUID
	State    State
}

type BlockInput struct {
	Kind        Kind
	Body        json.RawMessage
	Width       Width
	Position    *int
	FromBlockID pgtype.UUID
	RunID       pgtype.UUID
}

type BlockChange struct {
	Version  int
	Body     json.RawMessage
	Width    *Width
	Position *int
	RunID    pgtype.UUID
}
