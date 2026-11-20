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

func TestChangingTheAudienceIsRecordedWithBeforeAndAfter(t *testing.T) {
	r, svc := newRig(t, testKey)
	def := prometheus("metrics", "https://prom.example.com")
	if _, err := svc.Create(reading(t.Context()), r.admin, def, Secrets{}); err != nil {
		t.Fatal(err)
	}
	def.Audience = datasource.AudienceAdmins
	if _, err := svc.Update(reading(t.Context()), r.admin, "metrics", def, SecretsChange{}); err != nil {
		t.Fatal(err)
	}

	events := r.recorded(t)
	if len(events) != 2 || events[1].Type != "datasource.updated" {
		t.Fatalf("events = %+v", events)
	}
	updated := events[1].Data
	changes, _ := updated["changes"].(map[string]any)
	audience, _ := changes["audience"].(map[string]any)
	if audience["from"] != "everyone" || audience["to"] != "admins" || len(changes) != 1 {
		t.Fatalf("changes = %v", changes)
	}
	if _, ok := updated["settings_changed"]; ok {
		t.Fatalf("an audience change also counted as a settings change: %v", updated)
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
