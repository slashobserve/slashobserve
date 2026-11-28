package prompts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/examples"
)

//go:embed investigate.md
var investigate string

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

func playbook() string {
	parts := []string{fullPlaybook, incidentPlaybook, measurePlaybook, logsPlaybook, tracesPlaybook, inventoryPlaybook, explainPlaybook}
	for i, part := range parts {
		parts[i] = trimmed(part)
	}
	return strings.Join(parts, "\n\n")
}

func trimmed(s string) string { return strings.TrimRight(s, "\n") }

func Investigate() string {
	body := trimmed(investigate) + "\n\n" + playbook() + "\n"
	return body + "\n" + trimmed(Scope()) + "\n\n# Authoring guide\n" + trimmed(GuideFor(allTypes)) + "\n"
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
