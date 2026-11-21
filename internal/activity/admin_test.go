package activity

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func (r rig) appendAdmin(t *testing.T, by actor.Actor, typ string) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = Append(t.Context(), q, Entry{Type: typ, TargetID: newTarget(), Actor: by}, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

func TestAdminTypesAreKnownAndNotWorkspaceTypes(t *testing.T) {
	for _, typ := range AdminTypes() {
		if !Known(typ) {
			t.Errorf("%s is not a known type", typ)
		}
		if slices.Contains(workspaceTypes, typ) {
			t.Errorf("%s is listed as both administrative and workspace", typ)
		}
	}
	if IsAdminType(TypeDashboardCreated) || !IsAdminType(TypeUserRoleChanged) {
		t.Fatal("IsAdminType does not follow the administrative list")
	}
}

func TestPeopleAndServiceAccountDeleteEventsAreAdministrative(t *testing.T) {
	for _, typ := range []string{TypeUserInvited, TypeUserInviteUpdated, TypeUserInviteWithdrawn, TypeUserDeactivated, TypeUserReactivated, TypeUserRemoved, TypeServiceAccountDeleted} {
		if !IsAdminType(typ) {
			t.Errorf("%s is not an administrative type", typ)
		}
	}
}

func TestAServiceAccountDeleteRecordsOnlyItsName(t *testing.T) {
	r := newRig(t)
	account := dbsqlc.ServiceAccount{ID: newTarget(), Name: "ci", Role: "admin"}
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return AuthChanges{}.ServiceAccountDeleted(t.Context(), q, plain, account)
	})
	if err != nil {
		t.Fatal(err)
	}
	var typ, target, data string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT type, target_id::text, data::text FROM activity`).Scan(&typ, &target, &data); err != nil {
		t.Fatal(err)
	}
	if typ != TypeServiceAccountDeleted || target != account.ID.String() || data != `{"name": "ci"}` {
		t.Fatalf("recorded %s %s %s", typ, target, data)
	}
}

func TestAppendAdminRefusesAWorkspaceType(t *testing.T) {
	err := AppendAdmin(t.Context(), nil, editorActor, TypeDashboardCreated, newTarget(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestAdminEventsAreReadableByAdminsOnly(t *testing.T) {
	r := newRig(t)
	by := actor.Of(plain, origin.Origin{Kind: origin.Web}, "")
	changed := r.appendAdmin(t, by, TypeUserRoleChanged)
	workspace := r.append(t, Entry{Type: TypeFindingCreated, Actor: by})

	tests := []struct {
		name string
		who  auth.Principal
		want []int64
	}{
		{"an admin sees them", admin, []int64{changed, workspace}},
		{"the actor no longer sees its own admin event once it is not an Admin", plain, []int64{workspace}},
		{"another viewer does not", reader, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := r.store.List(t.Context(), r.audience(t, tt.who), Filter{}, "", 100)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(sorted(seqsOf(got)), sorted(tt.want)) {
				t.Fatalf("got %v, want %v", sorted(seqsOf(got)), sorted(tt.want))
			}
			follow, _, _, err := r.store.Next(t.Context(), r.audience(t, tt.who), Filter{}, At(0, 0), 100)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(sorted(seqsOf(follow)), sorted(tt.want)) {
				t.Fatalf("follow got %v, want %v", sorted(seqsOf(follow)), sorted(tt.want))
			}
		})
	}
}

func TestListNarrowsByActorAndByTime(t *testing.T) {
	r := newRig(t)
	other := actor.Of(reader, origin.Origin{}, "")
	first := r.appendAdmin(t, editorActor, TypeUserRoleChanged)
	r.db.Exec(t, `UPDATE activity SET created_at = now() - interval '2 hours' WHERE seq = $1`, first)
	second := r.appendAdmin(t, other, TypeUserRoleChanged)
	third := r.appendAdmin(t, editorActor, TypeWebhookDeleted)
	all := r.audience(t, admin)
	editorID := mustUUID(t, editorActor.ID)

	tests := []struct {
		name   string
		filter Filter
		want   []int64
	}{
		{"by actor", Filter{ActorID: editorID}, []int64{third, first}},
		{"since", Filter{Since: time.Now().Add(-time.Hour)}, []int64{third, second}},
		{"until", Filter{Until: time.Now().Add(-time.Hour)}, []int64{first}},
		{"actor and type", Filter{ActorID: editorID, Types: []string{TypeWebhookDeleted}}, []int64{third}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := r.store.List(t.Context(), all, tt.filter, "", 100)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(seqsOf(got), tt.want) {
				t.Fatalf("got %v, want %v", seqsOf(got), tt.want)
			}
		})
	}
}

func TestParseFilterReadsTheActorAndUntilFilters(t *testing.T) {
	f, err := ParseFilter(url.Values{
		"actor_id": {"11111111-1111-4111-8111-111111111111"},
		"until":    {"2026-11-02T10:00:00Z"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.ActorID.String() != "11111111-1111-4111-8111-111111111111" || !f.Until.Equal(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %+v", f)
	}
}

func TestParseFilterRefusesBadActorAndUntilFilters(t *testing.T) {
	for name, q := range map[string]url.Values{
		"actor":         {"actor_id": {"nope"}},
		"until":         {"until": {"tomorrow"}},
		"until no zone": {"until": {"2026-11-02T10:00:00"}},
	} {
		if _, err := ParseFilter(q); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
