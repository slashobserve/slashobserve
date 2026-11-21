package revisions

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Intent string

const (
	IntentAsk     Intent = "ask"
	IntentApply   Intent = "apply"
	IntentSuggest Intent = "suggest"
)

type RunShape struct {
	Intent          Intent
	BaseVersion     pgtype.Int4
	CommentID       pgtype.UUID
	ThreadMessageID pgtype.UUID
	DashboardID     pgtype.UUID
	ObservationID   pgtype.UUID
	SubjectID       pgtype.UUID
}

func (r RunShape) Source() Source {
	switch {
	case r.Intent == IntentSuggest:
		return SourceSuggest
	case r.Intent == IntentApply && !r.BaseVersion.Valid:
		return SourceObserve
	case r.Intent == IntentApply && (r.CommentID.Valid || r.ThreadMessageID.Valid):
		return SourceComment
	case r.Intent == IntentApply:
		return SourcePrompt
	case r.DashboardID.Valid || r.ObservationID.Valid || r.SubjectID.Valid:
		return SourceInvestigate
	}
	return SourceObserve
}

func shapeOfRow(row dbsqlc.AgentRequest) RunShape {
	return RunShape{
		Intent:          Intent(row.Intent),
		BaseVersion:     row.BaseVersion,
		CommentID:       row.CommentID,
		ThreadMessageID: row.ThreadMessageID,
		DashboardID:     row.DashboardID,
		ObservationID:   row.ObservationID,
		SubjectID:       row.SubjectObservationID,
	}
}

func SourceOf(row dbsqlc.AgentRequest) Source {
	return shapeOfRow(row).Source()
}

func listedSource(row dbsqlc.ListAgentRequestsByRow) Source {
	return listedShape(row).Source()
}

func listedShape(row dbsqlc.ListAgentRequestsByRow) RunShape {
	return RunShape{
		Intent:          Intent(row.Intent),
		BaseVersion:     row.BaseVersion,
		CommentID:       row.CommentID,
		ThreadMessageID: row.ThreadMessageID,
		DashboardID:     row.DashboardID,
		ObservationID:   row.ObservationID,
		SubjectID:       row.SubjectObservationID,
	}
}

func privateAsk(row dbsqlc.AgentRequest) bool {
	return Intent(row.Intent) == IntentAsk && row.ChatID.Valid && !row.ThreadMessageID.Valid && !row.ObservationID.Valid
}
