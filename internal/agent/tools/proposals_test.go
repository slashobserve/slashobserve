package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const unknownID = "99999999-9999-4999-8999-999999999999"

type proposalsRig struct {
	*panelsRig
	dashboard string
}

func newProposalsRig(t *testing.T) *proposalsRig {
	t.Helper()
	rig := newPanelsRig(t)
	return &proposalsRig{panelsRig: rig, dashboard: rig.seedStats(t)}
}

func asUser(p auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), p)
}

func (r *proposalsRig) propose(by context.Context, t *testing.T, p1Title string) string {
	t.Helper()
	res, _ := r.as(by).run(t, nameUpdatePanels, panelPatch(r.dashboard, 1, `[`+suggestStat("p1", p1Title, "count(up)", 0)+`]`))
	out, ok := res.Content.(proposedOut)
	if res.IsError || !ok {
		t.Fatalf("propose: %#v", res.Content)
	}
	return out.ProposalID
}

func (r *proposalsRig) editMain(t *testing.T, p1Title string) {
	t.Helper()
	savedVersion(t, firstOf(r.as(asUser(editorCaller)).run(t, nameUpdatePanels, panelPatch(r.dashboard, r.latest(t, r.dashboard).Number, `[`+suggestStat("p1", p1Title, "count(up)", 0)+`]`))))
}

func (r *proposalsRig) call(by context.Context, t *testing.T, tool, input string) Result {
	t.Helper()
	res, _ := r.as(by).run(t, tool, input)
	return res
}

func summaryOf(t *testing.T, res Result) proposals.SummaryJSON {
	t.Helper()
	out, ok := res.Content.(proposals.SummaryJSON)
	if res.IsError || !ok {
		t.Fatalf("expected a proposal summary, got %#v", res.Content)
	}
	return out
}

func mergeInput(id string, revision int, choices string) string {
	if choices == "" {
		return fmt.Sprintf(`{"proposal_id":%q,"revision":%d}`, id, revision)
	}
	return fmt.Sprintf(`{"proposal_id":%q,"revision":%d,"choices":%s}`, id, revision, choices)
}

func idInput(id string) string { return fmt.Sprintf(`{"proposal_id":%q}`, id) }

func TestListProposals(t *testing.T) {
	rig := newProposalsRig(t)
	id := rig.propose(asUser(commenterCaller), t, "Proposed")
	list := func(input string) Result { return rig.call(asUser(editorCaller), t, nameListProposals, input) }
	head := `{"dashboard_id":"` + rig.dashboard + `"`

	t.Run("open proposals by default", func(t *testing.T) {
		res := list(head + `}`)
		out, ok := res.Content.(proposalListOut)
		if res.IsError || !ok || len(out.Proposals) != 1 || out.NextCursor != nil {
			t.Fatalf("res = %#v", res.Content)
		}
		got := out.Proposals[0]
		if got.ID != id || got.State != "open" || got.Revision != 1 || got.DashboardID != rig.dashboard || !slices.Contains(got.AllowedActions, "merge") {
			t.Fatalf("proposal = %+v", got)
		}
		if res.Step != "Listed 1 proposal" {
			t.Errorf("step = %q", res.Step)
		}
	})
	t.Run("a state filter", func(t *testing.T) {
		for state, want := range map[string]int{"open": 1, "merged": 0, "discarded": 0, "all": 1} {
			out, ok := list(head + `,"state":"` + state + `"}`).Content.(proposalListOut)
			if !ok || len(out.Proposals) != want {
				t.Errorf("state %s: %#v", state, out)
			}
		}
	})
	t.Run("input and access failures", func(t *testing.T) {
		for name, tt := range map[string]struct {
			input, code string
		}{
			"no dashboard":     {`{}`, codeInvalidRequest},
			"a bad dashboard":  {`{"dashboard_id":"nope"}`, codeInvalidRequest},
			"a bad state":      {head + `,"state":"stale"}`, codeInvalidRequest},
			"a bad cursor":     {head + `,"cursor":"nope"}`, codeInvalidRequest},
			"an unknown board": {`{"dashboard_id":"` + unknownID + `"}`, codeNotFound},
			"an unknown field": {head + `,"limit":5}`, codeInvalidRequest},
		} {
			if got := errorCodeOf(t, list(tt.input)); got != tt.code {
				t.Errorf("%s: code = %q, want %q", name, got, tt.code)
			}
		}
	})
	t.Run("a viewer may list", func(t *testing.T) {
		res := rig.call(asUser(viewerCaller), t, nameListProposals, head+`}`)
		if out, ok := res.Content.(proposalListOut); res.IsError || !ok || len(out.Proposals) != 1 {
			t.Fatalf("res = %#v", res.Content)
		}
	})
}

func TestGetProposal(t *testing.T) {
	rig := newProposalsRig(t)
	id := rig.propose(asUser(commenterCaller), t, "Proposed")
	rig.editMain(t, "Main")

	t.Run("the spec, the changes and the conflicts", func(t *testing.T) {
		res := rig.call(asUser(editorCaller), t, nameGetProposal, idInput(id))
		out, ok := res.Content.(proposals.ProposalJSON)
		if res.IsError || !ok {
			t.Fatalf("res = %#v", res.Content)
		}
		if out.ID != id || out.Spec == nil || len(out.Changes) != 1 || out.Changes[0].ID != "p1" || !out.Behind || out.LatestVersion != 2 {
			t.Fatalf("out = %+v", out)
		}
		if len(out.Conflicts) != 1 || out.Conflicts[0].Kind != spec.UnitPanel || out.Conflicts[0].ID != "p1" {
			t.Fatalf("conflicts = %+v", out.Conflicts)
		}
		if !slices.Contains(out.AllowedActions, "merge") || res.Step != "Read proposal revision 1" {
			t.Errorf("actions = %v step = %q", out.AllowedActions, res.Step)
		}
	})
	t.Run("a person's proposal names its author", func(t *testing.T) {
		out := rig.call(asUser(editorCaller), t, nameGetProposal, idInput(id)).Content.(proposals.ProposalJSON)
		if out.Actor.Label != "Cam" || out.Actor.Client != nil || out.Actor.RunID != nil {
			t.Fatalf("actor = %+v", out.Actor)
		}
	})
	t.Run("failures", func(t *testing.T) {
		for name, tt := range map[string]struct{ input, code string }{
			"no id":      {`{}`, codeInvalidRequest},
			"a bad id":   {`{"proposal_id":"nope"}`, codeInvalidRequest},
			"an unknown": {idInput(unknownID), codeNotFound},
		} {
			if got := errorCodeOf(t, rig.call(asUser(editorCaller), t, nameGetProposal, tt.input)); got != tt.code {
				t.Errorf("%s: code = %q, want %q", name, got, tt.code)
			}
		}
	})
	t.Run("a spec over the limit is too large", func(t *testing.T) {
		big := newPanelsRig(t)
		chunk := strings.Repeat("a", 19000)
		var panels []string
		for i := range MaxModelSpecBytes/len(chunk) + 1 {
			panels = append(panels, suggestText(fmt.Sprintf("t%d", i), chunk, i*4))
		}
		dash := big.seed(t, panels...)
		res, _ := big.as(asUser(commenterCaller)).run(t, nameUpdatePanels, panelPatch(dash, 1, `[`+suggestText("small", "small", 400)+`]`))
		proposal, ok := res.Content.(proposedOut)
		if !ok {
			t.Fatalf("propose: %#v", res.Content)
		}
		res, _ = big.as(asUser(editorCaller)).run(t, nameGetProposal, idInput(proposal.ProposalID))
		if got := errorCodeOf(t, res); got != codeTooLarge {
			t.Fatalf("code = %q", got)
		}
	})
}

func TestProposalsOverMCPNameTheClient(t *testing.T) {
	rig := newProposalsRig(t)
	mcp := origin.With(asUser(commenterCaller), origin.Origin{Kind: origin.MCP, Client: "claude-code"})
	id := rig.propose(mcp, t, "From an agent")

	got := rig.call(asUser(editorCaller), t, nameGetProposal, idInput(id)).Content.(proposals.ProposalJSON)
	if got.Actor.Label != "Cam (via Claude Code)" || got.Actor.Client == nil || *got.Actor.Client != "Claude Code" {
		t.Fatalf("get actor = %+v", got.Actor)
	}
	listed := rig.call(asUser(editorCaller), t, nameListProposals, `{"dashboard_id":"`+rig.dashboard+`"}`).Content.(proposalListOut)
	if len(listed.Proposals) != 1 || listed.Proposals[0].Actor.Label != "Cam (via Claude Code)" {
		t.Fatalf("list = %+v", listed.Proposals)
	}
}

func TestMergeProposal(t *testing.T) {
	t.Run("an editor merges and the dashboard gains a version", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		res := rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, ""))
		out := summaryOf(t, res)
		if out.State != "merged" || out.MergedVersion == nil || *out.MergedVersion != 2 || res.Step != "Merged proposal as version 2" {
			t.Fatalf("out = %+v step = %q", out, res.Step)
		}
		if got := rig.latest(t, rig.dashboard).Number; got != 2 {
			t.Fatalf("latest = %d", got)
		}
	})
	t.Run("a change already in the dashboard closes the proposal without a version", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Same")
		rig.editMain(t, "Same")
		res := rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, ""))
		out := summaryOf(t, res)
		if out.State != "merged" || out.MergedVersion != nil || res.Step != "Merged proposal; the dashboard already had its change" {
			t.Fatalf("out = %+v step = %q", out, res.Step)
		}
	})
	t.Run("conflicts are named and a choice resolves them", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Theirs")
		rig.editMain(t, "Ours")
		res := rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, ""))
		conflict, ok := res.Content.(conflictOut)
		if !res.IsError || !ok || conflict.Code != codeVersionConflict || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].ID != "p1" {
			t.Fatalf("res = %#v", res.Content)
		}
		res = rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, `[{"kind":"panel","id":"p1","take":"theirs"}]`))
		if out := summaryOf(t, res); out.State != "merged" || out.MergedVersion == nil || *out.MergedVersion != 3 {
			t.Fatalf("out = %+v", out)
		}
		titles := map[string]string{}
		for _, p := range rig.latest(t, rig.dashboard).Spec.AllPanels() {
			titles[p.Panel.ID] = p.Panel.Title
		}
		if titles["p1"] != "Theirs" {
			t.Fatalf("titles = %v", titles)
		}
	})
	t.Run("failures", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		editor := asUser(editorCaller)
		for name, tt := range map[string]struct {
			by          context.Context
			input, code string
		}{
			"no revision":           {editor, idInput(id), codeInvalidRequest},
			"a zero revision":       {editor, mergeInput(id, 0, ""), codeInvalidRequest},
			"a bad id":              {editor, mergeInput("nope", 1, ""), codeInvalidRequest},
			"an unknown proposal":   {editor, mergeInput(unknownID, 1, ""), codeNotFound},
			"a bad choice":          {editor, mergeInput(id, 1, `[{"kind":"panel","id":"p1","take":"mine"}]`), codeInvalidRequest},
			"a commenter":           {asUser(commenterCaller), mergeInput(id, 1, ""), codeForbiddenRole},
			"a propose-mode editor": {grantmode.With(editor, grantmode.Propose), mergeInput(id, 1, ""), codeProposeMode},
		} {
			res := rig.call(tt.by, t, nameMergeProposal, tt.input)
			if got := errorCodeOf(t, res); got != tt.code {
				t.Errorf("%s: code = %q (%#v), want %q", name, got, res.Content, tt.code)
			}
		}
		if got := rig.latest(t, rig.dashboard).Number; got != 1 {
			t.Fatalf("a refused merge saved version %d", got)
		}
	})
	t.Run("a stale revision names the current one", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "First")
		rig.propose(asUser(commenterCaller), t, "Second")
		res := rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, ""))
		out, ok := res.Content.(proposalChangedOut)
		if !res.IsError || !ok || out.Code != codeProposalChanged || out.Revision != 2 {
			t.Fatalf("res = %#v", res.Content)
		}
	})
	t.Run("a closed proposal names its state and what is allowed", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		summaryOf(t, rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, "")))
		res := rig.call(asUser(editorCaller), t, nameMergeProposal, mergeInput(id, 1, ""))
		out, ok := res.Content.(invalidTransitionOut)
		if !res.IsError || !ok || out.Code != codeInvalidTransition || out.State != "merged" || out.AllowedActions == nil || len(out.AllowedActions) != 0 {
			t.Fatalf("res = %#v", res.Content)
		}
	})
}

func TestDiscardProposal(t *testing.T) {
	t.Run("the author discards their own", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		res := rig.call(asUser(commenterCaller), t, nameDiscardProposal, idInput(id))
		if out := summaryOf(t, res); out.State != "discarded" || res.Step != "Discarded proposal" {
			t.Fatalf("out = %+v step = %q", out, res.Step)
		}
		again := rig.call(asUser(commenterCaller), t, nameDiscardProposal, idInput(id))
		if out, ok := again.Content.(invalidTransitionOut); !again.IsError || !ok || out.State != "discarded" {
			t.Fatalf("again = %#v", again.Content)
		}
	})
	t.Run("an editor discards another person's", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		if out := summaryOf(t, rig.call(asUser(editorCaller), t, nameDiscardProposal, idInput(id))); out.State != "discarded" {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("failures", func(t *testing.T) {
		rig := newProposalsRig(t)
		id := rig.propose(asUser(commenterCaller), t, "Proposed")
		other := auth.Principal{Kind: auth.KindUser, ID: "88888888-8888-4888-8888-888888888888", Name: "Dee", Role: auth.RoleCommenter}
		for name, tt := range map[string]struct {
			by          context.Context
			input, code string
		}{
			"no id":                 {asUser(editorCaller), `{}`, codeInvalidRequest},
			"an unknown proposal":   {asUser(editorCaller), idInput(unknownID), codeNotFound},
			"another commenter":     {asUser(other), idInput(id), codeForbiddenRole},
			"a propose-mode editor": {grantmode.With(asUser(editorCaller), grantmode.Propose), idInput(id), codeForbiddenRole},
		} {
			if got := errorCodeOf(t, rig.call(tt.by, t, nameDiscardProposal, tt.input)); got != tt.code {
				t.Errorf("%s: code = %q, want %q", name, got, tt.code)
			}
		}
	})
}
