package datasourceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func readsAs(t *testing.T, r *rig, p auth.Principal, uid string) error {
	t.Helper()
	ctx := datasource.WithAccess(context.Background(), datasource.Access{By: p})
	_, err := r.registry.For(ctx, uid)
	_, missing := r.registry.For(ctx, "missing")
	if err != nil && (!errors.Is(err, datasource.ErrNotVisible) || err.Error() != missing.Error()) {
		t.Fatalf("refusal %q differs from the one for a missing datasource %q", err, missing)
	}
	listed := slices.ContainsFunc(r.registry.Visible(ctx), func(in datasource.Instance) bool { return in.UID == uid })
	if listed != (err == nil) {
		t.Fatalf("listed %v but For err = %v", listed, err)
	}
	return err
}

func roleOf(r *rig, role auth.Role) auth.Principal {
	p := r.admin
	p.Role = role
	return p
}

func TestAnAppDatasourceForAdminsIsHiddenFromEveryOtherRoleUntilOpenedAgain(t *testing.T) {
	server := upstream(t, new(string))
	r, svc := newRig(t, testKey)
	def := prometheus("audit", server.URL)
	def.Audience = datasource.AudienceAdmins
	created, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Definition.Audience != datasource.AudienceAdmins {
		t.Fatalf("created audience = %q", created.Definition.Audience)
	}
	lower := []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor}
	for _, role := range lower {
		if err := readsAs(t, r, roleOf(r, role), "audit"); err == nil {
			t.Fatalf("%s reads an Admins-only datasource", role)
		}
	}
	if err := readsAs(t, r, r.admin, "audit"); err != nil {
		t.Fatalf("admin: %v", err)
	}

	if err := svc.Reconcile(reading(t.Context())); err != nil {
		t.Fatal(err)
	}
	if err := readsAs(t, r, roleOf(r, auth.RoleViewer), "audit"); err == nil {
		t.Fatal("a reload opened the datasource to a viewer")
	}

	opened := def
	opened.Audience = datasource.AudienceEveryone
	if _, err := svc.Update(reading(t.Context()), r.admin, "audit", opened, SecretsChange{}); err != nil {
		t.Fatal(err)
	}
	for _, role := range lower {
		if err := readsAs(t, r, roleOf(r, role), "audit"); err != nil {
			t.Fatalf("%s after opening: %v", role, err)
		}
	}
}

func TestADatasourceAddedWithoutAnAudienceIsForEveryone(t *testing.T) {
	r, svc := newRig(t, testKey)
	created, err := svc.Create(reading(t.Context()), r.admin, prometheus("metrics", "https://prom.example.com"), Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Definition.Audience != datasource.AudienceEveryone {
		t.Fatalf("audience = %q", created.Definition.Audience)
	}
	if err := readsAs(t, r, roleOf(r, auth.RoleViewer), "metrics"); err != nil {
		t.Fatal(err)
	}
}

func TestAnAudienceThatIsNeitherEveryoneNorAdminsIsRefusedByName(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := prometheus("metrics", "https://prom.example.com")
	def.Audience = "editors"
	_, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{})
	var invalidErr *InvalidError
	if !errors.As(err, &invalidErr) || len(invalidErr.Fields) != 1 || invalidErr.Fields[0].Field != "audience" {
		t.Fatalf("err = %v", err)
	}
}

type auditedEvent struct {
	action, targetKind, targetID, actorID string
	data                                  map[string]any
}

func (r *rig) audited(t *testing.T) []auditedEvent {
	t.Helper()
	rows, err := r.database.SQLDB(t).QueryContext(reading(t.Context()), `SELECT action, target_kind, target_id, actor_id::text, data::text FROM audit_events WHERE action LIKE 'datasource.%' ORDER BY activity_seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []auditedEvent
	for rows.Next() {
		var e auditedEvent
		var raw string
		if err := rows.Scan(&e.action, &e.targetKind, &e.targetID, &e.actorID, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &e.data); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAnAudienceChangeIsRecordedOnceAsItsOwnAuditAction(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := prometheus("metrics", "https://prom.example.com")
	created, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	def.Audience = datasource.AudienceAdmins
	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", def, SecretsChange{}); err != nil {
		t.Fatal(err)
	}

	events := r.recorded(t)
	if len(events) != 2 || events[1].Type != "datasource.audience_changed" {
		t.Fatalf("activity = %+v", events)
	}
	if got := events[1].Data; got["from"] != "everyone" || got["to"] != "admins" || got["uid"] != "metrics" || got["name"] != created.Definition.Name {
		t.Fatalf("audience change data = %v", got)
	}
	audited := r.audited(t)
	if len(audited) != 2 || audited[0].action != "datasource.create" {
		t.Fatalf("audit events = %+v", audited)
	}
	change := audited[1]
	if change.action != "datasource.audience_change" || change.targetKind != "datasource" || change.actorID != r.admin.ID || change.data["from"] != "everyone" || change.data["to"] != "admins" {
		t.Fatalf("audience change audit event = %+v", change)
	}
	if change.targetID == "" || change.targetID != audited[0].targetID {
		t.Fatalf("audience change target %q, create target %q", change.targetID, audited[0].targetID)
	}
}

func TestAChangeOfTheAudienceAndTheNameRecordsEachOnceWithTheAudienceOutOfTheUpdate(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := prometheus("metrics", "https://prom.example.com")
	if _, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{}); err != nil {
		t.Fatal(err)
	}
	def.Audience, def.Name = datasource.AudienceAdmins, "Metrics"
	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", def, SecretsChange{}); err != nil {
		t.Fatal(err)
	}

	audited := r.audited(t)
	var actions []string
	for _, e := range audited {
		actions = append(actions, e.action)
	}
	if !slices.Equal(actions, []string{"datasource.create", "datasource.audience_change", "datasource.update"}) {
		t.Fatalf("audit actions = %v", actions)
	}
	changes, _ := audited[2].data["changes"].(map[string]any)
	if _, ok := changes["audience"]; ok || changes["name"] == nil || len(changes) != 1 {
		t.Fatalf("update changes = %v", changes)
	}
	if _, ok := audited[2].data["settings_changed"]; ok {
		t.Fatalf("an audience change also counted as a settings change: %v", audited[2].data)
	}
}

func audienceOver(t *testing.T, h *httpRig, uid string) datasource.Audience {
	t.Helper()
	caller := h.caller
	h.caller = h.admin
	defer func() { h.caller = caller }()
	rec := h.call(t, http.MethodGet, "/api/v1/datasources/"+uid+"/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("read answered %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Audience datasource.Audience `json:"audience"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Audience
}

func TestOnlyAnAdminSetsReadsAndChangesTheAudienceOverHTTP(t *testing.T) {
	h := newHTTPRig(t)
	rec := h.call(t, http.MethodPost, "/api/v1/datasources", `{"uid":"audit","type":"prometheus","address":"http://audit:9090","audience":"admins"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", rec.Code, rec.Body)
	}
	if got := audienceOver(t, h, "audit"); got != datasource.AudienceAdmins {
		t.Fatalf("audience = %q", got)
	}
	if rec := h.call(t, http.MethodPatch, "/api/v1/datasources/audit", `{"name":"Audit"}`); rec.Code != http.StatusOK {
		t.Fatalf("rename answered %d: %s", rec.Code, rec.Body)
	}
	if got := audienceOver(t, h, "audit"); got != datasource.AudienceAdmins {
		t.Fatalf("a change that left the audience out set it to %q", got)
	}
	expectCode(t, h.call(t, http.MethodPatch, "/api/v1/datasources/audit", `{"audience":"editors"}`), http.StatusUnprocessableEntity, "invalid_datasource")

	for _, role := range []auth.Role{auth.RoleEditor, auth.RoleCommenter, auth.RoleViewer} {
		h.caller = roleOf(h.rig, role)
		if rec := h.call(t, http.MethodPatch, "/api/v1/datasources/audit", `{"audience":"everyone"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("%s changing the audience answered %d", role, rec.Code)
		}
		if rec := h.call(t, http.MethodGet, "/api/v1/datasources/audit/config", ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s reading the audience answered %d", role, rec.Code)
		}
	}
	if got := audienceOver(t, h, "audit"); got != datasource.AudienceAdmins {
		t.Fatalf("audience after lower roles tried = %q", got)
	}
}

func TestAnEmptyAudienceInAChangeIsRefusedAndKeepsTheStoredOne(t *testing.T) {
	h := newHTTPRig(t)
	if rec := h.call(t, http.MethodPost, "/api/v1/datasources", `{"uid":"audit","type":"prometheus","address":"http://audit:9090","audience":"admins"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", rec.Code, rec.Body)
	}
	rec := h.call(t, http.MethodPatch, "/api/v1/datasources/audit", `{"audience":""}`)
	expectCode(t, rec, http.StatusUnprocessableEntity, "invalid_datasource")
	var body struct {
		Fields []struct {
			Field string `json:"field"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Fields) != 1 || body.Fields[0].Field != "audience" {
		t.Fatalf("refusal = %s", rec.Body)
	}
	if got := audienceOver(t, h, "audit"); got != datasource.AudienceAdmins {
		t.Fatalf("audience after an empty change = %q", got)
	}
}
