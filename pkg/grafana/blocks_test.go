package grafana

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

var blocksTestOptions = Options{Datasources: []Datasource{{UID: "prom", Type: "prometheus"}}}

func blocksOf(t *testing.T, doc string) []Block {
	t.Helper()
	res, err := Convert([]byte(doc), blocksTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	return res.Report.Blocks
}

func blockAt(blocks []Block, path string) *Block {
	for i := range blocks {
		if blocks[i].Path == path {
			return &blocks[i]
		}
		if found := blockAt(blocks[i].Blocks, path); found != nil {
			return found
		}
	}
	return nil
}

func lineAt(b *Block, path string) *OptionLine {
	if b == nil {
		return nil
	}
	for i := range b.Options {
		if b.Options[i].Path == path {
			return &b.Options[i]
		}
	}
	return nil
}

func requireLine(t *testing.T, blocks []Block, blockPath, optionPath string, outcome Outcome) *OptionLine {
	t.Helper()
	b := blockAt(blocks, blockPath)
	if b == nil {
		t.Fatalf("no block at %q", blockPath)
	}
	line := lineAt(b, optionPath)
	if line == nil {
		t.Fatalf("block %q has no option %q: %+v", blockPath, optionPath, b.Options)
	}
	if line.Outcome != outcome {
		t.Fatalf("block %q option %q is %s, want %s", blockPath, optionPath, line.Outcome, outcome)
	}
	return line
}

const fullBlocksDashboard = `{"title":"Shop","uid":"abc","schemaVersion":39,"panels":[
	{"id":1,"type":"timeseries","title":"Requests","gridPos":{"x":0,"y":0,"w":12,"h":8},
	 "pluginVersion":"11.0.0","datasource":{"type":"prometheus","uid":"prom"},
	 "options":{"mystery":1,"legend":{"showLegend":true}},
	 "targets":[{"refId":"A","expr":"up","legendFormat":"{{pod}}"}],
	 "transformations":[{"id":"reduce","options":{"reducers":["max"]}}],
	 "fieldConfig":{"defaults":{"unit":"short","custom":{"lineWidth":3,"fillOpacity":0,"gradientMode":"none"}},
	  "overrides":[{"matcher":{"id":"byName","options":"up"},"properties":[{"id":"unit","value":"percent"}]}]}},
	{"id":2,"type":"annolist","title":"Notes","gridPos":{"x":0,"y":8,"w":12,"h":8}},
	{"id":3,"type":"row","title":"More","collapsed":true,"gridPos":{"x":0,"y":16,"w":24,"h":1},"panels":[
	 {"id":4,"type":"stat","title":"Up","gridPos":{"x":0,"y":17,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},
	  "targets":[{"refId":"A","expr":"up"}]}]}]}`

func TestBlocksNestDashboardPanelAndItsParts(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)

	if blocks[0].Kind != blockDashboard {
		t.Fatalf("first block is %s, want the dashboard", blocks[0].Kind)
	}
	if line := requireLine(t, blocks, "", "title", outcomeMapped); !slices.Equal(line.Spec, []string{"title"}) {
		t.Fatalf("dashboard title maps to %v", line.Spec)
	}

	panel := blockAt(blocks, "panels[0]")
	var kinds []BlockKind
	for _, child := range panel.Blocks {
		kinds = append(kinds, child.Kind)
	}
	want := []BlockKind{blockQuery, blockTransformation, blockFieldConfig, blockOverride}
	if !slices.Equal(kinds, want) {
		t.Fatalf("panel children are %v, want %v", kinds, want)
	}

	cases := []struct {
		block, option string
		spec          string
	}{
		{"panels[0].targets[0]", "legendFormat", "legend"},
		{"panels[0].targets[0]", "expr", "query"},
		{"panels[0].fieldConfig.defaults", "unit", "unit"},
		{"panels[0].fieldConfig.defaults", "custom.lineWidth", "lineWidth"},
		{"panels[0].fieldConfig.overrides[0]", "properties[0]", "unit"},
	}
	for _, c := range cases {
		line := requireLine(t, blocks, c.block, c.option, outcomeMapped)
		if !slices.Contains(line.Spec, c.spec) {
			t.Errorf("%s %s maps to %v, want it to name %q", c.block, c.option, line.Spec, c.spec)
		}
	}
}

func TestBlocksReportAnUnknownOptionAndRecordTheRest(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)

	line := requireLine(t, blocks, "panels[0]", "options.mystery", outcomeReported)
	if line.Reason == nil || *line.Reason == "" {
		t.Fatal("a reported option carries a reason")
	}
	requireLine(t, blocks, "panels[0]", "pluginVersion", outcomeBookkeeping)
	requireLine(t, blocks, "panels[0].fieldConfig.defaults", "custom.gradientMode", outcomeDefault)
	requireLine(t, blocks, "", "uid", outcomeBookkeeping)
}

func TestBlocksImplicitCountsTheImplicitLines(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)
	walkBlocks(blocks, func(b *Block) {
		implicit := 0
		for _, line := range b.Options {
			if line.Outcome.implicit() {
				implicit++
			}
		}
		if b.Implicit != implicit {
			t.Errorf("block %q counts %d implicit options but lists %d", b.Path, b.Implicit, implicit)
		}
	})
}

func TestBlocksHoldNoNilSlices(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)
	if blocks == nil {
		t.Fatal("blocks is nil")
	}
	walkBlocks(blocks, func(b *Block) {
		if b.Options == nil || b.Blocks == nil {
			t.Errorf("block %q has a nil slice", b.Path)
		}
		for _, line := range b.Options {
			if line.Spec == nil {
				t.Errorf("block %q option %q has a nil Spec", b.Path, line.Path)
			}
		}
	})
}

func TestBlocksMarkAnExcludedPanelWithoutOptions(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)
	b := blockAt(blocks, "panels[1]")
	if b == nil || b.Outcome == nil || *b.Outcome != outcomeExcluded {
		t.Fatalf("annolist block = %+v, want an excluded outcome", b)
	}
	if b.Reason == nil || !isDecisionReason(*b.Reason) {
		t.Fatalf("annolist reason = %v, want a decision", b.Reason)
	}
	if len(b.Options) != 0 || len(b.Blocks) != 0 {
		t.Fatalf("a placeholder block holds %d options and %d blocks", len(b.Options), len(b.Blocks))
	}
}

func TestBlocksPutACollapsedRowsChildrenUnderTheRow(t *testing.T) {
	blocks := blocksOf(t, fullBlocksDashboard)
	row := blockAt(blocks, "panels[2]")
	if row == nil || row.Kind != blockRow {
		t.Fatalf("row block = %+v", row)
	}
	if len(row.Blocks) != 1 || row.Blocks[0].Kind != blockPanel || row.Blocks[0].Path != "panels[2].panels[0]" {
		t.Fatalf("row children = %+v", row.Blocks)
	}
	requireLine(t, blocks, "panels[2].panels[0].targets[0]", "expr", outcomeMapped)
}

func TestBlocksReportADroppedQueryOnItsQueryBlock(t *testing.T) {
	doc := `{"title":"T","schemaVersion":39,"panels":[
	{"id":1,"type":"timeseries","title":"P","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},
	 "targets":[{"refId":"A","expr":"up"},
	  {"refId":"B","datasource":{"type":"__expr__","uid":"__expr__"},"type":"classic_conditions","conditions":[]}]}]}`
	blocks := blocksOf(t, doc)
	dropped := blockAt(blocks, "panels[0].targets[1]")
	if dropped == nil {
		t.Fatal("the dropped query has no block")
	}
	if dropped.Outcome == nil || (*dropped.Outcome != outcomeReported && *dropped.Outcome != outcomeExcluded) {
		t.Fatalf("dropped query outcome = %v, want reported or excluded", dropped.Outcome)
	}
	if dropped.Reason == nil || *dropped.Reason == "" {
		t.Fatal("a dropped query carries its reason")
	}
	kept := blockAt(blocks, "panels[0].targets[0]")
	if kept == nil || kept.Outcome != nil {
		t.Fatalf("the kept query has an outcome: %+v", kept)
	}
}

func TestBlocksRecordASkippedFieldPropAsNoEffect(t *testing.T) {
	doc := `{"title":"T","schemaVersion":39,"panels":[
	{"id":1,"type":"stat","title":"P","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},
	 "targets":[{"refId":"A","expr":"up"}],
	 "fieldConfig":{"defaults":{"filterable":true},"overrides":[]}}]}`
	blocks := blocksOf(t, doc)
	requireLine(t, blocks, "panels[0].fieldConfig.defaults", "filterable", outcomeNoEffect)
}

func TestBlocksMapDashboardVariables(t *testing.T) {
	doc := `{"title":"T","schemaVersion":39,"templating":{"list":[
	 {"name":"env","type":"custom","query":"a,b","current":{"text":"a","value":"a"},"options":[],"mystery":true}]},
	 "panels":[{"id":1,"type":"stat","title":"P","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up"}]}]}`
	blocks := blocksOf(t, doc)
	v := blockAt(blocks, "templating.list[0]")
	if v == nil || v.Kind != blockVariable {
		t.Fatalf("variable block = %+v", v)
	}
	if v.Title != "env" {
		t.Fatalf("variable title = %q, want its name", v.Title)
	}
	requireLine(t, blocks, "templating.list[0]", "mystery", outcomeReported)
}

func TestBlocksNeverHoldGrafanaValues(t *testing.T) {
	doc := `{"title":"SECRET-TITLE","schemaVersion":39,"panels":[
	{"id":1,"type":"timeseries","title":"P","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},
	 "options":{"mystery":"SECRET-VALUE"},
	 "targets":[{"refId":"A","expr":"SECRET-QUERY(up)","unknownKey":"SECRET-TARGET"}]}]}`
	raw, err := json.Marshal(blocksOf(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET-QUERY") || strings.Contains(string(raw), "SECRET-VALUE") || strings.Contains(string(raw), "SECRET-TARGET") {
		t.Fatalf("a block holds a Grafana value: %s", raw)
	}
}

func lines(outcome Outcome, count int, prefix string) []OptionLine {
	out := make([]OptionLine, count)
	for i := range out {
		out[i] = OptionLine{Path: fmt.Sprintf("%s%d", prefix, i), Outcome: outcome, Spec: []string{}}
	}
	return out
}

func countOutcome(b *Block, o Outcome) int {
	n := 0
	for _, line := range b.Options {
		if line.Outcome == o {
			n++
		}
	}
	return n
}

func TestApplyLimitsCutsImplicitBeforeMappedBeforeReportedInABlock(t *testing.T) {
	b := Block{Blocks: []Block{}}
	b.Options = append(b.Options, lines(outcomeReported, 10, "r")...)
	b.Options = append(b.Options, lines(outcomeMapped, 100, "m")...)
	b.Options = append(b.Options, lines(outcomeDefault, 150, "d")...)
	b.Implicit = 150
	blocks := []Block{b}
	applyLimits(blocks)
	got := &blocks[0]
	if countOutcome(got, outcomeReported) != 10 || countOutcome(got, outcomeMapped) != 100 || countOutcome(got, outcomeDefault) != 90 {
		t.Fatalf("kept %d reported, %d mapped, %d default", countOutcome(got, outcomeReported), countOutcome(got, outcomeMapped), countOutcome(got, outcomeDefault))
	}
	if got.Omitted != 60 || got.Implicit != 150 {
		t.Fatalf("omitted %d implicit %d, want 60 and 150", got.Omitted, got.Implicit)
	}
}

func TestApplyLimitsBoundsReportedOptionsPerBlock(t *testing.T) {
	blocks := []Block{{Options: lines(outcomeReported, 250, "r"), Blocks: []Block{}}}
	applyLimits(blocks)
	if len(blocks[0].Options) != maxBlockOptions || blocks[0].Omitted != 50 {
		t.Fatalf("kept %d omitted %d, want 200 and 50", len(blocks[0].Options), blocks[0].Omitted)
	}
}

func manyBlocks(count int, options func(i int) []OptionLine) []Block {
	blocks := make([]Block, count)
	for i := range blocks {
		blocks[i] = Block{Path: fmt.Sprintf("panels[%d]", i), Options: options(i), Blocks: []Block{}}
	}
	return blocks
}

func totalLines(blocks []Block) int {
	n := 0
	walkBlocks(blocks, func(b *Block) { n += len(b.Options) })
	return n
}

func TestApplyLimitsKeepsTheTotalUnderTenThousandAndDropsImplicitFirst(t *testing.T) {
	blocks := manyBlocks(60, func(i int) []OptionLine {
		return append(lines(outcomeMapped, 100, "m"), lines(outcomeDefault, 100, "d")...)
	})
	applyLimits(blocks)
	if got := totalLines(blocks); got != maxReportOptions {
		t.Fatalf("kept %d lines, want %d", got, maxReportOptions)
	}
	mapped, omitted := 0, 0
	for i := range blocks {
		mapped += countOutcome(&blocks[i], outcomeMapped)
		omitted += blocks[i].Omitted
	}
	if mapped != 6000 {
		t.Fatalf("kept %d mapped, want all 6000", mapped)
	}
	if omitted != 2000 {
		t.Fatalf("omitted %d, want 2000 implicit", omitted)
	}
}

func TestApplyLimitsKeepsEveryReportedAndExcludedLineWithinTheirBlockBound(t *testing.T) {
	blocks := manyBlocks(60, func(i int) []OptionLine {
		return append(lines(outcomeReported, 150, "r"), lines(outcomeMapped, 50, "m")...)
	})
	applyLimits(blocks)
	reported, mapped := 0, 0
	for i := range blocks {
		reported += countOutcome(&blocks[i], outcomeReported)
		mapped += countOutcome(&blocks[i], outcomeMapped)
	}
	if reported != 9000 {
		t.Fatalf("kept %d reported, want all 9000", reported)
	}
	if mapped != 1000 {
		t.Fatalf("kept %d mapped, want the 1000 that fit", mapped)
	}
}

func TestApplyLimitsCutsMappedWhenReportedAndExcludedAlreadyFillTheTotal(t *testing.T) {
	blocks := manyBlocks(60, func(i int) []OptionLine {
		return append(lines(outcomeExcluded, 100, "x"), lines(outcomeReported, 100, "r")...)
	})
	blocks = append(blocks, Block{Path: "panels[60]", Options: lines(outcomeMapped, 5, "m"), Blocks: []Block{}})
	applyLimits(blocks)
	if got := countOutcome(&blocks[60], outcomeMapped); got != 0 {
		t.Fatalf("kept %d mapped past a full total", got)
	}
	if blocks[60].Omitted != 5 {
		t.Fatalf("omitted %d, want 5", blocks[60].Omitted)
	}
}

func TestIssueLogPairsEveryPathWithItsReasons(t *testing.T) {
	log := newIssueLog(func(string) (decision, bool) { return decision{}, false })
	log.add("options.a", "first")
	log.add("options.a", "second")
	log.add("options.a", "first")
	log.add("options.b", "first")
	got := log.entries()
	if len(got) != 2 {
		t.Fatalf("entries = %+v, want 2 groups", got)
	}
	if got[0].path != "options.a" || !slices.Equal(got[0].reasons, []string{"first", "second"}) {
		t.Fatalf("first group = %+v", got[0])
	}
	if got[1].path != "options.b" || !slices.Equal(got[1].reasons, []string{"first"}) {
		t.Fatalf("second group = %+v", got[1])
	}
}

func TestIssueLogEntriesKeepPathsPastTheListedCap(t *testing.T) {
	log := newIssueLog(func(string) (decision, bool) { return decision{}, false })
	for i := 0; i < maxPanelPaths+20; i++ {
		log.add(fmt.Sprintf("options.o%d", i), "why")
	}
	paths, _ := log.report()
	if len(paths) > maxPanelPaths {
		t.Fatalf("listed %d paths, want at most %d", len(paths), maxPanelPaths)
	}
	if got := len(log.entries()); got != maxPanelPaths+20 {
		t.Fatalf("entries hold %d groups, want %d", got, maxPanelPaths+20)
	}
}

func TestIssueLogRollBackForgetsTheEntriesAfterTheMark(t *testing.T) {
	log := newIssueLog(func(string) (decision, bool) { return decision{}, false })
	log.add("options.keep", "why")
	mark := log.mark()
	log.add("options.undo", "why")
	log.add("options.keep", "other")
	log.rollBack(mark)
	got := log.entries()
	if len(got) != 1 || got[0].path != "options.keep" || !slices.Equal(got[0].reasons, []string{"why"}) {
		t.Fatalf("entries after roll back = %+v", got)
	}
	if log.covers("options.undo") {
		t.Fatal("a rolled back path still counts as covered")
	}
	if !log.covers("options.keep") {
		t.Fatal("a kept path stopped counting as covered")
	}
}

func TestIssueLogCoversTheDescendantsOfAnEnteredPathOnly(t *testing.T) {
	log := newIssueLog(func(string) (decision, bool) { return decision{}, false })
	log.add("panels[1]", "why")
	cases := map[string]bool{
		"panels[1]":          true,
		"panels[1].title":    true,
		"panels[1].a.b":      true,
		"panels[10]":         false,
		"panels[10].title":   false,
		"panels[2]":          false,
		"panels":             false,
		"panels[1]x":         false,
		"other.panels[1]":    false,
		"options.panels[1]":  false,
		"panels[11].panels":  false,
		"panels[1][0].value": true,
	}
	for path, want := range cases {
		if got := log.covers(path); got != want {
			t.Errorf("covers(%q) = %v, want %v", path, got, want)
		}
	}
}

func syntheticDashboard(panels int) string {
	var sb strings.Builder
	sb.WriteString(`{"title":"Big","schemaVersion":39,"panels":[`)
	for i := 0; i < panels; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":%d,"type":"timeseries","title":"P%d","gridPos":{"x":0,"y":%d,"w":12,"h":8},`, i+1, i, i*8)
		sb.WriteString(`"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up"}],"options":{`)
		for j := 0; j < 40; j++ {
			if j > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `"unknown%d":%d`, j, j)
		}
		sb.WriteString(`},"fieldConfig":{"defaults":{"custom":{"gradientMode":"none","axisPlacement":"auto","axisSoftMin":null,"drawStyle":"line","lineInterpolation":"linear","showPoints":"auto","spanNulls":false,"fillOpacity":0,"barAlignment":0,"scaleDistribution":{"type":"linear"},"stacking":{"mode":"none","group":"A"},"hideFrom":{"legend":false,"tooltip":false,"viz":false},"axisCenteredZero":false,"axisColorMode":"text","axisLabel":"","pointSize":5,"lineWidth":1}},"overrides":[]}}`)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

func TestBlocksOfAHugeDashboardStayWithinTheLimits(t *testing.T) {
	res, err := Convert([]byte(syntheticDashboard(200)), blocksTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	blocks := res.Report.Blocks
	listed := 0
	reported := 0
	omitted := 0
	walkBlocks(blocks, func(b *Block) {
		listed += len(b.Options)
		omitted += b.Omitted
		reported += countOutcome(b, outcomeReported)
		if len(b.Options) > maxBlockOptions {
			t.Errorf("block %q lists %d options", b.Path, len(b.Options))
		}
	})
	if reported != 8000 {
		t.Errorf("listed %d reported options, want all 8000", reported)
	}
	if listed != maxReportOptions {
		t.Errorf("listed %d options, want exactly %d", listed, maxReportOptions)
	}
	if omitted == 0 {
		t.Error("no option was omitted although the dashboard has more than the limit")
	}
	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 2<<20 {
		t.Errorf("blocks take %d bytes, want at most 2 MiB", len(raw))
	}
}
