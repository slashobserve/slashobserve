package revisions

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestViewOfHidesTheWordsOfAPrivateChatFromEveryoneButItsRequesterAndAdmins(t *testing.T) {
	asker := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Role: auth.RoleEditor}
	editor := auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Role: auth.RoleEditor}
	admin := auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Role: auth.RoleAdmin}
	some := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	row := func(intent string, chat, observation, message pgtype.UUID, investigation bool) dbsqlc.AgentRequest {
		var requester pgtype.UUID
		if err := requester.Scan(asker.ID); err != nil {
			t.Fatal(err)
		}
		return dbsqlc.AgentRequest{
			Intent: intent, ChatID: chat, ObservationID: observation, ThreadMessageID: message, Investigation: investigation,
			DashboardID: some, RequestedByKind: string(auth.KindUser), RequestedByID: requester,
		}
	}
	none := pgtype.UUID{}
	tests := []struct {
		name        string
		row         dbsqlc.AgentRequest
		readsThread bool
		want        [3]runView
	}{
		{"a dashboard change from a chat", row("apply", some, none, none, false), false, [3]runView{viewFull, viewRedacted, viewFull}},
		{"a saved private ask", row("ask", some, some, none, false), false, [3]runView{viewFull, viewRedacted, viewFull}},
		{"an unsaved private ask", row("ask", some, none, none, false), false, [3]runView{viewFull, viewNone, viewFull}},
		{"a private ask shared into a thread the caller reads", row("ask", some, none, some, false), true, [3]runView{viewFull, viewFull, viewFull}},
		{"a private ask shared into a thread the caller cannot read", row("ask", some, none, some, false), false, [3]runView{viewFull, viewNone, viewFull}},
		{"a saved private ask shared into a thread the caller cannot read", row("ask", some, some, some, false), false, [3]runView{viewFull, viewRedacted, viewFull}},
		{"an investigation from a chat", row("ask", some, some, none, true), false, [3]runView{viewFull, viewFull, viewFull}},
		{"a run outside any chat", row("apply", none, none, none, false), false, [3]runView{viewFull, viewFull, viewFull}},
	}
	for _, tc := range tests {
		for i, caller := range []auth.Principal{asker, editor, admin} {
			if got := audienceOf(tc.row).viewFor(caller, func() bool { return tc.readsThread }); got != tc.want[i] {
				t.Errorf("%s, caller %d: view %d, want %d", tc.name, i, got, tc.want[i])
			}
		}
	}
}

func TestUsageKeepsTheHypothesisLedgerOnlyForItsRequesterAndAdmins(t *testing.T) {
	asker := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Role: auth.RoleEditor}
	other := auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Role: auth.RoleEditor}
	admin := auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Role: auth.RoleAdmin}
	var requester pgtype.UUID
	if err := requester.Scan(asker.ID); err != nil {
		t.Fatal(err)
	}
	row := dbsqlc.AgentRequest{RequestedByKind: string(auth.KindUser), RequestedByID: requester}
	stored := &Usage{ToolCalls: 4, Hypotheses: []Hypothesis{{ID: "pool", Text: "The pool is exhausted", Status: HypothesisSupport}}}
	for _, c := range []struct {
		name   string
		caller auth.Principal
		want   int
	}{
		{"the requester", asker, 1},
		{"an admin", admin, 1},
		{"another editor", other, 0},
	} {
		got := usageShownTo(stored, row, c.caller)
		if len(got.Hypotheses) != c.want || got.ToolCalls != 4 {
			t.Errorf("%s: usage = %+v", c.name, got)
		}
	}
	if len(stored.Hypotheses) != 1 {
		t.Error("hiding the ledger changed the stored usage")
	}
	if usageShownTo(nil, row, other) != nil {
		t.Error("no usage became some")
	}
}
