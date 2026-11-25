package contextsources

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type recordedCall struct {
	target Target
	name   string
	args   map[string]any
}

type scriptedConnector struct {
	mu     sync.Mutex
	calls  []recordedCall
	answer func(ctx context.Context, target Target, name string, args map[string]any) (Called, error)
}

func (c *scriptedConnector) Tools(context.Context, Target) ([]Found, error) { return nil, nil }

func (c *scriptedConnector) Call(ctx context.Context, target Target, name string, args map[string]any) (Called, error) {
	c.mu.Lock()
	c.calls = append(c.calls, recordedCall{target: target, name: name, args: args})
	answer := c.answer
	c.mu.Unlock()
	if answer == nil {
		return Called{Text: "found it"}, nil
	}
	return answer(ctx, target, name, args)
}

func (c *scriptedConnector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

const searchSchema = `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1},"token":{"type":"string","writeOnly":true}},"required":["query"],"additionalProperties":false}`

func approvedSearch(t *testing.T, state ToolState) Tool {
	t.Helper()
	return storedFrom(t, state, Found{Name: "search", Description: "Search the code\nsecond line", InputSchema: json.RawMessage(searchSchema), ReadOnly: yes()})
}

type callRig struct {
	service   *Service
	connector *scriptedConnector
	row       dbsqlc.ContextSource
	attempt   *callAttempt
}

func newCallRig(t *testing.T, limits Limits) *callRig {
	t.Helper()
	connector := &scriptedConnector{}
	service := New(Deps{Key: testKey, Connector: connector, Limits: limits, Logger: slog.New(slog.DiscardHandler)})
	row := dbsqlc.ContextSource{ID: newID(), Uid: "code", Name: "Code", Url: "https://" + fakeHost + "/mcp", Audience: string(datasource.AudienceEveryone)}
	return &callRig{service: service, connector: connector, row: row, attempt: &callAttempt{uid: row.Uid, tool: "search", row: row, known: true}}
}

func (r *callRig) withSecret(t *testing.T, header, secret string) {
	t.Helper()
	sealed, err := r.service.seal(r.row.ID, secret)
	if err != nil {
		t.Fatal(err)
	}
	r.row.Secret = sealed
	r.row.SecretHeader = text(header)
	r.attempt.row = r.row
}

func (r *callRig) dispatch(ctx context.Context, access datasource.Access, arguments string, stored ...Tool) (tools.ContextResult, error) {
	return r.service.dispatch(ctx, access, r.attempt, stored, json.RawMessage(arguments))
}

func personAccess(role auth.Role) datasource.Access {
	return datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "u", Role: role}}
}

func failureOf(t *testing.T, err error) *tools.ContextFailure {
	t.Helper()
	var failure *tools.ContextFailure
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, want a context failure", err)
	}
	return failure
}

func TestAnApprovedToolIsCalledWithTheSourcesAddressAndSecret(t *testing.T) {
	r := newCallRig(t, Limits{})
	r.withSecret(t, "X-Api-Key", "s3cret-value")
	r.connector.answer = func(context.Context, Target, string, map[string]any) (Called, error) {
		return Called{Text: "main.go:12"}, nil
	}
	got, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"checkout","limit":3}`, approvedSearch(t, ToolApproved))
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "main.go:12" || got.Cut || got.IsError {
		t.Fatalf("result = %+v", got)
	}
	call := r.connector.calls[0]
	if call.target.URL != r.row.Url || call.target.Header != "X-Api-Key" || call.target.Secret.Reveal() != "s3cret-value" || call.name != "search" {
		t.Errorf("call = %+v", call)
	}
	if call.args["query"] != "checkout" || call.args["limit"].(json.Number).String() != "3" {
		t.Errorf("arguments = %v", call.args)
	}
}

func TestOnlyAnApprovedUnchangedToolIsCalled(t *testing.T) {
	cases := []struct {
		state ToolState
		want  string
	}{
		{ToolOffered, "has not approved"},
		{ToolChanged, "approves it again"},
		{ToolGone, "no longer lists"},
		{ToolRefused, "it is refused: "},
	}
	for _, c := range cases {
		t.Run(string(c.state), func(t *testing.T) {
			r := newCallRig(t, Limits{})
			tool := approvedSearch(t, c.state)
			tool.Refusal = "The server marks this tool as destructive."
			_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), `{"query":"x"}`, tool)
			failure := failureOf(t, err)
			if failure.Code != tools.ContextCodeToolNotApproved || !strings.Contains(failure.Message, c.want) {
				t.Fatalf("failure = %+v", failure)
			}
			if r.connector.count() != 0 {
				t.Fatal("the source was called for a tool that is not approved")
			}
		})
	}
}

func TestAnUnknownToolNamesTheApprovedOnes(t *testing.T) {
	r := newCallRig(t, Limits{})
	r.attempt.tool = "wipe"
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), `{}`, approvedSearch(t, ToolApproved))
	failure := failureOf(t, err)
	if failure.Code != tools.ContextCodeToolNotApproved || !strings.Contains(failure.Message, "approved tools are search") {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestArgumentsAreCheckedAgainstThePinnedSchema(t *testing.T) {
	cases := map[string]string{
		"missing required": `{}`,
		"wrong type":       `{"query":7}`,
		"below minimum":    `{"query":"x","limit":0}`,
		"extra field":      `{"query":"x","other":1}`,
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			r := newCallRig(t, Limits{})
			_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), arguments, approvedSearch(t, ToolApproved))
			failure := failureOf(t, err)
			if failure.Code != tools.ContextCodeInvalidArguments || !strings.Contains(failure.Message, "input schema") {
				t.Fatalf("failure = %+v", failure)
			}
			if r.connector.count() != 0 {
				t.Fatal("the source was called with arguments that break its schema")
			}
		})
	}
}

func TestAMissingArgumentIsNamedInTheRefusal(t *testing.T) {
	r := newCallRig(t, Limits{})
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), `{"limit":2}`, approvedSearch(t, ToolApproved))
	if message := failureOf(t, err).Message; !strings.Contains(message, "query") {
		t.Fatalf("message = %q", message)
	}
}

func TestAnOversizedOrNonObjectArgumentIsRefused(t *testing.T) {
	r := newCallRig(t, Limits{})
	big := `{"query":"` + strings.Repeat("a", MaxArgumentBytes) + `"}`
	for name, arguments := range map[string]string{"too big": big, "array": `[1]`, "text": `"x"`} {
		_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), arguments, approvedSearch(t, ToolApproved))
		if failure := failureOf(t, err); failure.Code != tools.ContextCodeInvalidArguments {
			t.Errorf("%s: failure = %+v", name, failure)
		}
	}
}

func TestOmittedArgumentsAreAnEmptyObject(t *testing.T) {
	r := newCallRig(t, Limits{})
	open := storedFrom(t, ToolApproved, Found{Name: "search", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: yes()})
	for _, arguments := range []string{``, `null`} {
		if _, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), arguments, open); err != nil {
			t.Fatalf("%q: %v", arguments, err)
		}
	}
	if len(r.connector.calls[0].args) != 0 {
		t.Errorf("args = %v", r.connector.calls[0].args)
	}
}

func TestASchemaThatLoadsOtherDocumentsIsNeverFollowed(t *testing.T) {
	r := newCallRig(t, Limits{})
	remote := storedFrom(t, ToolApproved, Found{Name: "search", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"$ref":"https://169.254.169.254/schema.json"}}}`), ReadOnly: yes()})
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleAdmin), `{"query":"x"}`, remote)
	if failure := failureOf(t, err); failure.Code != tools.ContextCodeSchemaUnusable {
		t.Fatalf("failure = %+v", failure)
	}
	if r.connector.count() != 0 {
		t.Fatal("the source was called although its schema cannot be checked")
	}
}

func TestAnAdminsOnlySourceIsHiddenFromEveryoneElse(t *testing.T) {
	r := newCallRig(t, Limits{})
	r.row.Audience = string(datasource.AudienceAdmins)
	r.attempt.row = r.row
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleEditor} {
		_, err := r.dispatch(t.Context(), personAccess(role), `{"query":"x"}`, approvedSearch(t, ToolApproved))
		if failure := failureOf(t, err); failure.Code != tools.ContextCodeSourceNotFound {
			t.Errorf("%s: failure = %+v", role, failure)
		}
	}
	if r.connector.count() != 0 {
		t.Fatal("the source was called for someone who may not use it")
	}
}

func TestAnAdminsRunThatEveryoneReadsIsWithheldFromAnAdminsOnlySource(t *testing.T) {
	r := newCallRig(t, Limits{})
	r.row.Audience = string(datasource.AudienceAdmins)
	r.attempt.row = r.row
	shared := datasource.Access{By: auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}, Audience: datasource.AudienceEveryone}
	_, err := r.dispatch(t.Context(), shared, `{"query":"x"}`, approvedSearch(t, ToolApproved))
	if !errors.Is(err, datasource.ErrWithheld) {
		t.Fatalf("err = %v, want withheld", err)
	}
	private := datasource.Access{By: shared.By}
	ctx, restricted := datasource.NotingRestrictedReads(datasource.WithAccess(t.Context(), private))
	if _, err := r.dispatch(ctx, private, `{"query":"x"}`, approvedSearch(t, ToolApproved)); err != nil {
		t.Fatal(err)
	}
	if !restricted() {
		t.Error("a call to an Admins-only source must mark the run as having read restricted data")
	}
}

func TestALongResultIsCutAtTheLimitOnACharacterBoundary(t *testing.T) {
	r := newCallRig(t, Limits{ResultBytes: 100})
	long := strings.Repeat("é", 200)
	r.connector.answer = func(context.Context, Target, string, map[string]any) (Called, error) {
		return Called{Text: long}, nil
	}
	got, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Cut || got.Bytes != len(long) || len(got.Text) > 100 || len(got.Text) < 98 || strings.ContainsRune(got.Text, '�') {
		t.Fatalf("cut = %v bytes = %d text = %d bytes", got.Cut, got.Bytes, len(got.Text))
	}
}

func TestASourceThatAnswersTooSlowlyIsReportedAsTimedOut(t *testing.T) {
	r := newCallRig(t, Limits{Timeout: 20 * time.Millisecond})
	r.connector.answer = func(ctx context.Context, _ Target, _ string, _ map[string]any) (Called, error) {
		<-ctx.Done()
		return Called{}, classify(ctx, ctx.Err())
	}
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
	failure := failureOf(t, err)
	if failure.Code != tools.ContextCodeSourceDown || failure.Reason != FailureTimeout || !strings.Contains(failure.Message, "go on without it") {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestACancelledRunIsNotReportedAsADownSource(t *testing.T) {
	r := newCallRig(t, Limits{})
	ctx, cancel := context.WithCancel(t.Context())
	r.connector.answer = func(ctx context.Context, _ Target, _ string, _ map[string]any) (Called, error) {
		cancel()
		<-ctx.Done()
		return Called{}, classify(ctx, ctx.Err())
	}
	_, err := r.dispatch(ctx, personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestADownSourceGivesAPlainRefusalAndNeverTheSecret(t *testing.T) {
	for reason, want := range map[string]string{
		FailureUnreachable:  "could not be reached",
		FailureUnauthorized: "refused the stored secret",
		FailureNotAllowed:   "outbound domain list",
	} {
		r := newCallRig(t, Limits{})
		r.withSecret(t, "X-Api-Key", "s3cret-value")
		r.connector.answer = func(context.Context, Target, string, map[string]any) (Called, error) {
			return Called{}, &Failure{Code: reason}
		}
		_, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
		failure := failureOf(t, err)
		if failure.Code != tools.ContextCodeSourceDown || failure.Reason != reason || !strings.Contains(failure.Message, want) || strings.Contains(failure.Message, "s3cret-value") {
			t.Errorf("%s: failure = %+v", reason, failure)
		}
	}
}

func TestASourceWhoseSecretCannotBeReadIsDown(t *testing.T) {
	r := newCallRig(t, Limits{})
	r.row.Secret, r.row.SecretHeader = []byte("not sealed by this key"), text("X-Api-Key")
	r.attempt.row = r.row
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
	if failure := failureOf(t, err); failure.Reason != FailureUnreadable {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestOneSourceTakesAtMostItsCapOfCallsAtOnce(t *testing.T) {
	r := newCallRig(t, Limits{InFlight: 2})
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	r.connector.answer = func(context.Context, Target, string, map[string]any) (Called, error) {
		entered <- struct{}{}
		<-release
		return Called{Text: "ok"}, nil
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempt := *r.attempt
			if _, err := r.service.dispatch(t.Context(), personAccess(auth.RoleViewer), &attempt, []Tool{approvedSearch(t, ToolApproved)}, json.RawMessage(`{"query":"x"}`)); err != nil {
				t.Error(err)
			}
		}()
	}
	<-entered
	<-entered
	_, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved))
	if failure := failureOf(t, err); failure.Code != tools.ContextCodeSourceBusy {
		t.Fatalf("failure = %+v", failure)
	}
	close(release)
	wg.Wait()
	r.connector.answer = nil
	if _, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"x"}`, approvedSearch(t, ToolApproved)); err != nil {
		t.Fatalf("a finished call must free its place: %v", err)
	}
}

func TestAnotherSourceIsNotHeldUpByABusyOne(t *testing.T) {
	var g gate
	release, ok := g.acquire("a", 1)
	if !ok {
		t.Fatal("first place refused")
	}
	if _, ok := g.acquire("a", 1); ok {
		t.Error("a second call on a full source was admitted")
	}
	if other, ok := g.acquire("b", 1); !ok {
		t.Error("another source was held up")
	} else {
		other()
	}
	release()
	release()
	if again, ok := g.acquire("a", 1); !ok {
		t.Error("the place was not freed")
	} else {
		again()
	}
}

func TestArgumentsAreLoggedWithoutTheValuesTheSchemaMarksSecret(t *testing.T) {
	got := loggedArguments(json.RawMessage(searchSchema), json.RawMessage(`{"query":"checkout","limit":3,"token":"abc-123"}`))
	if got["query"] != "checkout" || got["limit"] != float64(3) || got["token"] != maskedValue {
		t.Fatalf("logged = %v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "abc-123") {
		t.Errorf("the secret value is in %s", raw)
	}
}

func TestLongNestedAndPasswordArgumentsAreSummarisedOrMasked(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"pw":{"type":"string","format":"password"}}}`)
	long := strings.Repeat("x", 1000)
	got := loggedArguments(schema, json.RawMessage(`{"pw":"hunter2","long":"`+long+`","list":[1,2],"obj":{"a":1}}`))
	if got["pw"] != maskedValue || got["list"] != "[2 values]" || got["obj"] != "[object]" || len(got["long"].(string)) > maxLoggedValueBytes+3 {
		t.Fatalf("logged = %v", got)
	}
}

func TestACallEventCarriesSourceToolAndDurationAndNoResult(t *testing.T) {
	r := newCallRig(t, Limits{})
	if _, err := r.dispatch(t.Context(), personAccess(auth.RoleViewer), `{"query":"checkout","token":"abc-123"}`, approvedSearch(t, ToolApproved)); err != nil {
		t.Fatal(err)
	}
	event := callEvent(*r.attempt, nil, 42*time.Millisecond)
	raw, _ := json.Marshal(event.Data)
	if event.Action != actionCall || event.Outcome != audit.OutcomeSuccess || event.Duration != 42*time.Millisecond || event.Target.ID != "code" || event.Target.Name != "Code" {
		t.Fatalf("event = %+v", event)
	}
	if event.Data["tool"] != "search" || event.Data["host"] != fakeHost || strings.Contains(string(raw), "abc-123") || strings.Contains(string(raw), "found it") {
		t.Errorf("data = %s", raw)
	}
	if _, ok := audit.Lookup(actionCall); !ok {
		t.Error("the call action is not in the audit catalog")
	}
}

func TestACallEventSaysHowARefusedCallWasRefused(t *testing.T) {
	attempt := callAttempt{uid: "code", tool: "search", known: true, row: dbsqlc.ContextSource{Uid: "code", Name: "Code", Url: "https://" + fakeHost + "/mcp"}}
	cases := []struct {
		err     error
		outcome audit.Outcome
		reason  string
	}{
		{&tools.ContextFailure{Code: tools.ContextCodeToolNotApproved}, audit.OutcomeFailed, tools.ContextCodeToolNotApproved},
		{&tools.ContextFailure{Code: tools.ContextCodeSourceDown, Reason: FailureTimeout}, audit.OutcomeFailed, FailureTimeout},
		{&tools.ContextFailure{Code: tools.ContextCodeSourceNotFound}, audit.OutcomeDenied, tools.ContextCodeSourceNotFound},
		{datasource.ErrWithheld, audit.OutcomeDenied, "withheld"},
		{context.Canceled, audit.OutcomeFailed, "call_failed"},
	}
	for _, c := range cases {
		event := callEvent(attempt, c.err, time.Millisecond)
		if event.Outcome != c.outcome || event.Reason != c.reason {
			t.Errorf("%v: outcome %q reason %q, want %q %q", c.err, event.Outcome, event.Reason, c.outcome, c.reason)
		}
	}
	attempt.result = tools.ContextResult{IsError: true, Cut: true}
	event := callEvent(attempt, nil, time.Millisecond)
	if event.Outcome != audit.OutcomeFailed || event.Reason != reasonToolError || event.Data["truncated"] != true {
		t.Errorf("event = %+v", event)
	}
}

func TestOffersListApprovedToolsWithOneLineAndTheirArguments(t *testing.T) {
	stored := []Tool{
		approvedSearch(t, ToolApproved),
		storedFrom(t, ToolOffered, Found{Name: "other", Description: "not approved", ReadOnly: yes()}),
		storedFrom(t, ToolChanged, Found{Name: "moved", Description: "changed", ReadOnly: yes()}),
	}
	offer := offerOf(Source{UID: "code", Name: "Code", Description: "Runbooks", URL: "https://" + fakeHost + "/mcp", State: StateUnreachable, Failure: FailureTimeout}, stored)
	if offer.Host != fakeHost || offer.Description != "Runbooks" || !offer.Unreachable || offer.Failure != FailureTimeout || len(offer.Tools) != 1 {
		t.Fatalf("offer = %+v", offer)
	}
	tool := offer.Tools[0]
	if tool.Name != "search" || tool.Description != "Search the code" || tool.Arguments != "query (string, required), limit (integer), token (string)" {
		t.Errorf("tool = %+v", tool)
	}
}

func TestACallerWithoutDatasourceAccessIsRefused(t *testing.T) {
	r := newCallRig(t, Limits{})
	if _, err := r.service.Call(t.Context(), "code", "search", nil); !errors.Is(err, datasource.ErrNoAccess) {
		t.Fatalf("err = %v", err)
	}
	if _, err := r.service.Offers(t.Context()); !errors.Is(err, datasource.ErrNoAccess) {
		t.Fatalf("err = %v", err)
	}
}
