package contextsources

import (
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func filedCode(approve ...string) []config.ContextSource {
	return []config.ContextSource{{UID: "code", Name: "Code", URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceEveryone, AllowPrivate: true, Approve: approve}}
}

func TestListedApprovalsPicksOnlyOfferedToolsAndSaysWhyItSkippedTheRest(t *testing.T) {
	stored := []Tool{
		{Name: "offered", State: ToolOffered},
		{Name: "seen", State: ToolOffered},
		{Name: "approved", State: ToolApproved},
		{Name: "changed", State: ToolChanged},
		{Name: "gone", State: ToolGone},
		{Name: "wipe", State: ToolRefused, Refusal: refusalDestructive},
	}
	approve, skipped := listedApprovals(stored, []string{"offered", "seen", "approved", "changed", "gone", "wipe", "ghost"}, map[string]bool{"seen": true, "approved": true})
	if len(approve) != 1 || approve[0].Name != "offered" {
		t.Fatalf("approve = %+v", approve)
	}
	want := map[string]string{"seen": skipKnown, "changed": skipChanged, "gone": skipNotOffered, "wipe": refusalDestructive, "ghost": skipNotOffered}
	if len(skipped) != len(want) {
		t.Fatalf("skipped = %+v", skipped)
	}
	for _, s := range skipped {
		if want[s.Name] != s.Reason {
			t.Errorf("%s skipped for %q, want %q", s.Name, s.Reason, want[s.Name])
		}
	}
}

func TestTheFileApprovesTheToolsItListsAndRefusesTheRest(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"), readOnlyTool("list", "List"), destructiveTool("wipe"))
	entries := filedCode("search", "wipe", "ghost")
	if err := r.service.Provision(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	r.allow(t, fakeHost)
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	tools := r.tools(t, "code")
	for name, want := range map[string]string{"search": "approved", "list": "offered", "wipe": "refused"} {
		if tools[name].State != want {
			t.Errorf("%s is %q, want %q", name, tools[name].State, want)
		}
	}
	logs := r.logs.String()
	for _, want := range []string{"tool=ghost", "tool=wipe", skipNotOffered, refusalDestructive} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q: %s", want, logs)
		}
	}

	r.flush()
	events := r.database.AuditEvents(t, string(actionApprove))
	if len(events) != 1 || events[0].ActorKind != string(audit.ActorSystem) || events[0].Data["tool"] != "search" || events[0].Data["provisioned"] != true || events[0].Data["private"] != true {
		t.Fatalf("events = %+v", events)
	}

	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	r.flush()
	if again := r.database.AuditEvents(t, string(actionApprove)); len(again) != 1 {
		t.Errorf("a second start approved again: %+v", again)
	}
}

func TestTheFileNeverApprovesAChangedDefinition(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	entries := filedCode("search")
	if err := r.service.Provision(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	r.allow(t, fakeHost)
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	r.fake.setTools(readOnlyTool("search", "Search, and now it does something else"))
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if got := r.tools(t, "code")["search"].State; got != "changed" {
		t.Fatalf("search is %q after its definition changed, want changed", got)
	}
	if !strings.Contains(r.logs.String(), skipChanged) {
		t.Errorf("the log does not say the change needs a review: %s", r.logs.String())
	}
	r.flush()
	if events := r.database.AuditEvents(t, string(actionApprove)); len(events) != 1 {
		t.Errorf("approve events = %+v", events)
	}
	if got := decodeJSON[toolView](t, r.call(t, http.MethodPost, base+"/code/tools/search/approve", ""), http.StatusOK); got.State != "approved" {
		t.Errorf("an Admin could not approve the changed tool: %+v", got)
	}
}

func TestASourceThatDoesNotAnswerDoesNotStopTheStart(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	entries := filedCode("search")
	if err := r.service.Provision(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if got := r.tools(t, "code"); len(got) != 0 {
		t.Errorf("tools = %+v", got)
	}
	if !strings.Contains(r.logs.String(), "discovery failed") {
		t.Errorf("the log does not say discovery failed: %s", r.logs.String())
	}
}

func TestARevokedToolIsNotApprovedAgainAtTheNextStart(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"), readOnlyTool("list", "List"))
	entries := filedCode("search", "list")
	if err := r.service.Provision(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	r.allow(t, fakeHost)
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if got := decodeJSON[toolView](t, r.call(t, http.MethodPost, base+"/code/tools/search/revoke", ""), http.StatusOK); got.State != "offered" {
		t.Fatalf("revoke left %+v", got)
	}
	if err := r.service.ApproveListed(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	tools := r.tools(t, "code")
	if tools["search"].State != "offered" || tools["list"].State != "approved" {
		t.Fatalf("after a second start search is %q and list is %q, want offered and approved", tools["search"].State, tools["list"].State)
	}
	if !strings.Contains(r.logs.String(), skipKnown) {
		t.Errorf("the log does not say why search was left: %s", r.logs.String())
	}
}

func TestOneFailingSourceDoesNotStopTheOthers(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search"))
	entries := append(filedCode("search"), config.ContextSource{UID: "gone", Name: "Gone", URL: "https://" + fakeHost + "/mcp", Audience: datasource.AudienceEveryone, AllowPrivate: true, Approve: []string{"search"}})
	if err := r.service.Provision(t.Context(), entries[:1]); err != nil {
		t.Fatal(err)
	}
	r.allow(t, fakeHost)
	err := r.service.ApproveListed(t.Context(), entries)
	if err == nil || !strings.Contains(err.Error(), `"gone"`) {
		t.Fatalf("err = %v, want it to name the source that is not registered", err)
	}
	if got := r.tools(t, "code")["search"].State; got != "approved" {
		t.Errorf("search is %q, want approved although another source failed", got)
	}
}
