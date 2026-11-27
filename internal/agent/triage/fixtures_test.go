package triage

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixtureDir = "../../../spec/examples/agent/triage"

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

var updateFixtures = flag.Bool("update", false, "rewrite spec/examples/agent/triage")

func readFixture(t *testing.T, name string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name)) //nolint:gosec // fixture names are constants in this package
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
	if err := os.WriteFile(filepath.Join(fixtureDir, name), b.Bytes(), 0o600); err != nil {
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

type rowJSON struct {
	Service  string     `json:"service"`
	Signal   string     `json:"signal"`
	Rung     int64      `json:"rung"`
	Window   float64    `json:"window"`
	Baseline float64    `json:"baseline"`
	Unit     string     `json:"unit"`
	Source   string     `json:"source"`
	Basis    string     `json:"basis"`
	Moved    bool       `json:"moved"`
	Onset    *time.Time `json:"onset"`
}

type originJSON struct {
	Source    string   `json:"source"`
	Service   string   `json:"service"`
	Operation string   `json:"operation"`
	Message   string   `json:"message"`
	Traces    int      `json:"traces"`
	Share     float64  `json:"share"`
	Example   string   `json:"example"`
	RelayedBy []string `json:"relayed_by"`
}

type eventJSON struct {
	Kind    string    `json:"kind"`
	Source  string    `json:"source"`
	Service string    `json:"service"`
	At      time.Time `json:"at"`
	Text    string    `json:"text"`
}

type patternJSON struct {
	Source   string `json:"source"`
	Service  string `json:"service"`
	Level    string `json:"level"`
	Text     string `json:"text"`
	Window   int64  `json:"window"`
	Baseline int64  `json:"baseline"`
}

type findingsJSON struct {
	Rows     []rowJSON     `json:"rows"`
	Origins  []originJSON  `json:"origins"`
	Events   []eventJSON   `json:"events"`
	Patterns []patternJSON `json:"patterns"`
}

type candidateJSON struct {
	ID       string        `json:"id"`
	Service  string        `json:"service"`
	Onset    *time.Time    `json:"onset"`
	Rows     []rowJSON     `json:"rows"`
	KnockOn  []string      `json:"knock_on"`
	Origins  []originJSON  `json:"origins"`
	Events   []eventJSON   `json:"events"`
	Patterns []patternJSON `json:"patterns"`
}

type serviceJSON struct {
	Source   string   `json:"source"`
	Type     string   `json:"type"`
	Signal   string   `json:"signal"`
	Field    string   `json:"field"`
	Services []string `json:"services"`
}

type skipJSON struct {
	Step    string   `json:"step"`
	Source  string   `json:"source"`
	Reason  string   `json:"reason"`
	Signals []string `json:"signals"`
}

type windowJSON struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type reportJSON struct {
	End        time.Time       `json:"end"`
	Question   *windowJSON     `json:"question"`
	Services   []serviceJSON   `json:"services"`
	Signals    []string        `json:"signals"`
	Rates      []rowJSON       `json:"rates"`
	Candidates []candidateJSON `json:"candidates"`
	Background []rowJSON       `json:"background"`
	Skipped    []skipJSON      `json:"skipped"`
	Calls      int             `json:"calls"`
	DurationMS int64           `json:"duration_ms"`
}

func list[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func rowsJSON(rows []Row) []rowJSON {
	out := []rowJSON{}
	for _, r := range rows {
		out = append(out, rowJSON{r.Service, r.Signal, int64(r.Rung / time.Second), r.Window, r.Baseline, r.Unit, r.Source, r.Basis, r.Moved, utcPtr(r.Onset)})
	}
	return out
}

func rowsOf(in []rowJSON) []Row {
	var out []Row
	for _, r := range in {
		out = append(out, Row{r.Service, r.Signal, time.Duration(r.Rung) * time.Second, r.Window, r.Baseline, r.Unit, r.Source, r.Basis, r.Moved, r.Onset})
	}
	return out
}

func originsJSON(in []Origin) []originJSON {
	out := []originJSON{}
	for _, o := range in {
		out = append(out, originJSON{o.Source, o.Service, o.Operation, o.Message, o.Traces, o.Share, o.Example, list(o.RelayedBy)})
	}
	return out
}

func originsOfJSON(in []originJSON) []Origin {
	var out []Origin
	for _, o := range in {
		out = append(out, Origin(o))
	}
	return out
}

func eventsJSON(in []Event) []eventJSON {
	out := []eventJSON{}
	for _, e := range in {
		out = append(out, eventJSON{e.Kind, e.Source, e.Service, e.At.UTC(), e.Text})
	}
	return out
}

func eventsOf(in []eventJSON) []Event {
	var out []Event
	for _, e := range in {
		out = append(out, Event(e))
	}
	return out
}

func patternsJSON(in []Pattern) []patternJSON {
	out := []patternJSON{}
	for _, p := range in {
		out = append(out, patternJSON(p))
	}
	return out
}

func patternsOfJSON(in []patternJSON) []Pattern {
	var out []Pattern
	for _, p := range in {
		out = append(out, Pattern(p))
	}
	return out
}

func candidatesJSON(in []Candidate) []candidateJSON {
	out := []candidateJSON{}
	for _, c := range in {
		out = append(out, candidateJSON{c.ID, c.Service, utcPtr(c.Onset), rowsJSON(c.Rows), list(c.KnockOn), originsJSON(c.Origins), eventsJSON(c.Events), patternsJSON(c.Patterns)})
	}
	return out
}

func candidatesOf(in []candidateJSON) []Candidate {
	var out []Candidate
	for _, c := range in {
		out = append(out, Candidate{c.ID, c.Service, c.Onset, rowsOf(c.Rows), c.KnockOn, originsOfJSON(c.Origins), eventsOf(c.Events), patternsOfJSON(c.Patterns)})
	}
	return out
}

func reportOfJSON(r reportJSON) Report {
	out := Report{End: r.End, Signals: r.Signals, Rates: rowsOf(r.Rates), Candidates: candidatesOf(r.Candidates), Background: rowsOf(r.Background), Calls: r.Calls, Duration: time.Duration(r.DurationMS) * time.Millisecond}
	if r.Question != nil {
		out.Question = &Window{From: r.Question.From, To: r.Question.To}
	}
	for _, s := range r.Services {
		out.Services = append(out.Services, ServiceSource(s))
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, Skip(s))
	}
	return out
}

func reportJSONOf(r Report) reportJSON {
	out := reportJSON{End: r.End.UTC(), Signals: list(r.Signals), Rates: rowsJSON(r.Rates), Candidates: candidatesJSON(r.Candidates), Background: rowsJSON(r.Background), Calls: r.Calls, DurationMS: r.Duration.Milliseconds(), Services: []serviceJSON{}, Skipped: []skipJSON{}}
	if r.Question != nil {
		out.Question = &windowJSON{r.Question.From.UTC(), r.Question.To.UTC()}
	}
	for _, s := range r.Services {
		out.Services = append(out.Services, serviceJSON{s.Source, s.Type, s.Signal, s.Field, list(s.Services)})
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, skipJSON{s.Step, s.Source, s.Reason, list(s.Signals)})
	}
	return out
}

type onsetFixture struct {
	Start  time.Time `json:"start"`
	Onsets []struct {
		Name        string     `json:"name"`
		Values      []*float64 `json:"values"`
		WindowStart int        `json:"window_start"`
		Rule        struct {
			Factor  float64 `json:"factor"`
			Floor   float64 `json:"floor"`
			Sustain int     `json:"sustain"`
		} `json:"rule"`
		Want *int `json:"want"`
	} `json:"onsets"`
	Medians []struct {
		In   []float64 `json:"in"`
		Want float64   `json:"want"`
	} `json:"medians"`
}

func TestOnsetFixtures(t *testing.T) {
	var fx onsetFixture
	readFixture(t, "onset.json", &fx)
	if len(fx.Onsets) == 0 || len(fx.Medians) == 0 {
		t.Fatal("the onset fixtures are empty")
	}
	for i, c := range fx.Onsets {
		points := make([]Point, len(c.Values))
		for j, v := range c.Values {
			value := math.NaN()
			if v != nil {
				value = *v
			}
			points[j] = Point{At: fx.Start.Add(time.Duration(j) * time.Minute), Value: value}
		}
		at, ok := Onset(points, fx.Start.Add(time.Duration(c.WindowStart)*time.Minute), Rule{Factor: c.Rule.Factor, Floor: c.Rule.Floor, Sustain: c.Rule.Sustain})
		var got *int
		if ok {
			minutes := int(at.Sub(fx.Start) / time.Minute)
			got = &minutes
		}
		if *updateFixtures {
			fx.Onsets[i].Want = got
		} else {
			sameJSON(t, c.Name, got, c.Want)
		}
	}
	for i, c := range fx.Medians {
		if got := median(c.In); *updateFixtures {
			fx.Medians[i].Want = got
		} else if got != c.Want {
			t.Errorf("median(%v) = %v, want %v", c.In, got, c.Want)
		}
	}
	if *updateFixtures {
		writeFixture(t, "onset.json", fx)
	}
}

type clusterWant struct {
	Candidates []candidateJSON `json:"candidates"`
	Background []rowJSON       `json:"background"`
	Leads      []Lead          `json:"leads"`
}

type clusterCase struct {
	Name     string       `json:"name"`
	Findings findingsJSON `json:"findings"`
	Want     *clusterWant `json:"want"`
}

func TestClusterFixtures(t *testing.T) {
	var cases []clusterCase
	readFixture(t, "cluster.json", &cases)
	if len(cases) == 0 {
		t.Fatal("the cluster fixtures are empty")
	}
	for i, c := range cases {
		f := Findings{Rows: rowsOf(c.Findings.Rows), Origins: originsOfJSON(c.Findings.Origins), Events: eventsOf(c.Findings.Events), Patterns: patternsOfJSON(c.Findings.Patterns)}
		candidates, background := Cluster(f)
		got := &clusterWant{Candidates: candidatesJSON(candidates), Background: rowsJSON(background), Leads: list(Leads(candidates))}
		if *updateFixtures {
			cases[i].Want = got
		} else {
			sameJSON(t, c.Name, got, c.Want)
		}
	}
	if *updateFixtures {
		writeFixture(t, "cluster.json", cases)
	}
}

type renderCase struct {
	Name   string     `json:"name"`
	Report reportJSON `json:"report"`
	Budget int        `json:"budget"`
	Want   string     `json:"want"`
}

func TestRenderFixtures(t *testing.T) {
	var cases []renderCase
	if *updateFixtures {
		cases = renderCases()
	} else {
		readFixture(t, "render.json", &cases)
	}
	if len(cases) == 0 {
		t.Fatal("the render fixtures are empty")
	}
	for i, c := range cases {
		got := Render(reportOfJSON(c.Report), c.Budget)
		if *updateFixtures {
			cases[i].Want = got
		} else if got != c.Want {
			t.Errorf("%s: block =\n%s\nwant\n%s", c.Name, got, c.Want)
		}
	}
	if *updateFixtures {
		writeFixture(t, "render.json", cases)
	}
}
