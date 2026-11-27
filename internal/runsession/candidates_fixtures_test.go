package runsession

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const concludeFixture = "../../spec/examples/agent/triage/conclude.json"

type concludeWant struct {
	Problems   []string              `json:"problems"`
	Open       []string              `json:"open"`
	Unclosed   string                `json:"unclosed"`
	Unverified []findings.Unverified `json:"unverified"`
}

type concludeFixtures struct {
	Leads []triage.Lead `json:"leads"`
	Seed  []struct {
		Name   string                 `json:"name"`
		Loop   bool                   `json:"loop"`
		Task   string                 `json:"task"`
		Swept  bool                   `json:"swept"`
		Ledger []revisions.Hypothesis `json:"ledger"`
		Want   []revisions.Hypothesis `json:"want"`
	} `json:"seed"`
	Problems []struct {
		Name        string                 `json:"name"`
		Ledger      []revisions.Hypothesis `json:"ledger"`
		TransientAt map[string]int         `json:"transient_at"`
		Finding     json.RawMessage        `json:"finding"`
		Want        *concludeWant          `json:"want"`
	} `json:"problems"`
	Transient []struct {
		Name  string `json:"name"`
		Steps []struct {
			Reads  int                    `json:"reads"`
			Ledger []revisions.Hypothesis `json:"ledger"`
		} `json:"steps"`
		Want map[string]int `json:"want"`
	} `json:"transient"`
}

func TestConcludeFixtures(t *testing.T) {
	data, err := os.ReadFile(concludeFixture)
	if err != nil {
		t.Fatal(err)
	}
	var fx concludeFixtures
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Leads) == 0 || len(fx.Seed) == 0 || len(fx.Problems) == 0 || len(fx.Transient) == 0 {
		t.Fatal("the conclude fixtures are empty")
	}
	same := func(name string, got, want any) {
		t.Helper()
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if !bytes.Equal(g, w) {
			t.Errorf("%s\n got %s\nwant %s", name, g, w)
		}
	}
	for i, c := range fx.Seed {
		task, ok := route.Parse(c.Task)
		if !ok {
			t.Fatalf("%s: unknown task %q", c.Name, c.Task)
		}
		s := State{Guard: guard.New(tools.ModeInvestigate), Loop: c.Loop, Task: task, Ledger: c.Ledger}
		if c.Swept {
			s.Triage = &Triage{Leads: fx.Leads}
		}
		s.seedLedger()
		got := s.Ledger
		if got == nil {
			got = []revisions.Hypothesis{}
		}
		if *updateSessions {
			fx.Seed[i].Want = got
		} else {
			same(c.Name, got, c.Want)
		}
	}
	for i, c := range fx.Problems {
		problems, open := candidateProblems(fx.Leads, c.Ledger, c.TransientAt, concludedOf(c.Finding))
		s := State{Guard: guard.New(tools.ModeInvestigate), Loop: true, Task: route.Incident, Triage: &Triage{Leads: fx.Leads}, Ledger: c.Ledger, TransientAt: c.TransientAt}
		call := guard.Call{Name: toolSaveFinding, Input: c.Finding}
		items := s.checkCandidates(&call)
		got := &concludeWant{Problems: problems, Open: []string{}, Unclosed: call.Unclosed, Unverified: items}
		if got.Problems == nil {
			got.Problems = []string{}
		}
		if got.Unverified == nil {
			got.Unverified = []findings.Unverified{}
		}
		for _, l := range open {
			got.Open = append(got.Open, l.ID)
		}
		if *updateSessions {
			fx.Problems[i].Want = got
		} else {
			same(c.Name, got, c.Want)
		}
	}
	for i, c := range fx.Transient {
		s := &State{Guard: guard.New(tools.ModeInvestigate), Loop: true, Task: route.Incident, Triage: &Triage{Leads: fx.Leads}}
		for _, step := range c.Steps {
			s.DataReads = step.Reads
			s.noteLedger(step.Ledger)
		}
		got := s.TransientAt
		if got == nil {
			got = map[string]int{}
		}
		if *updateSessions {
			fx.Transient[i].Want = got
		} else {
			same(c.Name, got, c.Want)
		}
	}
	if *updateSessions {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fx); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(concludeFixture, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
