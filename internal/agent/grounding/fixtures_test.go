package grounding

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const fixtureDir = "../../../spec/examples/agent/grounding"

var updateFixtures = flag.Bool("update", false, "rewrite the wants in spec/examples/agent/grounding")

func readFixture(t *testing.T, name string, into any) {
	t.Helper()
	root, err := os.OpenRoot(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func writeFixture(t *testing.T, name string, from any) {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(from); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, name), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sameJSON(t *testing.T, what string, got, want any) {
	t.Helper()
	g, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	w, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(g, w) {
		t.Errorf("%s\n got %s\nwant %s", what, g, w)
	}
}

type problemJSON struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Fix  string `json:"fix"`
}

func problemsJSON(ps []Problem) []problemJSON {
	out := []problemJSON{}
	for _, p := range ps {
		out = append(out, problemJSON{p.Kind, p.Text, p.Fix})
	}
	return out
}

type figureCase struct {
	Name     string   `json:"name"`
	Text     string   `json:"text"`
	Question string   `json:"question"`
	Want     []string `json:"want"`
}

type figureFixture struct {
	Uncited []figureCase `json:"uncited"`
	Steps   []struct {
		Text string  `json:"text"`
		Step float64 `json:"step"`
	} `json:"steps"`
	Rounded []struct {
		In   float64 `json:"in"`
		Want float64 `json:"want"`
	} `json:"rounded"`
	TraceIDs struct {
		Known []string `json:"known"`
		Cases []struct {
			Text string        `json:"text"`
			Want []problemJSON `json:"want"`
		} `json:"cases"`
	} `json:"trace_ids"`
	SourceNames []struct {
		Name string   `json:"name"`
		Text string   `json:"text"`
		Want []string `json:"want"`
	} `json:"source_names"`
}

func TestFigureFixtures(t *testing.T) {
	var fx figureFixture
	readFixture(t, "figures.json", &fx)
	if len(fx.Uncited) == 0 || len(fx.Steps) == 0 || len(fx.Rounded) == 0 || len(fx.TraceIDs.Cases) == 0 || len(fx.SourceNames) == 0 {
		t.Fatal("the figure fixtures are empty")
	}
	for i, c := range fx.Uncited {
		got := []string{}
		for _, f := range UncitedFigures(c.Text, c.Question) {
			got = append(got, f.Text)
		}
		if *updateFixtures {
			fx.Uncited[i].Want = got
		} else if !slices.Equal(got, c.Want) {
			t.Errorf("%s: UncitedFigures(%q) = %q, want %q", c.Name, c.Text, got, c.Want)
		}
	}
	for i, c := range fx.Steps {
		got := scan(blank(c.Text))
		if len(got) != 1 {
			t.Fatalf("figures of %q = %+v, want one", c.Text, got)
		}
		if *updateFixtures {
			fx.Steps[i].Step = got[0].Step
		} else if got[0].Step != c.Step {
			t.Errorf("step of %q = %v, want %v", c.Text, got[0].Step, c.Step)
		}
	}
	for i, c := range fx.Rounded {
		if got := rounded(c.In); *updateFixtures {
			fx.Rounded[i].Want = got
		} else if got != c.Want {
			t.Errorf("rounded(%v) = %v, want %v", c.In, got, c.Want)
		}
	}
	for i, c := range fx.TraceIDs.Cases {
		got := problemsJSON(traceIDProblems(c.Text, fx.TraceIDs.Known))
		if *updateFixtures {
			fx.TraceIDs.Cases[i].Want = got
		} else {
			sameJSON(t, "traceIDProblems("+c.Text+")", got, c.Want)
		}
	}
	for i, c := range fx.SourceNames {
		got := sourceNames(c.Text)
		if got == nil {
			got = []string{}
		}
		if *updateFixtures {
			fx.SourceNames[i].Want = got
		} else if !slices.Equal(got, c.Want) {
			t.Errorf("%s: sourceNames = %q, want %q", c.Name, got, c.Want)
		}
	}
	if *updateFixtures {
		writeFixture(t, "figures.json", fx)
	}
}

type renderWant struct {
	Text     string        `json:"text"`
	Problems []problemJSON `json:"problems"`
}

type renderCase struct {
	Name string      `json:"name,omitempty"`
	Text string      `json:"text"`
	Want *renderWant `json:"want"`
}

func renderOf(l *Ledger, text string) *renderWant {
	out, problems := l.Render(text)
	return &renderWant{Text: out, Problems: problemsJSON(problems)}
}

type answerCase struct {
	Name    string        `json:"name,omitempty"`
	Answers []Answer      `json:"answers,omitempty"`
	Text    string        `json:"text,omitempty"`
	Want    []problemJSON `json:"want"`
}

type renderFixture struct {
	Ledger  []Fact       `json:"ledger"`
	Render  []renderCase `json:"render"`
	Answers struct {
		Ledger          []Fact       `json:"ledger"`
		AnswerProblems  []answerCase `json:"answer_problems"`
		Conflicts       []answerCase `json:"conflicts"`
		Unstated        []answerCase `json:"unstated"`
		UnstatedAnswers []Answer     `json:"unstated_answers"`
		SendBack        []Answer     `json:"send_back"`
		SendBackWant    string       `json:"send_back_want"`
	} `json:"answers"`
	MissingAnswers struct {
		Question string `json:"question"`
		Cases    []struct {
			Input json.RawMessage `json:"input"`
			Want  *problemJSON    `json:"want"`
		} `json:"cases"`
	} `json:"missing_answers"`
}

func TestRenderFixtures(t *testing.T) {
	var fx renderFixture
	readFixture(t, "render.json", &fx)
	ledger := &Ledger{Facts: fx.Ledger}
	for i, c := range fx.Render {
		got := renderOf(ledger, c.Text)
		if *updateFixtures {
			fx.Render[i].Want = got
		} else {
			sameJSON(t, "Render("+c.Text+")", got, c.Want)
		}
	}
	answers := &Ledger{Facts: fx.Answers.Ledger}
	check := func(cases []answerCase, of func(answerCase) []Problem) {
		for i, c := range cases {
			got := problemsJSON(of(c))
			if *updateFixtures {
				cases[i].Want = got
			} else {
				sameJSON(t, c.Name+c.Text, got, c.Want)
			}
		}
	}
	check(fx.Answers.AnswerProblems, func(c answerCase) []Problem { return answers.AnswerProblems(c.Answers) })
	check(fx.Answers.Conflicts, func(c answerCase) []Problem { return answers.conflicts(c.Text) })
	check(fx.Answers.Unstated, func(c answerCase) []Problem { return answers.unstated(fx.Answers.UnstatedAnswers, c.Text) })
	sendBack := SendBackProblems(answers.AnswerProblems(fx.Answers.SendBack))
	if *updateFixtures {
		fx.Answers.SendBackWant = sendBack
	} else if sendBack != fx.Answers.SendBackWant {
		t.Errorf("send-back =\n%s\nwant\n%s", sendBack, fx.Answers.SendBackWant)
	}
	for i, c := range fx.MissingAnswers.Cases {
		var got *problemJSON
		if p, ok := MissingAnswers(c.Input, fx.MissingAnswers.Question); ok {
			got = &problemJSON{p.Kind, p.Text, p.Fix}
		}
		if *updateFixtures {
			fx.MissingAnswers.Cases[i].Want = got
		} else {
			sameJSON(t, "MissingAnswers("+string(c.Input)+")", got, c.Want)
		}
	}
	if *updateFixtures {
		writeFixture(t, "render.json", fx)
	}
}

type rangeJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func rangesFrom(t *testing.T, in []rangeJSON) []Range {
	t.Helper()
	out := []Range{}
	for _, r := range in {
		from, err := time.Parse(time.RFC3339Nano, r.From)
		if err != nil {
			t.Fatal(err)
		}
		to, err := time.Parse(time.RFC3339Nano, r.To)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Range{From: from, To: to})
	}
	return out
}

type stepJSON struct {
	Read *struct {
		Content string      `json:"content"`
		Names   []string    `json:"names,omitempty"`
		Ranges  []rangeJSON `json:"ranges,omitempty"`
	} `json:"read,omitempty"`
	Issue *struct {
		Tool    string      `json:"tool"`
		Input   string      `json:"input"`
		Content string      `json:"content"`
		Ranges  []rangeJSON `json:"ranges,omitempty"`
		Want    string      `json:"want"`
	} `json:"issue,omitempty"`
	Sourced *struct {
		Content string `json:"content"`
	} `json:"sourced,omitempty"`
}

type reviewCase struct {
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input"`
	Question string          `json:"question,omitempty"`
	Want     []problemJSON   `json:"want"`
	SendBack string          `json:"send_back"`
}

type scoredJSON struct {
	Score   float64  `json:"score"`
	Band    string   `json:"band"`
	Queries []string `json:"queries"`
}

type signalsJSON struct {
	Kinds       []string `json:"kinds"`
	Component   bool     `json:"component"`
	OnsetRead   bool     `json:"onset_read"`
	OnsetFact   bool     `json:"onset_fact"`
	TriggerRead bool     `json:"trigger_read"`
	RuledOut    bool     `json:"ruled_out"`
}

func signalsOfJSON(s findings.Signals) signalsJSON {
	kinds := s.Kinds
	if kinds == nil {
		kinds = []string{}
	}
	return signalsJSON{kinds, s.Component, s.OnsetRead, s.OnsetFact, s.TriggerRead, s.RuledOut}
}

type signalWant struct {
	Signals []signalsJSON `json:"signals"`
	Scored  []scoredJSON  `json:"scored"`
}

type signalCase struct {
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input"`
	Want  *signalWant     `json:"want"`
}

type recordJSON struct {
	Reads      [][]float64 `json:"reads"`
	Names      []string    `json:"names"`
	Discovered []string    `json:"discovered"`
	Sourced    []string    `json:"sourced"`
	Ranges     []rangeJSON `json:"ranges"`
	Instants   []string    `json:"instants"`
	IDs        []string    `json:"ids"`
	Facts      []Fact      `json:"facts"`
	NextFact   int         `json:"next_fact"`
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func snapshot(r *Record) *recordJSON {
	out := &recordJSON{
		Reads: orEmpty(r.Reads), Names: orEmpty(r.Names), Discovered: orEmpty(r.Discovered), Sourced: orEmpty(r.Sourced),
		Ranges: []rangeJSON{}, Instants: []string{}, IDs: orEmpty(r.IDs), Facts: orEmpty(r.Facts), NextFact: r.NextFact,
	}
	for _, rng := range r.Ranges {
		out.Ranges = append(out.Ranges, rangeJSON{rng.From.Format(time.RFC3339Nano), rng.To.Format(time.RFC3339Nano)})
	}
	for _, at := range r.Instants {
		out.Instants = append(out.Instants, at.Format(time.RFC3339))
	}
	return out
}

type scenario struct {
	Name    string          `json:"name"`
	Steps   []stepJSON      `json:"steps"`
	Reviews []reviewCase    `json:"reviews,omitempty"`
	Signals []signalCase    `json:"signals,omitempty"`
	Renders []renderCase    `json:"renders,omitempty"`
	Record  json.RawMessage `json:"record,omitempty"`
}

func signalsOfInput(r *Record, input json.RawMessage) *signalWant {
	signals := r.CauseSignals(input)
	if signals == nil {
		return nil
	}
	out := &signalWant{Signals: []signalsJSON{}}
	for _, s := range signals {
		out.Signals = append(out.Signals, signalsOfJSON(s))
	}
	var in causesSaved
	if json.Unmarshal(input, &in) != nil || len(in.Causes) == 0 {
		return out
	}
	s := findings.Structured{}
	for _, c := range in.Causes {
		var facts []string
		for _, id := range c.Evidence.Facts {
			if id = strings.TrimSpace(id); !slices.Contains(facts, id) {
				facts = append(facts, id)
			}
		}
		s.Causes = append(s.Causes, findings.Cause{Evidence: findings.CauseEvidence{Facts: facts}})
	}
	s.WithQueries(r.Ledger().QueryOf)
	out.Scored = []scoredJSON{}
	for i, sig := range signals {
		score := findings.Score(sig)
		out.Scored = append(out.Scored, scoredJSON{Score: score, Band: findings.BandOf(score), Queries: orEmpty(s.Causes[i].Evidence.Queries)})
	}
	return out
}

func TestScenarioFixtures(t *testing.T) {
	var scenarios []scenario
	readFixture(t, "scenarios.json", &scenarios)
	if len(scenarios) == 0 {
		t.Fatal("the scenarios are empty")
	}
	for si := range scenarios {
		sc := &scenarios[si]
		r := NewRecord(Enforce)
		for _, step := range sc.Steps {
			switch {
			case step.Read != nil:
				r.AddRead([]byte(step.Read.Content), step.Read.Names, rangesFrom(t, step.Read.Ranges))
			case step.Issue != nil:
				got := string(r.IssueFacts(step.Issue.Tool, []byte(step.Issue.Input), []byte(step.Issue.Content), rangesFrom(t, step.Issue.Ranges)))
				if *updateFixtures {
					step.Issue.Want = got
				} else if got != step.Issue.Want {
					t.Errorf("%s: IssueFacts(%s) =\n%s\nwant\n%s", sc.Name, step.Issue.Tool, got, step.Issue.Want)
				}
			case step.Sourced != nil:
				r.AddSourced([]byte(step.Sourced.Content))
			default:
				t.Fatalf("%s: a step with nothing to do", sc.Name)
			}
		}
		for i, c := range sc.Reviews {
			problems := r.Review(c.Input, c.Question)
			got, sendBack := problemsJSON(problems), ""
			if len(problems) > 0 {
				sendBack = SendBackProblems(problems)
			}
			if *updateFixtures {
				sc.Reviews[i].Want, sc.Reviews[i].SendBack = got, sendBack
			} else {
				sameJSON(t, sc.Name+": review "+string(c.Input), got, c.Want)
				if sendBack != c.SendBack {
					t.Errorf("%s: send-back =\n%s\nwant\n%s", sc.Name, sendBack, c.SendBack)
				}
			}
		}
		for i, c := range sc.Signals {
			got := signalsOfInput(r, c.Input)
			if *updateFixtures {
				sc.Signals[i].Want = got
			} else {
				sameJSON(t, sc.Name+": signals "+string(c.Input), got, c.Want)
			}
		}
		for i, c := range sc.Renders {
			got := renderOf(r.Ledger(), c.Text)
			if *updateFixtures {
				sc.Renders[i].Want = got
			} else {
				sameJSON(t, sc.Name+": render "+c.Text, got, c.Want)
			}
		}
		if len(sc.Record) > 0 {
			got := snapshot(r)
			if *updateFixtures {
				raw, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				sc.Record = raw
			} else {
				var want recordJSON
				if err := json.Unmarshal(sc.Record, &want); err != nil {
					t.Fatal(err)
				}
				sameJSON(t, sc.Name+": record", got, want)
			}
		}
	}
	if *updateFixtures {
		writeFixture(t, "scenarios.json", scenarios)
	}
}
