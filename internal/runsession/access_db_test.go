package runsession

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) folder(adminsOnly bool) pgtype.UUID {
	r.t.Helper()
	var id pgtype.UUID
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO folders (title, admins_only) VALUES ('Audit', $1) RETURNING id`, adminsOnly).Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r *rig) askInThread(req revisions.Request) {
	r.t.Helper()
	var thread pgtype.UUID
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1, 1) RETURNING id`, r.dash).Scan(&thread); err != nil {
		r.t.Fatal(err)
	}
	r.askIn(req, thread)
}

func (r *rig) askInAnnotationThread(req revisions.Request) {
	r.t.Helper()
	var annotation, thread pgtype.UUID
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO annotations (dashboard_id, time_from, text, created_by) VALUES ($1, now(), 'deploy', $2) RETURNING id`, r.dash, mustUUID(r.t, userID)).Scan(&annotation); err != nil {
		r.t.Fatal(err)
	}
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO threads (annotation_id) VALUES ($1) RETURNING id`, annotation).Scan(&thread); err != nil {
		r.t.Fatal(err)
	}
	r.askIn(req, thread)
}

func (r *rig) askIn(req revisions.Request, thread pgtype.UUID) {
	r.t.Helper()
	var message pgtype.UUID
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name) VALUES ($1, 'q', 'user', $2, 'Ada') RETURNING id`,
		thread, mustUUID(r.t, userID)).Scan(&message); err != nil {
		r.t.Fatal(err)
	}
	r.d.Exec(r.t, "UPDATE agent_requests SET thread_message_id = $2 WHERE id = $1", mustUUID(r.t, req.ID), message)
}

func (r *rig) fileInto(req revisions.Request, folder pgtype.UUID) {
	r.t.Helper()
	var observation pgtype.UUID
	if err := r.d.SQLDB(r.t).QueryRow(`INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES ($1, 'Checkout latency', 'user', $2, 'Ada') RETURNING id`,
		folder, mustUUID(r.t, userID)).Scan(&observation); err != nil {
		r.t.Fatal(err)
	}
	r.d.Exec(r.t, "UPDATE agent_requests SET observation_id = $2 WHERE id = $1", mustUUID(r.t, req.ID), observation)
}

func (r *rig) moveDashboard(adminsOnly bool) pgtype.UUID {
	r.t.Helper()
	folder := r.folder(adminsOnly)
	r.d.Exec(r.t, "UPDATE dashboards SET folder_id = $2 WHERE id = $1", r.dash, folder)
	return folder
}

func asRole(req revisions.Request, role auth.Role) revisions.Request {
	req.RequestedBy.Role = role
	return req
}

func TestTheScopeOfARunReadsAsItsRequesterForTheReadersOfWhatItWrites(t *testing.T) {
	inThread := func(r *rig) revisions.Request { req := r.revise(); r.askInThread(req); return req }
	onAnnotation := func(r *rig) revisions.Request { req := r.investigate(); r.askInAnnotationThread(req); return req }
	cases := []struct {
		name       string
		start      func(r *rig) revisions.Request
		files      bool
		adminsOnly bool
		role       auth.Role
		want       datasource.Audience
		reads      bool
	}{
		{"an Admin's revision of a dashboard in a shared folder", (*rig).revise, false, false, auth.RoleAdmin, datasource.AudienceEveryone, false},
		{"an Admin's revision of a dashboard only Admins read", (*rig).revise, false, true, auth.RoleAdmin, datasource.AudienceAdmins, true},
		{"an Editor's revision of a dashboard only Admins read", (*rig).revise, false, true, auth.RoleEditor, datasource.AudienceAdmins, false},
		{"an Admin's investigation with no observation yet", (*rig).investigate, false, false, auth.RoleAdmin, datasource.AudienceCaller, true},
		{"an Admin's investigation into an observation in a shared folder", (*rig).investigate, true, false, auth.RoleAdmin, datasource.AudienceEveryone, false},
		{"an Admin's investigation into an observation only Admins read", (*rig).investigate, true, true, auth.RoleAdmin, datasource.AudienceAdmins, true},
		{"an Editor's investigation into an observation only Admins read", (*rig).investigate, true, true, auth.RoleEditor, datasource.AudienceAdmins, false},
		{"an Admin's suggestion on a dashboard in a shared folder", (*rig).suggest, false, false, auth.RoleAdmin, datasource.AudienceEveryone, false},
		{"an Admin's suggestion on a dashboard only Admins read", (*rig).suggest, false, true, auth.RoleAdmin, datasource.AudienceAdmins, true},
		{"an Admin's thread apply on a dashboard in a shared folder", inThread, false, false, auth.RoleAdmin, datasource.AudienceEveryone, false},
		{"an Admin's thread apply on a dashboard only Admins read", inThread, false, true, auth.RoleAdmin, datasource.AudienceAdmins, true},
		{"an Admin's ask in a thread on an annotation of a dashboard in a shared folder", onAnnotation, false, false, auth.RoleAdmin, datasource.AudienceEveryone, false},
		{"an Admin's ask in a thread on an annotation of a dashboard only Admins read", onAnnotation, false, true, auth.RoleAdmin, datasource.AudienceAdmins, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			req := asRole(tc.start(r), tc.role)
			folder := r.moveDashboard(tc.adminsOnly)
			if tc.files {
				r.fileInto(req, folder)
			}
			run, err := r.svc.Scope(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if run.Access.System || run.Access.By.ID != req.RequestedBy.ID || run.Access.By.Role != tc.role || run.Access.Audience != tc.want {
				t.Fatalf("run access = %+v, want the requester %+v writing for %q", run.Access, req.RequestedBy, tc.want)
			}
			if tc.want != datasource.AudienceCaller && !slices.Equal(run.WriteFolders, []pgtype.UUID{folder}) {
				t.Fatalf("the run writes to the folders %v, want only %v", run.WriteFolders, folder)
			}
			if datasource.ReadsRestricted(datasource.WithAccess(context.Background(), run.Access)) != tc.reads {
				t.Fatalf("reads Admins-only data = %v, want %v", !tc.reads, tc.reads)
			}
		})
	}
}

func TestARunThatAsksPrivatelyKeepsTheRequestersAccess(t *testing.T) {
	r := newRig(t)
	req := asRole(r.investigate(), auth.RoleAdmin)
	r.moveDashboard(false)
	run, err := r.svc.Scope(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.Access.Audience != datasource.AudienceCaller || !datasource.ReadsRestricted(datasource.WithAccess(context.Background(), run.Access)) {
		t.Fatalf("run access = %+v, want the Admin requester's own access", run.Access)
	}
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "audit", Audience: datasource.AudienceAdmins, Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.For(datasource.WithAccess(context.Background(), run.Access), "audit"); err != nil {
		t.Fatalf("an Admin's private ask querying the audit datasource: %v", err)
	}
}

func TestTheBriefOfAnAdminsRunWritingForEveryoneSaysWhereAdminsOnlyDataCanBeRead(t *testing.T) {
	for _, adminsOnly := range []bool{false, true} {
		r := newRig(t)
		req := asRole(r.revise(), auth.RoleAdmin)
		r.moveDashboard(adminsOnly)
		brief, err := r.svc.Brief(context.Background(), req, "")
		if err != nil {
			t.Fatal(err)
		}
		if brief.Brief.Withheld == adminsOnly {
			t.Fatalf("Admins only folder %v: brief carries the withheld note %v", adminsOnly, !adminsOnly)
		}
	}
	r := newRig(t)
	req := r.revise()
	brief, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if brief.Brief.Withheld {
		t.Fatal("an Editor's brief mentions Admin-only data")
	}
}

func TestACallThatReachedAnAdminsOnlyDatasourceMarksTheRun(t *testing.T) {
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "audit", Audience: datasource.AudienceAdmins, Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		role   auth.Role
		uid    string
		marked bool
	}{
		{"an Admin reading it", auth.RoleAdmin, "audit", true},
		{"an Editor refused it", auth.RoleEditor, "audit", false},
		{"an Admin reading nothing restricted", auth.RoleAdmin, "missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			req := asRole(r.revise(), tc.role)
			r.call(req, "list_metrics", `{}`, nil, func(ctx context.Context) (tools.Result, error) {
				_, _ = registry.For(datasource.WithAccess(ctx, datasource.Access{By: req.RequestedBy}), tc.uid)
				return tools.Result{Content: map[string]any{"ok": true}}, nil
			})
			if got := r.stored(req).ReadRestricted; got != tc.marked {
				t.Fatalf("read_restricted = %v, want %v", got, tc.marked)
			}
		})
	}
}

type fakeDatasource struct{ datasource.Datasource }

func (fakeDatasource) Type() string { return "prometheus" }
