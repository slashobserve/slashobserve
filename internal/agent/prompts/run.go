package prompts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

//go:embed investigate.md
var investigate string

//go:embed facts.md
var factsSection string

//go:embed ledger.md
var ledgerSection string

//go:embed playbook/measure.md
var measurePlaybook string

//go:embed playbook/logs.md
var logsPlaybook string

//go:embed playbook/traces.md
var tracesPlaybook string

//go:embed playbook/incident.md
var incidentPlaybook string

//go:embed playbook/inventory.md
var inventoryPlaybook string

//go:embed playbook/explain.md
var explainPlaybook string

//go:embed playbook/full.md
var fullPlaybook string

var playbooks = map[route.Task]string{
	route.Measure:   measurePlaybook,
	route.Logs:      logsPlaybook,
	route.Traces:    tracesPlaybook,
	route.Incident:  incidentPlaybook,
	route.Inventory: inventoryPlaybook,
	route.Explain:   explainPlaybook,
}

var enforcedRules = []string{
	"Give one value per quantity, copied at the precision the tool returned",
	"When two reads give different figures for it, make one more read that settles it",
}

type Variant struct {
	Task   route.Task
	Facts  bool
	Ledger bool
}

func playbookFor(task route.Task) string {
	if text, ok := playbooks[task]; ok {
		return trimmed(text)
	}
	parts := []string{trimmed(fullPlaybook)}
	for _, t := range []route.Task{route.Incident, route.Measure, route.Logs, route.Traces, route.Inventory, route.Explain} {
		parts = append(parts, trimmed(playbooks[t]))
	}
	return strings.Join(parts, "\n\n")
}

func trimmed(s string) string { return strings.TrimRight(s, "\n") }

func withoutEnforcedRules(body string) string {
	lines := strings.SplitAfter(body, "\n")
	lines = slices.DeleteFunc(lines, func(line string) bool {
		return slices.ContainsFunc(enforcedRules, func(rule string) bool { return strings.HasPrefix(line, rule) })
	})
	return strings.Join(lines, "")
}

func InvestigateBody(v Variant) string {
	core := investigate
	if v.Facts {
		core = withoutEnforcedRules(core)
	}
	parts := []string{trimmed(core), playbookFor(v.Task)}
	if v.Facts {
		parts = append(parts, trimmed(factsSection))
	}
	if v.Ledger {
		parts = append(parts, trimmed(ledgerSection))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func InvestigateFor(v Variant, types []spec.DatasourceType) string {
	return InvestigateBody(v) + "\n" + trimmed(Scope()) + "\n\n# Authoring guide\n" + trimmed(GuideFor(types)) + "\n"
}

func Build(types []spec.DatasourceType) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(System(), "\n"))
	b.WriteString("\n\n" + strings.TrimRight(Scope(), "\n"))
	b.WriteString("\n\n# Authoring guide\n")
	b.WriteString(strings.TrimRight(GuideFor(types), "\n"))
	b.WriteString("\n\n# Example Spec (format only)\n")
	writeJSONBlock(&b, compactJSON(examples.CheckoutServiceHealth))
	b.WriteString("\n")
	return b.String()
}

func Investigate() string { return InvestigateFor(Variant{}, allTypes) }

func InvestigateWithLedger() string { return InvestigateFor(Variant{Ledger: true}, allTypes) }

func InvestigateWithFacts(ledger bool) string {
	return InvestigateFor(Variant{Facts: true, Ledger: ledger}, allTypes)
}

func compactJSON(body []byte) []byte {
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		panic("prompts: embedded example does not compact: " + err.Error())
	}
	return compact.Bytes()
}

func writeJSONBlock(b *strings.Builder, body []byte) {
	b.WriteString("```json\n")
	b.WriteString(strings.TrimRight(string(body), "\n"))
	b.WriteString("\n```")
}
