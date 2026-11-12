package surface

import (
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/mcpauth"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/presence"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/public"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
	"github.com/slashobserve/slashobserve/internal/webhooks"
)

var updateStates = flag.Bool("update", false, "rewrite docs/reference/states.md")

const statesDocPath = "../../docs/reference/states.md"

func statesReference() string {
	return lifecycle.Render(
		revisions.RunMachine.Doc("Run", "A run is one agent request, started by `POST /observe`, a comment, a prompt, an investigation or a suggestion."),
		threads.ThreadMachine.Doc("Comment thread", "A comment thread is a conversation on a dashboard, a finding or an annotation, returned by `GET /threads` and by every route that opens, replies to, resolves or reopens one."),
		observations.Machine.Doc("Observation", "An observation is a page of blocks in a folder, started by POST /observations or by an investigation."),
		observations.BlockMachine.Doc("Observation block", "A block is one item on an observation's page: Markdown, a live panel, a frozen snapshot or an agent run."),
		proposals.ProposalMachine.Doc("Proposal", "A proposal is a change to a dashboard that someone who can edit merges or discards, returned by `GET /api/v1/proposals/{id}`."),
		health.ItemStates.Doc("Dashboard health", "`GET /health` returns the `state` of each dashboard that has a problem."),
		health.RepairOutcomes.Doc("Health repair", "`GET /health` returns the `outcome` of a dashboard's latest repair beside the repair run's own `state`."),
		webhooks.States.Doc("Webhook", "`GET /webhooks` returns the `state` of each webhook."),
		auth.KeyStates.Doc("Service-account key", "`GET /service-accounts/{id}/keys` returns the `state` of each key."),
		mcpauth.GrantStates.Doc("Connected agent", "`GET /me/grants` and `GET /grants` return the `state` of each connected agent."),
		public.LinkStates.Doc("Public link", "`GET /dashboards/{id}/public-links` returns the `state` of each link."),
		grafanaconn.SyncMachine.Doc("Grafana import", "A Grafana import is one run of `POST /grafana/{id}/imports`, answered by `GET /grafana/{id}/imports/{sync_id}`; its moves write server log lines with these names, not activity events, because a Grafana connection is Admin only."),
		presence.States.Doc("Agent presence", "Whether an outside agent is taking runs: derived from when it last asked for work and the runs it holds."),
	)
}

func TestStatesReferenceIsCurrent(t *testing.T) {
	want := statesReference()
	if *updateStates {
		if err := os.WriteFile(statesDocPath, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(statesDocPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Error("docs/reference/states.md is stale; run make generate")
	}
}

func enumOf(t *testing.T, doc *openapi3.T, schema, property string) []string {
	t.Helper()
	ref, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("schema %s is not in the document", schema)
	}
	prop, ok := ref.Value.Properties[property]
	if !ok {
		t.Fatalf("%s has no property %s", schema, property)
	}
	values := prop.Value.Enum
	if prop.Value.Items != nil {
		values = prop.Value.Items.Value.Enum
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i], _ = v.(string)
	}
	return out
}

func TestDerivedStateEnumsMatchTheTables(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		schema, property string
		want             []string
	}{
		{"HealthItem", "state", health.ItemStates.Names()},
		{"HealthRepair", "outcome", health.RepairOutcomes.Names()},
		{"Webhook", "state", webhooks.States.Names()},
		{"ServiceAccountKey", "state", auth.KeyStates.Names()},
		{"ConnectedAgent", "state", mcpauth.GrantStates.Names()},
		{"PublicLink", "state", public.LinkStates.Names()},
	}
	var offered []string
	for _, a := range threads.ThreadMachine.Offered() {
		offered = append(offered, string(a))
	}
	cases = append(cases, struct {
		schema, property string
		want             []string
	}{"Thread", "allowed_actions", offered})
	for _, c := range cases {
		if got := enumOf(t, doc, c.schema, c.property); !slices.Equal(got, c.want) {
			t.Errorf("%s.%s enum = %v, want %v", c.schema, c.property, got, c.want)
		}
	}
}

func TestRunStateEnumsMatchTheMachine(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var states, kinds, offered []string
	for _, s := range revisions.RunMachine.States {
		states = append(states, string(s))
	}
	for _, k := range revisions.ResultKinds {
		kinds = append(kinds, string(k))
	}
	for _, a := range revisions.RunMachine.Offered() {
		offered = append(offered, string(a))
	}
	for _, schema := range []string{"AgentRequest", "RequestSummary", "Revision"} {
		if got := enumOf(t, doc, schema, "state"); !slices.Equal(got, states) {
			t.Errorf("%s.state enum = %v, want %v", schema, got, states)
		}
		if got := enumOf(t, doc, schema, "result_kind"); !slices.Equal(got, kinds) {
			t.Errorf("%s.result_kind enum = %v, want %v", schema, got, kinds)
		}
	}
	for _, schema := range []string{"AgentRequest", "RequestSummary", "Revision"} {
		if got := enumOf(t, doc, schema, "allowed_actions"); !slices.Equal(got, offered) {
			t.Errorf("%s.allowed_actions items enum = %v, want %v", schema, got, offered)
		}
	}
	if got := enumOf(t, doc, "InvalidTransition", "code"); !slices.Equal(got, []string{"invalid_transition"}) {
		t.Errorf("InvalidTransition.code enum = %v", got)
	}
}

func TestProposalEnumsMatchTheMachine(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var states, offered []string
	for _, s := range proposals.ProposalMachine.States {
		states = append(states, string(s))
	}
	for _, a := range proposals.ProposalMachine.Offered() {
		offered = append(offered, string(a))
	}
	for _, schema := range []string{"Proposal", "ProposalSummary"} {
		if got := enumOf(t, doc, schema, "state"); !slices.Equal(got, states) {
			t.Errorf("%s.state enum = %v, want %v", schema, got, states)
		}
		if got := enumOf(t, doc, schema, "allowed_actions"); !slices.Equal(got, offered) {
			t.Errorf("%s.allowed_actions items enum = %v, want %v", schema, got, offered)
		}
	}
}

func TestGrafanaImportEnumsMatchTheMachine(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var states, offered []string
	for _, s := range grafanaconn.SyncMachine.States {
		states = append(states, string(s))
	}
	for _, a := range grafanaconn.SyncMachine.Offered() {
		offered = append(offered, string(a))
	}
	for _, schema := range []string{"GrafanaImport", "GrafanaImportSummary"} {
		for _, property := range []string{"state", "status"} {
			if got := enumOf(t, doc, schema, property); !slices.Equal(got, states) {
				t.Errorf("%s.%s enum = %v, want %v", schema, property, got, states)
			}
		}
		if got := enumOf(t, doc, schema, "allowed_actions"); !slices.Equal(got, offered) {
			t.Errorf("%s.allowed_actions items enum = %v, want %v", schema, got, offered)
		}
	}
}

func TestObservationEnumsMatchTheMachines(t *testing.T) {
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var states, offered, blockStates, blockOffered []string
	for _, s := range observations.Machine.States {
		states = append(states, string(s))
	}
	for _, a := range observations.Machine.Offered() {
		offered = append(offered, string(a))
	}
	for _, s := range observations.BlockMachine.States {
		if !slices.Contains(observations.BlockMachine.Terminal, s) {
			blockStates = append(blockStates, string(s))
		}
	}
	for _, a := range observations.BlockMachine.Offered() {
		blockOffered = append(blockOffered, string(a))
	}
	cases := []struct {
		schema, property string
		want             []string
	}{
		{"Observation", "state", states},
		{"Observation", "allowed_actions", offered},
		{"ObservationBlock", "state", blockStates},
		{"ObservationBlock", "allowed_actions", blockOffered},
	}
	for _, c := range cases {
		if got := enumOf(t, doc, c.schema, c.property); !slices.Equal(got, c.want) {
			t.Errorf("%s.%s enum = %v, want %v", c.schema, c.property, got, c.want)
		}
	}
}
