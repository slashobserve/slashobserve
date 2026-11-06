package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/llm/replay"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	toolCreateDashboard = "create_dashboard"
	toolUpdatePanels    = "update_panels"
	toolImportGrafana   = "import_grafana_dashboard"

	fixtureSuffix = ".json"

	firstRowHeight = 1
)

var (
	prepareOutput   io.Writer = os.Stdout
	e2eFixtureDir             = filepath.Join("e2e", "fixtures", "llm")
	evalsFixtureDir           = filepath.Join("evals", "fixtures", "llm")
)

type replayFixture struct {
	Prompt           string       `json:"prompt"`
	Turns            []replayTurn `json:"turns"`
	ResumeAfterSaves bool         `json:"resume_after_saves,omitempty"`
}

type replayTurn struct {
	ToolCalls []replayCall `json:"tool_calls"`
}

type replayCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type panelsInput struct {
	Summary string       `json:"summary"`
	Upsert  []spec.Panel `json:"upsert"`
	Row     *rowInput    `json:"row,omitempty"`
	Finish  bool         `json:"finish,omitempty"`
}

type rowInput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type createInput struct {
	Summary       string          `json:"summary"`
	Spec          *spec.Dashboard `json:"spec"`
	PlannedPanels int             `json:"planned_panels"`
}

type importInput struct {
	JSON string `json:"json"`
}

type replaySummary struct {
	Generated int
	Copied    int
	Skipped   []string
}

func newFixture(prompt, tool string, input any) (*replayFixture, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal the %s input: %w", tool, err)
	}
	return &replayFixture{Prompt: prompt, Turns: []replayTurn{{ToolCalls: []replayCall{{Name: tool, Input: body}}}}}, nil
}

func (f *replayFixture) inputBytes() int {
	n := 0
	for _, turn := range f.Turns {
		for _, call := range turn.ToolCalls {
			n += len(call.Input)
		}
	}
	return n
}

func generateReplay(out string, maxTokens int) error {
	p, err := planCorpus(corpusDir, nil, suiteReplay)
	if err != nil {
		return err
	}
	return prepareReplay(out, p, maxTokens)
}

func prepareReplay(out string, p *planner, maxTokens int) error {
	summary, err := writeReplay(out, p, maxTokens)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(prepareOutput, "replay fixtures: %d generated, %d copied, %d tasks skipped\n",
		summary.Generated, summary.Copied, len(summary.Skipped))
	return err
}

func writeReplay(out string, p *planner, maxTokens int) (replaySummary, error) {
	if err := clearFixtures(out); err != nil {
		return replaySummary{}, err
	}
	copied, err := copyFixtures(out)
	if err != nil {
		return replaySummary{}, err
	}
	summary := replaySummary{Copied: copied, Skipped: p.skipped}
	for _, t := range p.tasks {
		if t.fixture == nil {
			continue
		}
		body, err := json.Marshal(t.fixture)
		if err != nil {
			return replaySummary{}, fmt.Errorf("marshal the fixture of %s: %w", t.scenario.ID, err)
		}
		if err := writeShared(filepath.Join(out, t.scenario.ID+fixtureSuffix), body); err != nil {
			return replaySummary{}, err
		}
		summary.Generated++
	}
	renewal, err := renewalFixture()
	if err != nil {
		return replaySummary{}, err
	}
	body, err := json.Marshal(renewal)
	if err != nil {
		return replaySummary{}, fmt.Errorf("marshal the renewal fixture: %w", err)
	}
	if err := writeShared(filepath.Join(out, "build-nef-renewal"+fixtureSuffix), body); err != nil {
		return replaySummary{}, err
	}
	summary.Generated++
	if _, err := replay.New(out, maxTokens, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		return replaySummary{}, fmt.Errorf("the replay provider rejects the fixtures in %s: %w", out, err)
	}
	return summary, nil
}

func clearFixtures(dir string) error {
	if err := os.MkdirAll(dir, exposedDirPerm); err != nil {
		return err
	}
	stale, err := filepath.Glob(filepath.Join(dir, "*"+fixtureSuffix))
	if err != nil {
		return err
	}
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func copyFixtures(out string) (int, error) {
	copied := 0
	for _, dir := range []string{e2eFixtureDir, evalsFixtureDir} {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) && dir == evalsFixtureDir {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), fixtureSuffix) {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // the directory is a fixed path inside the repository
			if err != nil {
				return 0, err
			}
			if err := writeShared(filepath.Join(out, entry.Name()), content); err != nil {
				return 0, err
			}
			copied++
		}
	}
	return copied, nil
}

type rebuildShape struct {
	panels int
	kinds  []spec.DatasourceType
}

type rebuildPanel struct {
	panel spec.Panel
	row   *spec.Panel
}

func rebuildFixture(a analyzed) (*replayFixture, rebuildShape, error) {
	usable := usableRebuildPanels(a.dash)
	shape := rebuildShape{panels: len(usable)}
	if len(usable) == 0 {
		return nil, shape, nil
	}
	kinds := map[spec.DatasourceType]bool{}
	fixture := &replayFixture{ResumeAfterSaves: true}
	for i, u := range usable {
		for _, q := range u.panel.Queries {
			kinds[spec.DatasourceType(stackDatasources[q.Datasource])] = true
		}
		tool, input := rebuildCall(a, usable, i)
		body, err := json.Marshal(input)
		if err != nil {
			return nil, shape, fmt.Errorf("marshal the %s input: %w", tool, err)
		}
		fixture.Turns = append(fixture.Turns, replayTurn{ToolCalls: []replayCall{{Name: tool, Input: body}}})
	}
	shape.kinds = slices.Sorted(maps.Keys(kinds))
	return fixture, shape, nil
}

func rebuildCall(a analyzed, usable []rebuildPanel, i int) (string, any) {
	u := usable[i]
	if i == 0 {
		return toolCreateDashboard, createInput{Summary: "Start " + a.dash.Title, Spec: firstSave(a.dash, u), PlannedPanels: len(usable)}
	}
	save := panelsInput{Summary: "Add " + u.panel.Title, Upsert: []spec.Panel{u.panel}, Finish: i == len(usable)-1}
	if u.row != nil {
		save.Row = &rowInput{ID: u.row.ID, Title: rowTitle(u.row)}
	}
	return toolUpdatePanels, save
}

func firstSave(dash *spec.Dashboard, first rebuildPanel) *spec.Dashboard {
	doc := *dash
	doc.Variables, doc.Links = nil, nil
	panel := first.panel
	if first.row == nil {
		doc.Panels = []spec.Panel{panel}
		return &doc
	}
	panel.GridPos.Y = firstRowHeight
	doc.Panels = []spec.Panel{{
		ID: first.row.ID, Type: spec.Row, Title: rowTitle(first.row),
		GridPos: spec.GridPos{W: spec.GridColumns, H: firstRowHeight},
		Panels:  []spec.Panel{panel},
	}}
	return &doc
}

func usableRebuildPanels(dash *spec.Dashboard) []rebuildPanel {
	var out []rebuildPanel
	for _, entry := range dash.DataPanels() {
		panel, ok := healedPanel(dash, entry.Panel)
		if ok && parsesAlone(dash, panel) {
			out = append(out, rebuildPanel{panel: panel, row: entry.Row})
		}
	}
	return out
}

func healedPanel(dash *spec.Dashboard, source *spec.Panel) (spec.Panel, bool) {
	if source.Type == spec.Canvas || hasActions(source) {
		return spec.Panel{}, false
	}
	panel := *source
	panel.Queries, panel.Datasource, panel.Interval = nil, "", ""
	panel.GridPos = spec.GridPos{W: source.GridPos.W, H: source.GridPos.H}
	for _, q := range source.Queries {
		kind, ok := datasourceKind(dash, cmp.Or(q.Datasource, source.Datasource))
		uid := uidOf(kind)
		if blank(q.Query) || !ok || uid == "" {
			continue
		}
		healthy := healthyQuery(kind, q)
		healthy.Datasource = uid
		panel.Queries = append(panel.Queries, healthy)
		panel.Datasource = cmp.Or(panel.Datasource, uid)
	}
	return panel, len(panel.Queries) > 0
}

func uidOf(kind spec.DatasourceType) string {
	for uid, t := range stackDatasources {
		if t == string(kind) {
			return uid
		}
	}
	return ""
}

func hasActions(p *spec.Panel) bool {
	return len(p.Actions) > 0 || slices.ContainsFunc(p.Overrides, func(o spec.Override) bool { return len(o.Actions) > 0 })
}

func parsesAlone(dash *spec.Dashboard, panel spec.Panel) bool {
	probe := *dash
	probe.Variables, probe.Links, probe.Panels = nil, nil, []spec.Panel{panel}
	body, err := json.Marshal(probe)
	if err != nil {
		return false
	}
	_, err = spec.Parse(body)
	return err == nil
}

const (
	renewalPrompt     = "eval build: a long node exporter dashboard whose history outgrows one session"
	renewalPanels     = 136
	renewalPadding    = 2000
	renewalSummaryPad = 500
)

func renewalFixture() (*replayFixture, error) {
	fixture := &replayFixture{Prompt: renewalPrompt, ResumeAfterSaves: true}
	for i := range renewalPanels {
		panel := map[string]any{
			"id":          fmt.Sprintf("pad-%03d", i),
			"type":        "stat",
			"title":       fmt.Sprintf("Node padding %03d", i),
			"description": padding(renewalPadding, i),
			"gridPos":     map[string]int{"x": 0, "y": 0, "w": 6, "h": 4},
			"datasource":  "prom",
			"queries":     []map[string]any{{"query": healthyPromQuery, "instant": true}},
		}
		summary := padding(renewalSummaryPad, i)
		tool, input := toolUpdatePanels, any(map[string]any{"summary": summary, "upsert": []any{panel}, "finish": i == renewalPanels-1})
		if i == 0 {
			tool, input = toolCreateDashboard, map[string]any{
				"summary":        summary,
				"planned_panels": renewalPanels,
				"spec": map[string]any{
					"specVersion": "0",
					"title":       "Node Exporter Full",
					"time":        map[string]string{"from": "now-1h", "to": "now"},
					"panels":      []any{panel},
				},
			}
		}
		body, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("marshal the %s input: %w", tool, err)
		}
		fixture.Turns = append(fixture.Turns, replayTurn{ToolCalls: []replayCall{{Name: tool, Input: body}}})
	}
	return fixture, nil
}

func padding(n, seed int) string {
	word := fmt.Sprintf("node%03d ", seed)
	return strings.TrimSpace(strings.Repeat(word, n/len(word)+1)[:n])
}
