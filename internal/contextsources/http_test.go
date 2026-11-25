package contextsources

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

const base = "/api/v1/context-sources"

var allActions = []audit.Action{actionCreate, actionUpdate, actionDelete, actionDiscover, actionApprove, actionRevoke}

type sourceView struct {
	UID         string  `json:"uid"`
	Description *string `json:"description"`
	State       string  `json:"state"`
	Failure     *string `json:"failure"`
	SecretSet   bool    `json:"secret_set"`
	Provisioned bool    `json:"provisioned"`
	Tools       struct {
		Offered, Approved, Changed, Gone, Refused int
	} `json:"tools"`
}

type toolView struct {
	Name           string  `json:"name"`
	State          string  `json:"state"`
	Refusal        *string `json:"refusal"`
	Description    string  `json:"description"`
	Hash           string  `json:"hash"`
	Seen           *struct{ Hash string }
	AllowedActions []string `json:"allowed_actions"`
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("answered %d, want %d: %s", rec.Code, status, rec.Body)
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return out
}

func (r *rig) tools(t *testing.T, uid string) map[string]toolView {
	t.Helper()
	list := decodeJSON[struct {
		Tools []toolView `json:"tools"`
	}](t, r.call(t, http.MethodGet, base+"/"+uid+"/tools", ""), http.StatusOK)
	out := map[string]toolView{}
	for _, tool := range list.Tools {
		out[tool.Name] = tool
	}
	return out
}

type discovery struct {
	Source sourceView `json:"source"`
	Tally  Tally      `json:"tally"`
}

func (r *rig) discover(t *testing.T, uid string) discovery {
	t.Helper()
	return decodeJSON[discovery](t, r.call(t, http.MethodPost, base+"/"+uid+"/discover", ""), http.StatusOK)
}

func destructiveTool(name string) mcp.Tool {
	return mcp.NewTool(name, mcp.WithDescription("Wipe it"), mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(true))
}

func unmarkedTool(name string) mcp.Tool {
	tool := mcp.NewTool(name, mcp.WithDescription("No hints"))
	tool.Annotations = mcp.ToolAnnotation{}
	return tool
}

func (r *rig) assertNoSecret(t *testing.T, secrets ...string) {
	t.Helper()
	r.flush()
	var haystack []string
	haystack = append(haystack, r.bodies...)
	haystack = append(haystack, r.logs.String())
	for _, action := range allActions {
		for _, e := range r.database.AuditEvents(t, string(action)) {
			raw, _ := json.Marshal(e)
			haystack = append(haystack, string(raw))
		}
	}
	for _, secret := range secrets {
		for _, text := range haystack {
			if strings.Contains(text, secret) {
				t.Fatalf("%q appears in a response, a log line or an audit event: %s", secret, text)
			}
		}
	}
}

func TestAnAdminRegistersDiscoversAndApprovesTools(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"), readOnlyTool("list", "List"), destructiveTool("wipe"), unmarkedTool("plain"))

	created := r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	source := decodeJSON[sourceView](t, created, http.StatusCreated)
	if source.State != StateUnchecked || !source.SecretSet {
		t.Fatalf("created = %+v", source)
	}
	allowed, err := r.store.Allowed(t.Context())
	if err != nil || !allowed.Allows(fakeHost) {
		t.Fatalf("the server's domain was not put on the outbound list: %v %v", allowed, err)
	}

	found := r.discover(t, "docs")
	if found.Source.State != StateReady || found.Tally.Found != 4 || found.Tally.New != 4 {
		t.Fatalf("discovery = %+v", found)
	}
	tools := r.tools(t, "docs")
	for name, want := range map[string]string{"search": "offered", "list": "offered", "wipe": "refused", "plain": "refused"} {
		if tools[name].State != want {
			t.Errorf("%s is %q, want %q", name, tools[name].State, want)
		}
	}
	if tools["wipe"].Refusal == nil || !strings.Contains(*tools["wipe"].Refusal, "destructive") {
		t.Errorf("wipe refusal = %v", tools["wipe"].Refusal)
	}
	if tools["plain"].Refusal == nil || !strings.Contains(*tools["plain"].Refusal, "read-only") {
		t.Errorf("plain refusal = %v", tools["plain"].Refusal)
	}

	one := r.call(t, http.MethodPost, base+"/docs/tools/search/approve", `{"hash":"`+tools["search"].Hash+`"}`)
	if got := decodeJSON[toolView](t, one, http.StatusOK); got.State != "approved" {
		t.Fatalf("search = %+v", got)
	}
	expectCode(t, r.call(t, http.MethodPost, base+"/docs/tools/wipe/approve", ""), http.StatusConflict, "tool_refused")
	expectCode(t, r.call(t, http.MethodPost, base+"/docs/tools/plain/approve", ""), http.StatusConflict, "tool_refused")

	all := decodeJSON[struct {
		Approved []toolView `json:"approved"`
	}](t, r.call(t, http.MethodPost, base+"/docs/tools/approve-read-only", ""), http.StatusOK)
	if len(all.Approved) != 1 || all.Approved[0].Name != "list" {
		t.Fatalf("approve-read-only = %+v", all.Approved)
	}
	counts := decodeJSON[sourceView](t, r.call(t, http.MethodGet, base+"/docs", ""), http.StatusOK)
	if counts.Tools.Approved != 2 || counts.Tools.Refused != 2 || counts.Tools.Offered != 0 {
		t.Errorf("counts = %+v", counts.Tools)
	}

	var stored []byte
	if err := r.database.SQLDB(t).QueryRowContext(t.Context(), `SELECT secret FROM context_sources WHERE uid = 'docs'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 || strings.Contains(string(stored), "s3cret-value") {
		t.Errorf("the secret must be stored sealed")
	}
	r.assertNoSecret(t, "s3cret-value")
}

func TestAToolTheServerChangesStopsUntilReapproved(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"), readOnlyTool("list", "List"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	r.discover(t, "docs")
	r.call(t, http.MethodPost, base+"/docs/tools/approve-read-only", "")
	before := r.tools(t, "docs")["search"]

	r.fake.setTools(readOnlyTool("search", "Search, and now it also does something else"), readOnlyTool("list", "List"))
	found := r.discover(t, "docs")
	if found.Tally.Changed != 1 || found.Tally.New != 0 {
		t.Fatalf("tally = %+v", found.Tally)
	}
	after := r.tools(t, "docs")
	changed := after["search"]
	if changed.State != "changed" || changed.Hash != before.Hash || changed.Seen == nil || changed.Seen.Hash == before.Hash {
		t.Fatalf("search = %+v", changed)
	}
	if !slices.Contains(changed.AllowedActions, "approve") {
		t.Errorf("a changed tool must offer approve: %v", changed.AllowedActions)
	}
	if after["list"].State != "approved" {
		t.Errorf("the unchanged tool must stay approved: %+v", after["list"])
	}

	expectCode(t, r.call(t, http.MethodPost, base+"/docs/tools/search/approve", `{"hash":"`+before.Hash+`"}`), http.StatusConflict, "tool_changed")
	if r.tools(t, "docs")["search"].State != "changed" {
		t.Fatal("approving the old definition must not restore the tool")
	}
	approved := decodeJSON[toolView](t, r.call(t, http.MethodPost, base+"/docs/tools/search/approve", `{"hash":"`+changed.Seen.Hash+`"}`), http.StatusOK)
	if approved.State != "approved" || approved.Hash != changed.Seen.Hash || !strings.Contains(approved.Description, "something else") {
		t.Fatalf("approved = %+v", approved)
	}

	r.fake.setTools(readOnlyTool("list", "List"))
	if gone := r.discover(t, "docs"); gone.Tally.Gone != 1 {
		t.Fatalf("tally = %+v", gone.Tally)
	}
	if got := r.tools(t, "docs")["search"].State; got != "gone" {
		t.Errorf("search is %q, want gone", got)
	}
	r.assertNoSecret(t, "s3cret-value")
}

func TestAServerNeverApprovesItsOwnNewTools(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	r.discover(t, "docs")
	r.call(t, http.MethodPost, base+"/docs/tools/approve-read-only", "")

	r.fake.setTools(readOnlyTool("search", "Search"), readOnlyTool("extra", "Added later"))
	r.discover(t, "docs")
	tools := r.tools(t, "docs")
	if tools["extra"].State != "offered" || tools["search"].State != "approved" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestADomainOffTheAllowlistIsRefusedAtRegistrationAndReportedAtDiscovery(t *testing.T) {
	other, err := domains.ParsePattern("other.example.com")
	if err != nil {
		t.Fatal(err)
	}
	locked := newRigWith(t, rigOptions{key: testKey, lock: allowlist.Lock{Locked: true, Domains: domains.List{other}}}, readOnlyTool("search", "Search"))
	expectCode(t, locked.call(t, http.MethodPost, base, registration("docs", "s3cret-value")), http.StatusForbidden, "outbound_domain_not_allowed")

	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	r.database.Exec(t, `DELETE FROM outbound_domains`)
	found := r.discover(t, "docs")
	if found.Source.State != StateUnreachable || found.Source.Failure == nil || *found.Source.Failure != FailureNotAllowed {
		t.Fatalf("source = %+v", found.Source)
	}
	if r.fake.requestCount() != 0 {
		t.Errorf("the server saw %d requests for a domain that is not allowed", r.fake.requestCount())
	}
}

func TestAWrongSecretIsReportedAndNeverEchoed(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "wrong-secret-xyz"))
	found := r.discover(t, "docs")
	if found.Source.State != StateUnreachable || found.Source.Failure == nil || *found.Source.Failure != FailureUnauthorized {
		t.Fatalf("source = %+v", found.Source)
	}

	fixed := r.call(t, http.MethodPatch, base+"/docs", `{"secret":"s3cret-value"}`)
	if got := decodeJSON[sourceView](t, fixed, http.StatusOK); got.State != StateUnchecked {
		t.Errorf("a new secret must reset the check: %+v", got)
	}
	if found := r.discover(t, "docs"); found.Source.State != StateReady || found.Tally.Found != 1 {
		t.Fatalf("after the fix: %+v", found)
	}
	r.assertNoSecret(t, "wrong-secret-xyz", "s3cret-value")
}

func TestChangingTheAddressDropsTheApprovedTools(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	r.discover(t, "docs")
	r.call(t, http.MethodPost, base+"/docs/tools/approve-read-only", "")

	expectCode(t, r.call(t, http.MethodPatch, base+"/docs", `{"url":"https://elsewhere.example.org/mcp"}`), http.StatusUnprocessableEntity, "invalid_context_source")
	moved := r.call(t, http.MethodPatch, base+"/docs", `{"url":"https://elsewhere.example.org/mcp","clear_secret":true}`)
	source := decodeJSON[sourceView](t, moved, http.StatusOK)
	if source.State != StateUnchecked || source.SecretSet || source.Tools.Approved != 0 {
		t.Fatalf("source = %+v", source)
	}
	if len(r.tools(t, "docs")) != 0 {
		t.Error("approvals must not carry over to another server")
	}
}

func TestRegistrationIsValidatedAndUIDsAreUnique(t *testing.T) {
	r := newRig(t)
	for _, body := range []string{
		`{"uid":"docs","url":"http://mcp.example.com/mcp"}`,
		`{"uid":"docs","url":"https://127.0.0.1/mcp"}`,
		`{"uid":"docs","url":"https://` + fakeHost + `/mcp?key=abc"}`,
		`{"uid":"docs","url":"https://` + fakeHost + `/mcp","secret":"x"}`,
		`{"uid":"docs","url":"https://` + fakeHost + `/mcp","audience":"members"}`,
		`{"uid":"bad uid","url":"https://` + fakeHost + `/mcp"}`,
	} {
		expectCode(t, r.call(t, http.MethodPost, base, body), http.StatusUnprocessableEntity, "invalid_context_source")
	}
	if rec := r.call(t, http.MethodPost, base, registration("docs", "s3cret-value")); rec.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", rec.Code, rec.Body)
	}
	expectCode(t, r.call(t, http.MethodPost, base, registration("docs", "s3cret-value")), http.StatusConflict, "uid_taken")
	expectCode(t, r.call(t, http.MethodGet, base+"/missing", ""), http.StatusNotFound, "not_found")
}

func TestASecretNeedsTheSecretKey(t *testing.T) {
	r := newRigWith(t, rigOptions{})
	expectCode(t, r.call(t, http.MethodPost, base, registration("docs", "s3cret-value")), http.StatusConflict, "secret_key_missing")
	body := `{"uid":"open","url":"https://` + fakeHost + `/mcp"}`
	if rec := r.call(t, http.MethodPost, base, body); rec.Code != http.StatusCreated {
		t.Fatalf("a source without a secret needs no key: %d %s", rec.Code, rec.Body)
	}
}

func TestOnlyAdminsReachTheRoutes(t *testing.T) {
	r := newRig(t)
	r.caller.Role = auth.RoleViewer
	req := httptest.NewRequest(http.MethodGet, base, nil)
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a viewer got %d: %s", rec.Code, rec.Body)
	}
}

func TestEveryChangeIsAuditedWithoutTheSecret(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	r.discover(t, "docs")
	r.call(t, http.MethodPost, base+"/docs/tools/search/approve", "")
	r.call(t, http.MethodPost, base+"/docs/tools/search/revoke", "")
	r.call(t, http.MethodPatch, base+"/docs", `{"name":"Renamed","secret":"another-secret-1"}`)
	r.call(t, http.MethodDelete, base+"/docs", "")
	r.flush()

	for _, action := range allActions {
		events := r.database.AuditEvents(t, string(action))
		if len(events) != 1 {
			t.Fatalf("%s: %d events, want 1", action, len(events))
		}
		e := events[0]
		if e.TargetKind != "context_source" || e.TargetID != "docs" || e.ActorKind != "user" || e.ActorID != r.admin.ID || e.Outcome != "success" {
			t.Errorf("%s = %+v", action, e)
		}
		if e.Data["host"] != fakeHost {
			t.Errorf("%s data = %v", action, e.Data)
		}
	}
	update := r.database.AuditEvents(t, string(actionUpdate))[0]
	if update.Data["credential_changed"] != true {
		t.Errorf("update data = %v", update.Data)
	}
	approve := r.database.AuditEvents(t, string(actionApprove))[0]
	if approve.Data["tool"] != "search" || approve.Data["hash"] == "" {
		t.Errorf("approve data = %v", approve.Data)
	}
	r.assertNoSecret(t, "s3cret-value", "another-secret-1")
}

func TestAFailedDiscoveryIsAuditedAsFailed(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	r.call(t, http.MethodPost, base, registration("docs", "wrong-secret-xyz"))
	r.discover(t, "docs")
	r.flush()
	events := r.database.AuditEvents(t, string(actionDiscover))
	if len(events) != 1 || events[0].Outcome != "failed" || events[0].Reason != FailureUnauthorized {
		t.Fatalf("events = %+v", events)
	}
}

func TestProvisionedSourcesAreManagedByTheFile(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	entry := config.ContextSource{UID: "filed", Name: "Filed", URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceAdmins, SecretHeader: "X-Api-Key", Secret: datasource.NewSecret("s3cret-value")} //nolint:gosec // a test value
	if err := r.service.Provision(t.Context(), []config.ContextSource{entry}); err != nil {
		t.Fatal(err)
	}
	r.allow(t, fakeHost)
	source := decodeJSON[sourceView](t, r.call(t, http.MethodGet, base+"/filed", ""), http.StatusOK)
	if !source.Provisioned || !source.SecretSet {
		t.Fatalf("source = %+v", source)
	}
	expectCode(t, r.call(t, http.MethodPatch, base+"/filed", `{"name":"Mine"}`), http.StatusConflict, "provisioned")
	expectCode(t, r.call(t, http.MethodDelete, base+"/filed", ""), http.StatusConflict, "provisioned")

	if found := r.discover(t, "filed"); found.Source.State != StateReady {
		t.Fatalf("discovery = %+v", found)
	}
	r.call(t, http.MethodPost, base+"/filed/tools/approve-read-only", "")

	entry.Name = "Renamed in the file"
	if err := r.service.Provision(t.Context(), []config.ContextSource{entry}); err != nil {
		t.Fatal(err)
	}
	if got := r.tools(t, "filed")["search"].State; got != "approved" {
		t.Errorf("a rename must keep approvals, search is %q", got)
	}

	if err := r.service.Provision(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	expectCode(t, r.call(t, http.MethodGet, base+"/filed", ""), http.StatusNotFound, "not_found")
	r.assertNoSecret(t, "s3cret-value")
}

func TestProvisioningRefusesAUIDAnAdminAlreadyUses(t *testing.T) {
	r := newRig(t)
	r.call(t, http.MethodPost, base, registration("docs", "s3cret-value"))
	err := r.service.Provision(t.Context(), []config.ContextSource{{UID: "docs", URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceEveryone}})
	if err == nil || !strings.Contains(err.Error(), ErrUIDTaken.Error()) {
		t.Fatalf("err = %v", err)
	}
}

func filedSources(prefix string, n int) []config.ContextSource {
	out := make([]config.ContextSource, n)
	for i := range out {
		out[i] = config.ContextSource{UID: fmt.Sprintf("%s%d", prefix, i), URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceEveryone}
	}
	return out
}

func TestProvisioningCountsRowsItDeletesAndRowsItAdds(t *testing.T) {
	r := newRig(t)
	if err := r.service.Provision(t.Context(), filedSources("old", MaxSources)); err != nil {
		t.Fatal(err)
	}
	if err := r.service.Provision(t.Context(), filedSources("new", MaxSources)); err != nil {
		t.Fatalf("replacing every filed source must fit under the cap: %v", err)
	}

	err := r.service.Provision(t.Context(), filedSources("more", MaxSources+1))
	if !errors.Is(err, ErrTooMany) {
		t.Fatalf("a file with more than %d sources: err = %v, want ErrTooMany", MaxSources, err)
	}
	listed, err := r.service.List(t.Context())
	if err != nil || len(listed) != MaxSources {
		t.Fatalf("a refused file must change nothing, got %d sources, err %v", len(listed), err)
	}
}

func TestConcurrentCreatesCannotPassTheCap(t *testing.T) {
	r := newRig(t)
	r.allow(t, fakeHost)
	if err := r.service.Provision(t.Context(), filedSources("filed", MaxSources-1)); err != nil {
		t.Fatal(err)
	}
	const attempts = 6
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = r.service.Create(t.Context(), r.admin, Input{UID: fmt.Sprintf("race%d", i), URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceEveryone})
		}()
	}
	wg.Wait()
	created, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrTooMany):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || refused != attempts-1 {
		t.Fatalf("created %d, refused %d; want 1 and %d", created, refused, attempts-1)
	}
	if listed, err := r.service.List(t.Context()); err != nil || len(listed) != MaxSources {
		t.Fatalf("sources = %d, err %v; want %d", len(listed), err, MaxSources)
	}
}

func TestASourceDescriptionIsOptionalAndEditable(t *testing.T) {
	r := newRig(t)
	plain := decodeJSON[sourceView](t, r.call(t, http.MethodPost, base, registration("docs", "s3cret-value")), http.StatusCreated)
	if plain.Description != nil {
		t.Fatalf("description = %q, want null", *plain.Description)
	}

	described := decodeJSON[sourceView](t, r.call(t, http.MethodPatch, base+"/docs", `{"description":"  Runbooks and design notes  "}`), http.StatusOK)
	if described.Description == nil || *described.Description != "Runbooks and design notes" {
		t.Fatalf("description = %v", described.Description)
	}
	renamed := decodeJSON[sourceView](t, r.call(t, http.MethodPatch, base+"/docs", `{"name":"Renamed"}`), http.StatusOK)
	if renamed.Description == nil || *renamed.Description != "Runbooks and design notes" {
		t.Fatalf("a change that leaves the description out must keep it, got %v", renamed.Description)
	}
	cleared := decodeJSON[sourceView](t, r.call(t, http.MethodPatch, base+"/docs", `{"description":""}`), http.StatusOK)
	if cleared.Description != nil {
		t.Fatalf("description = %q, want null after clearing", *cleared.Description)
	}

	expectCode(t, r.call(t, http.MethodPatch, base+"/docs", `{"description":"two\nlines"}`), http.StatusUnprocessableEntity, "invalid_context_source")
	expectCode(t, r.call(t, http.MethodPatch, base+"/docs", `{"description":"`+strings.Repeat("a", 301)+`"}`), http.StatusUnprocessableEntity, "invalid_context_source")

	withDescription := `{"uid":"wiki","url":"https://` + fakeHost + `/mcp","description":"Team wiki"}`
	wiki := decodeJSON[sourceView](t, r.call(t, http.MethodPost, base, withDescription), http.StatusCreated)
	if wiki.Description == nil || *wiki.Description != "Team wiki" {
		t.Fatalf("created description = %v", wiki.Description)
	}
}
