package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type teamEvent struct {
	Action, Outcome, Reason, ActorKind, ActorID, ActorName, ActorRole string
	TargetKind, TargetID, TargetName                                  string
	Data                                                              map[string]any
}

func (r *rig) teamEvents() []teamEvent {
	r.t.Helper()
	rows, err := r.db.SQLDB(r.t).QueryContext(r.t.Context(), `SELECT action, outcome, coalesce(reason, ''), actor_kind, coalesce(actor_id::text, ''), coalesce(actor_name, ''), coalesce(actor_role, ''),
  coalesce(target_kind, ''), coalesce(target_id, ''), coalesce(target_name, ''), data::text
FROM audit_events WHERE action LIKE 'team.%' ORDER BY occurred_at, id`)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events []teamEvent
	for rows.Next() {
		var e teamEvent
		var data string
		if err := rows.Scan(&e.Action, &e.Outcome, &e.Reason, &e.ActorKind, &e.ActorID, &e.ActorName, &e.ActorRole, &e.TargetKind, &e.TargetID, &e.TargetName, &data); err != nil {
			r.t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			r.t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
	return events
}

func dataKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestTeamChangesRecordOneEventEachWithTheActorAndTheTeamEvenWhenNothingChanged(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	admin := adminCaller(ada, "Ada")
	sre := r.createTeam("sre", admin)
	r.do(http.MethodPut, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	r.do(http.MethodPut, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID+"/members/"+bob.String(), admin)
	if rec := r.do(http.MethodDelete, "/api/v1/teams/"+sre.ID, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete team = %d %s", rec.Code, rec.Body)
	}

	want := []struct {
		action, reason string
		keys           []string
	}{
		{"team.create", "", []string{}},
		{"team.member_add", "", []string{"user_id", "user_name"}},
		{"team.member_add", "no_change", []string{"user_id", "user_name"}},
		{"team.member_remove", "", []string{"user_id", "user_name"}},
		{"team.member_remove", "no_change", []string{"user_id", "user_name"}},
		{"team.delete", "", []string{}},
	}
	got := r.teamEvents()
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %d: a repeated add and a repeated remove record one event each with the reason no_change", got, len(want))
	}
	for i, w := range want {
		e := got[i]
		if e.Reason != w.reason {
			t.Errorf("event %d reason = %q, want %q", i, e.Reason, w.reason)
		}
		if e.Action != w.action || e.Outcome != "success" || e.ActorKind != "user" || e.ActorID != ada.String() || e.ActorName != "Ada" || e.ActorRole != "admin" {
			t.Errorf("event %d = %+v, want a successful %s by Ada", i, e, w.action)
		}
		if e.TargetKind != "team" || e.TargetID != sre.ID || e.TargetName != "sre" {
			t.Errorf("event %d target = %s %s %q, want team %s sre", i, e.TargetKind, e.TargetID, e.TargetName, sre.ID)
		}
		if !slices.Equal(dataKeys(e.Data), w.keys) {
			t.Errorf("event %d data keys = %v, want %v", i, dataKeys(e.Data), w.keys)
		}
	}
	for _, i := range []int{1, 2, 3, 4} {
		if got[i].Data["user_id"] != bob.String() || got[i].Data["user_name"] != "bob" {
			t.Errorf("event %d data = %v, want bob", i, got[i].Data)
		}
	}
}

func TestATeamMemberChangeAndItsEventCommitAndRollBackTogether(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	team := r.createTeam("sre", adminCaller(ada, "Ada"))
	teamID := pgtype.UUID{}
	if err := teamID.Scan(team.ID); err != nil {
		t.Fatal(err)
	}
	errRollback := errors.New("roll back")
	before := len(r.teamEvents())

	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		if _, err := q.AddTeamMember(t.Context(), dbsqlc.AddTeamMemberParams{TeamID: teamID, UserID: bob}); err != nil {
			return err
		}
		if err := recordTeamMember(t.Context(), q, audit.Action("team.member_add"), teamID, bob, ""); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("rolled back change: %v", err)
	}
	if after := len(r.teamEvents()); after != before {
		t.Fatalf("team events = %d, want the %d from before the rolled back change", after, before)
	}
	var members int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM team_members`).Scan(&members); err != nil || members != 0 {
		t.Fatalf("members = %d, %v; want 0", members, err)
	}
}

func TestAMemberWithoutANameIsRecordedByIdAndNeverByEmail(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	nameless, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: "nameless", Email: "nameless@example.com", Name: ""})
	if err != nil {
		t.Fatal(err)
	}
	admin := adminCaller(ada, "Ada")
	sre := r.createTeam("sre", admin)
	r.do(http.MethodPut, "/api/v1/teams/"+sre.ID+"/members/"+nameless.ID.String(), admin)

	got := r.teamEvents()
	add := got[len(got)-1]
	if add.Action != "team.member_add" || add.Data["user_id"] != nameless.ID.String() || add.Data["user_name"] != "" {
		t.Fatalf("event = %+v, want the id and an empty name", add)
	}
}

func TestRemovingAMemberOfATeamThatDoesNotExistIsNotFoundAndRecordsNothing(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now())
	bob := r.addUser("bob", time.Now())
	before := len(r.teamEvents())

	rec := r.do(http.MethodDelete, "/api/v1/teams/"+uuid.NewString()+"/members/"+bob.String(), adminCaller(ada, "Ada"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove = %d %s, want 404", rec.Code, rec.Body)
	}
	if after := len(r.teamEvents()); after != before {
		t.Fatalf("team events = %d, want %d", after, before)
	}
}
