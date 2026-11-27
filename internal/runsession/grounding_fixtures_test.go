package runsession

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/findings"
)

const groundingSessions = "../../spec/examples/agent/grounding/sessions.json"

var updateSessions = flag.Bool("update", false, "rewrite the wants in spec/examples/agent/grounding/sessions.json")

type sessionRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type sessionCall struct {
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	Content    string          `json:"content"`
	IsError    bool            `json:"is_error,omitempty"`
	Discovered *struct {
		Datasource string   `json:"datasource"`
		Of         string   `json:"of"`
		Names      []string `json:"names"`
	} `json:"discovered,omitempty"`
	Ranges []sessionRange `json:"ranges,omitempty"`
	Want   string         `json:"want"`
}

type sessionSaveWant struct {
	SendBack   string                `json:"send_back"`
	SentBack   bool                  `json:"sent_back"`
	Unverified []findings.Unverified `json:"unverified"`
	Saved      map[string]any        `json:"saved"`
}

type sessionSave struct {
	Input json.RawMessage  `json:"input"`
	Want  *sessionSaveWant `json:"want"`
}

type sessionCase struct {
	Name     string         `json:"name"`
	Mode     grounding.Mode `json:"mode"`
	Task     string         `json:"task,omitempty"`
	Question string         `json:"question"`
	Calls    []sessionCall  `json:"calls"`
	Saves    []sessionSave  `json:"saves"`
}

func sessionScript(t *testing.T, c sessionCase) *script {
	t.Helper()
	s := groundingScript(t, c.Mode)
	task, ok := route.Parse(c.Task)
	if !ok {
		t.Fatalf("%s: unknown task %q", c.Name, c.Task)
	}
	s.sess.state.Task = task
	return s
}

func (s *script) callOf(c sessionCall, turn *Turn) string {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: c.Tool, Input: c.Input}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil || admitted.answer != nil {
		s.t.Fatalf("admit %s: %+v, %v", c.Tool, admitted.answer, err)
	}
	res := tools.Result{Content: json.RawMessage(c.Content), IsError: c.IsError, Step: "Read " + c.Tool}
	if c.Discovered != nil {
		res.Discovered = &tools.Discovery{Datasource: c.Discovered.Datasource, Of: c.Discovered.Of, Names: c.Discovered.Names}
	}
	done := dispatched{res: res, content: []byte(c.Content)}
	for _, r := range c.Ranges {
		done.ranges = append(done.ranges, datasource.TimeRange{From: r.From, To: r.To})
	}
	answer, err := s.sess.settle(ctx, call, admitted, done)
	if err != nil {
		s.t.Fatalf("settle %s: %v", c.Tool, err)
	}
	return string(answer.Content)
}

func signalsMap(s findings.Signals) map[string]any {
	kinds := s.Kinds
	if kinds == nil {
		kinds = []string{}
	}
	return map[string]any{"kinds": kinds, "component": s.Component, "onset_read": s.OnsetRead, "onset_fact": s.OnsetFact, "trigger_read": s.TriggerRead, "ruled_out": s.RuledOut}
}

func savedOf(t *testing.T, input json.RawMessage, check groundingCheck) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(input, &out); err != nil {
		t.Fatal(err)
	}
	unverified := check.unverified
	if unverified == nil {
		unverified = []findings.Unverified{}
	}
	out["unverified"] = unverified
	if check.ledger == nil {
		return out
	}
	render := func(v any) any {
		if text, ok := v.(string); ok {
			rendered, _ := check.ledger.Render(text)
			return rendered
		}
		return v
	}
	for _, key := range []string{"title", "markdown", "impact"} {
		if v, ok := out[key]; ok {
			out[key] = render(v)
		}
	}
	causes, _ := out["causes"].([]any)
	if cause, ok := out["root_cause"].(map[string]any); ok {
		if len(causes) == 0 && len(check.signals) == 1 {
			text := func(v any) string { s, _ := v.(string); return s }
			scoreCause(cause, check, grounding.CitedFacts(text(out["markdown"]), text(out["impact"]), text(cause["summary"])), 0)
		}
		if v, ok := cause["summary"]; ok {
			cause["summary"] = render(v)
		}
	}
	for _, key := range []string{"ruled_out", "next_steps"} {
		if list, ok := out[key].([]any); ok {
			for i := range list {
				list[i] = render(list[i])
			}
		}
	}
	for i, raw := range causes {
		c, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := c["mechanism"]; ok {
			c["mechanism"] = render(v)
		}
		if i >= len(check.signals) {
			continue
		}
		var facts []string
		if evidence, ok := c["evidence"].(map[string]any); ok {
			list, _ := evidence["facts"].([]any)
			for _, id := range list {
				if text, ok := id.(string); ok && !slices.Contains(facts, strings.TrimSpace(text)) {
					facts = append(facts, strings.TrimSpace(text))
				}
			}
		}
		scoreCause(c, check, facts, i)
	}
	return out
}

func scoreCause(c map[string]any, check groundingCheck, facts []string, i int) {
	score := findings.Score(check.signals[i])
	c["signals"], c["score"], c["band"] = signalsMap(check.signals[i]), score, findings.BandOf(score)
	s := findings.Structured{Causes: []findings.Cause{{Evidence: findings.CauseEvidence{Facts: facts}}}}
	s.WithQueries(check.ledger.QueryOf)
	if queries := s.Causes[0].Evidence.Queries; len(queries) > 0 {
		c["queries"] = queries
	} else {
		delete(c, "queries")
	}
}

func TestGroundingSessionFixtures(t *testing.T) {
	data, err := os.ReadFile(filepath.Clean(groundingSessions))
	if err != nil {
		t.Fatal(err)
	}
	var cases []sessionCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("the session fixtures are empty")
	}
	for ci := range cases {
		c := &cases[ci]
		s := sessionScript(t, *c)
		c.Question = s.sess.req.Prompt
		for i, call := range c.Calls {
			got := s.callOf(call, &Turn{ID: "call" + string(rune('a'+i)), Calls: 1})
			if *updateSessions {
				c.Calls[i].Want = got
			} else if got != call.Want {
				t.Errorf("%s: %s content =\n%s\nwant\n%s", c.Name, call.Tool, got, call.Want)
			}
		}
		for i, save := range c.Saves {
			gc := guard.Call{Name: "save_finding", Input: save.Input}
			check := s.sess.state.checkGrounding(&gc, s.sess.req.Prompt)
			admitted := s.admitSave(string(save.Input), &Turn{ID: "save" + string(rune('a'+i)), Calls: 1})
			got := &sessionSaveWant{SendBack: gc.Ungrounded, SentBack: admitted.answer != nil, Unverified: check.unverified, Saved: savedOf(t, save.Input, check)}
			if got.Unverified == nil {
				got.Unverified = []findings.Unverified{}
			}
			if got.SentBack && gc.Ungrounded != "" && !strings.Contains(string(admitted.answer.Content), "Some figures, facts, ids or names") && !strings.Contains(string(admitted.answer.Content), "evidence panel") {
				t.Errorf("%s: the send-back carries neither the grounding text nor the evidence ask: %s", c.Name, admitted.answer.Content)
			}
			if *updateSessions {
				c.Saves[i].Want = got
				continue
			}
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(save.Want)
			if !bytes.Equal(g, w) {
				t.Errorf("%s: save %d\n got %s\nwant %s", c.Name, i, g, w)
			}
		}
	}
	if *updateSessions {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cases); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(groundingSessions, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
