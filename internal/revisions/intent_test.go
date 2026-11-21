package revisions

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var shapedObservationID = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

func shapedAs(row dbsqlc.AgentRequest, source Source) dbsqlc.AgentRequest {
	row.Kind = KindBuild
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
		row.Intent, row.Kind = string(IntentAsk), KindInvestigate
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
		{"a suggestion", dbsqlc.AgentRequest{Intent: "suggest", Kind: "build", DashboardID: dashboard}, SourceSuggest},
		{"a build ask builds or answers", dbsqlc.AgentRequest{Intent: "ask", Kind: "build"}, SourceObserve},
		{"an ask on a dashboard investigates", dbsqlc.AgentRequest{Intent: "ask", Kind: "investigate", DashboardID: dashboard}, SourceInvestigate},
		{"an ask into an observation investigates", dbsqlc.AgentRequest{Intent: "ask", Kind: "investigate", ObservationID: shapedObservationID}, SourceInvestigate},
		{"an investigate ask that lost its observation still investigates", dbsqlc.AgentRequest{Intent: "ask", Kind: "investigate"}, SourceInvestigate},
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

func TestInvestigatesWithoutObservationOnlyForAnInvestigationThatLostItsOwn(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	investigation := dbsqlc.AgentRequest{Intent: string(IntentAsk), Kind: KindInvestigate}
	with := func(edit func(*dbsqlc.AgentRequest)) dbsqlc.AgentRequest {
		row := investigation
		edit(&row)
		return row
	}
	tests := []struct {
		name string
		row  dbsqlc.AgentRequest
		want bool
	}{
		{"a Home investigation in a chat", with(func(r *dbsqlc.AgentRequest) { r.ChatID = id }), true},
		{"a dashboard investigation outside a chat", with(func(r *dbsqlc.AgentRequest) { r.DashboardID = id }), true},
		{"an investigation with its observation", with(func(r *dbsqlc.AgentRequest) { r.ChatID, r.ObservationID = id, id }), false},
		{"a private ask about a dashboard", with(func(r *dbsqlc.AgentRequest) { r.ChatID, r.DashboardID = id, id }), false},
		{"a private ask about an observation", with(func(r *dbsqlc.AgentRequest) { r.ChatID, r.SubjectObservationID = id, id }), false},
		{"a thread ask, answered in its thread", with(func(r *dbsqlc.AgentRequest) { r.ThreadMessageID = id }), false},
		{"a build", dbsqlc.AgentRequest{Intent: string(IntentAsk), Kind: KindBuild}, false},
	}
	for _, tc := range tests {
		if got := InvestigatesWithoutObservation(tc.row); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
