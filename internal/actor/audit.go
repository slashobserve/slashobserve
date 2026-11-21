package actor

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
)

func (c Columns) AuditBy() *audit.By {
	return &audit.By{
		Actor:  audit.Actor{Kind: audit.ActorKind(c.Kind), ID: nullUUID(c.ID), Name: c.Name},
		Origin: audit.Origin{Kind: c.Origin.String, Client: c.Client.String},
		RunID:  nullUUID(c.RunID),
	}
}

func nullUUID(id pgtype.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id.Bytes, Valid: id.Valid}
}
