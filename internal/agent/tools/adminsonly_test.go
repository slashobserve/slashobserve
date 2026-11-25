package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func adminsOnlyTools(t *testing.T) (*Registry, *atomic.Int32) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}})
	if err != nil {
		t.Fatal(err)
	}
	var touched atomic.Int32
	audit := &fakeDS{
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			touched.Add(1)
			return nil, nil
		},
		discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
			touched.Add(1)
			return &datasource.Catalog{}, nil
		},
		lintFn: func(context.Context, string) ([]datasource.Diagnostic, error) {
			touched.Add(1)
			return nil, nil
		},
	}
	if err := reg.AddBuiltIn(datasource.Instance{UID: "audit", Name: "Audit", Datasource: audit}); err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger}), &touched
}

var adminsOnlyCalls = []struct {
	tool  string
	input string
}{
	{nameQueryInstant, `{"datasource":"%s","query":"up"}`},
	{nameListMetrics, `{"datasource":"%s"}`},
	{nameListLabels, `{"datasource":"%s"}`},
	{nameLintQuery, `{"datasource":"%s","query":"up"}`},
	{nameRecheckDatasource, `{"datasource":"%s"}`},
}

func content(t *testing.T, res Result) string {
	t.Helper()
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAnAdminsOnlyDatasourceAnswersANonAdminToolLikeAMissingOne(t *testing.T) {
	principals := []struct {
		name  string
		who   auth.Principal
		reads bool
	}{
		{"viewer", auth.Principal{Kind: auth.KindUser, ID: "v", Role: auth.RoleViewer}, false},
		{"commenter", auth.Principal{Kind: auth.KindUser, ID: "c", Role: auth.RoleCommenter}, false},
		{"editor", auth.Principal{Kind: auth.KindUser, ID: "e", Role: auth.RoleEditor}, false},
		{"admin", auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}, true},
		{"editor service account key", auth.Principal{Kind: auth.KindServiceAccount, ID: "ek", Role: auth.RoleEditor}, false},
		{"admin service account key", auth.Principal{Kind: auth.KindServiceAccount, ID: "ak", Role: auth.RoleAdmin}, true},
	}
	for _, p := range principals {
		t.Run(p.name, func(t *testing.T) {
			r, touched := adminsOnlyTools(t)
			ctx := auth.WithPrincipal(t.Context(), p.who)
			listed, err := r.Call(ctx, nameListDatasources, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(content(t, listed), `"audit"`) != p.reads {
				t.Fatalf("list_datasources = %s", content(t, listed))
			}
			for _, c := range adminsOnlyCalls {
				got, err := r.Call(ctx, c.tool, []byte(fmt.Sprintf(c.input, "audit")))
				if err != nil {
					t.Fatal(err)
				}
				if p.reads {
					if got.IsError {
						t.Fatalf("%s as an Admin = %s", c.tool, content(t, got))
					}
					continue
				}
				missing, err := r.Call(ctx, c.tool, []byte(fmt.Sprintf(c.input, "missing")))
				if err != nil {
					t.Fatal(err)
				}
				if !got.IsError || strings.ReplaceAll(content(t, got), "audit", "missing") != content(t, missing) {
					t.Fatalf("%s restricted = %s, missing = %s", c.tool, content(t, got), content(t, missing))
				}
			}
			if !p.reads && touched.Load() != 0 {
				t.Fatalf("a non-Admin call reached the admins only datasource %d times", touched.Load())
			}
		})
	}
}

func TestARunReadsAnAdminsOnlyDatasourceOnlyForAnAdminWritingWhereOnlyAdminsRead(t *testing.T) {
	admin := auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}
	editor := auth.Principal{Kind: auth.KindUser, ID: "e", Role: auth.RoleEditor}
	private := RunWrites{}
	adminsFolder := RunWrites{Shared: true, AdminsOnly: true}
	sharedFolder := RunWrites{Shared: true}
	cases := []struct {
		name   string
		access datasource.Access
		reads  bool
		code   string
	}{
		{"an Admin's private run", RunAccess(revisions.Request{RequestedBy: admin}, private), true, ""},
		{"an Admin's run writing into a folder only Admins read", RunAccess(revisions.Request{RequestedBy: admin}, adminsFolder), true, ""},
		{"an Admin's run writing where others read", RunAccess(revisions.Request{RequestedBy: admin}, sharedFolder), false, CodeAdminsOnlyDatasource},
		{"an Editor's private run", RunAccess(revisions.Request{RequestedBy: editor}, private), false, string(queryproxy.CodeUnknownDatasource)},
		{"an Editor's run writing into a folder only Admins read", RunAccess(revisions.Request{RequestedBy: editor}, adminsFolder), false, string(queryproxy.CodeUnknownDatasource)},
		{"a run with no access worked out", datasource.Access{}, false, string(queryproxy.CodeUnknownDatasource)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, touched := adminsOnlyTools(t)
			requester := tc.access.By
			if requester.Role == "" {
				requester = auth.Principal{Kind: auth.KindUser, ID: "v", Role: auth.RoleViewer}
			}
			ctx := WithRun(auth.WithPrincipal(t.Context(), requester), Run{Mode: ModeInvestigate, Access: tc.access})
			res, err := r.Call(ctx, nameQueryInstant, []byte(`{"datasource":"audit","query":"up"}`))
			if err != nil {
				t.Fatal(err)
			}
			if res.IsError == tc.reads || (touched.Load() != 0) != tc.reads {
				t.Fatalf("query_instant = %s, touched %d", content(t, res), touched.Load())
			}
			if !tc.reads && !strings.Contains(content(t, res), `"code":"`+tc.code+`"`) {
				t.Fatalf("query_instant = %s, want code %s", content(t, res), tc.code)
			}
			if tc.code == CodeAdminsOnlyDatasource && !strings.Contains(content(t, res), "private chat") {
				t.Fatalf("query_instant = %s, want where the data can be read", content(t, res))
			}
		})
	}
}

func TestTheRunAudienceFollowsWhoReadsWhatTheRunWrites(t *testing.T) {
	cases := []struct {
		writes RunWrites
		want   datasource.Audience
	}{
		{RunWrites{}, datasource.AudienceCaller},
		{RunWrites{Shared: true, AdminsOnly: true}, datasource.AudienceAdmins},
		{RunWrites{Shared: true}, datasource.AudienceEveryone},
	}
	for _, tc := range cases {
		if got := RunAudience(tc.writes); got != tc.want {
			t.Fatalf("RunAudience(%+v) = %q, want %q", tc.writes, got, tc.want)
		}
	}
	by := auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}
	run, err := RunFor(revisions.Request{ID: runForRequestID, Intent: revisions.IntentAsk, RequestedBy: by})
	if err != nil {
		t.Fatal(err)
	}
	if run.Access.By.ID != "" || run.Access.Audience != datasource.AudienceCaller || run.Access.System {
		t.Fatalf("RunFor decided access %+v; the run session works it out from where the run writes", run.Access)
	}
	if !Withheld(RunAccess(revisions.Request{RequestedBy: by}, RunWrites{Shared: true})) || Withheld(RunAccess(revisions.Request{RequestedBy: by}, RunWrites{})) {
		t.Fatal("Withheld does not follow an Admin writing for everyone")
	}
}

func TestListDatasourcesNamesTheAudienceOnlyForAnAdmin(t *testing.T) {
	r, _ := adminsOnlyTools(t)
	cases := []struct {
		who  auth.Principal
		want []string
		not  string
	}{
		{auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}, []string{`"uid":"audit"`, `"audience":"admins"`, `"audience":"everyone"`}, ""},
		{auth.Principal{Kind: auth.KindUser, ID: "e", Role: auth.RoleEditor}, []string{`"uid":"` + fakeUID + `"`}, `"audience"`},
	}
	for _, tc := range cases {
		listed, err := r.Call(auth.WithPrincipal(t.Context(), tc.who), nameListDatasources, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		got := content(t, listed)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("%s: list_datasources = %s, want %s", tc.who.Role, got, want)
			}
		}
		if tc.not != "" && strings.Contains(got, tc.not) {
			t.Fatalf("%s: list_datasources = %s, want no %s", tc.who.Role, got, tc.not)
		}
	}
}

func TestTheSpecGuideTeachesTheAuditTrailOnlyToAnAdmin(t *testing.T) {
	for _, who := range []auth.Principal{
		{Kind: auth.KindUser, ID: "e", Role: auth.RoleEditor},
		{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin},
	} {
		r, _ := adminsOnlyTools(t)
		ctx := auth.WithPrincipal(t.Context(), who)
		res, err := r.Call(ctx, nameGetSpecGuide, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := strings.Contains(content(t, res), "The audit trail"), who.Role == auth.RoleAdmin; got != want {
			t.Errorf("%s: the guide holds the audit section = %v, want %v", who.Role, got, want)
		}
		if got, want := r.DatasourceTypes(ctx), who.Role == auth.RoleAdmin; slices.Contains(got, prompts.AuditTrail) != want {
			t.Errorf("%s: the observe prompt's types = %v", who.Role, got)
		}
	}
}

func TestLookingUpAnAdminsOnlyDatasourcesTypeDoesNotMarkTheRun(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.AddBuiltIn(datasource.Instance{UID: "audit", Name: "Audit", Datasource: &fakeDS{typ: "loki"}}); err != nil {
		t.Fatal(err)
	}
	r := New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
	admin := auth.Principal{Kind: auth.KindUser, ID: "a", Role: auth.RoleAdmin}
	ctx, read := datasource.NotingRestrictedReads(auth.WithPrincipal(t.Context(), admin))
	res, err := r.Call(WithRun(ctx, Run{Mode: ModeInvestigate, Access: RunAccess(revisions.Request{RequestedBy: admin}, RunWrites{})}), nameListMetrics, []byte(`{"datasource":"audit"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || read() {
		t.Fatalf("list_metrics on a logs datasource only Admins read = %s, marked %v", content(t, res), read())
	}
}
