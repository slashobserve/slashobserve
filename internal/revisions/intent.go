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

type runShape struct {
	intent      Intent
	revises     bool
	fromThread  bool
	dashboard   bool
	observation bool
}

func (r runShape) source() Source {
	switch {
	case r.intent == IntentSuggest:
		return SourceSuggest
	case r.intent == IntentApply && !r.revises:
		return SourceObserve
	case r.intent == IntentApply && r.fromThread:
		return SourceComment
	case r.intent == IntentApply:
		return SourcePrompt
	case r.dashboard || r.observation:
		return SourceInvestigate
	}
	return SourceObserve
}

func shapeOf(intent string, baseVersion pgtype.Int4, commentID, threadMessageID, dashboardID, observationID pgtype.UUID) runShape {
	return runShape{
		intent:      Intent(intent),
		revises:     baseVersion.Valid,
		fromThread:  commentID.Valid || threadMessageID.Valid,
		dashboard:   dashboardID.Valid,
		observation: observationID.Valid,
	}
}

func rowShape(row dbsqlc.AgentRequest) runShape {
	return shapeOf(row.Intent, row.BaseVersion, row.CommentID, row.ThreadMessageID, row.DashboardID, row.ObservationID)
}

func SourceOf(row dbsqlc.AgentRequest) Source {
	return rowShape(row).source()
}

func listedSource(row dbsqlc.ListAgentRequestsByRow) Source {
	shape := shapeOf(row.Intent, row.BaseVersion, row.CommentID, row.ThreadMessageID, row.DashboardID, row.ObservationID)
	shape.observation = shape.observation || row.ChatObservationID.Valid
	return shape.source()
}

func SourceFor(intent string, baseVersion pgtype.Int4, commentID, threadMessageID, dashboardID, observationID pgtype.UUID) Source {
	return shapeOf(intent, baseVersion, commentID, threadMessageID, dashboardID, observationID).source()
}
