package revisions

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var shapedObservationID = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

func shapedAs(row dbsqlc.AgentRequest, source Source) dbsqlc.AgentRequest {
	switch source {
	case SourceSuggest:
		row.Intent = string(IntentSuggest)
	case SourceObserve:
		row.Intent = string(IntentAsk)
		if row.DashboardID.Valid {
			row.Intent = string(IntentApply)
		}
	case SourcePrompt:
		row.Intent, row.BaseVersion = string(IntentApply), pgtype.Int4{Int32: 1, Valid: true}
	case SourceComment:
		row.Intent, row.BaseVersion, row.CommentID = string(IntentApply), pgtype.Int4{Int32: 1, Valid: true}, shapedObservationID
	case SourceInvestigate:
		row.Intent = string(IntentAsk)
		if !row.DashboardID.Valid {
			row.ObservationID = shapedObservationID
		}
	}
	return row
}

func TestSourceOfDerivesTheRunKindFromIntentAndShape(t *testing.T) {
	dashboard := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	base := pgtype.Int4{Int32: 3, Valid: true}
	tests := []struct {
		name string
		row  dbsqlc.AgentRequest
		want Source
	}{
		{"a suggestion", dbsqlc.AgentRequest{Intent: "suggest", DashboardID: dashboard}, SourceSuggest},
		{"an ask with no subject builds or answers", dbsqlc.AgentRequest{Intent: "ask"}, SourceObserve},
		{"an ask on a dashboard investigates", dbsqlc.AgentRequest{Intent: "ask", DashboardID: dashboard}, SourceInvestigate},
		{"an ask into an observation investigates", dbsqlc.AgentRequest{Intent: "ask", ObservationID: shapedObservationID}, SourceInvestigate},
		{"an apply from a chat revises", dbsqlc.AgentRequest{Intent: "apply", DashboardID: dashboard, BaseVersion: base}, SourcePrompt},
		{"an apply from a legacy comment revises from a comment", dbsqlc.AgentRequest{Intent: "apply", DashboardID: dashboard, BaseVersion: base, CommentID: dashboard}, SourceComment},
		{"an apply from a thread revises from a comment", dbsqlc.AgentRequest{Intent: "apply", DashboardID: dashboard, BaseVersion: base, ThreadMessageID: dashboard}, SourceComment},
		{"an apply with no base is a build that saved a new dashboard", dbsqlc.AgentRequest{Intent: "apply", DashboardID: dashboard}, SourceObserve},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SourceOf(tc.row); got != tc.want {
				t.Fatalf("SourceOf = %q, want %q", got, tc.want)
			}
		})
	}
	for _, source := range []Source{SourceSuggest, SourceObserve, SourcePrompt, SourceComment, SourceInvestigate} {
		for _, row := range []dbsqlc.AgentRequest{{}, {DashboardID: dashboard}} {
			if source == SourceSuggest && !row.DashboardID.Valid {
				continue
			}
			if got := SourceOf(shapedAs(row, source)); got != source {
				t.Errorf("shapedAs(%q) derives %q", source, got)
			}
		}
	}
}
