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

var investigateSystem = strings.TrimRight(investigate, "\n") + "\n\n" + strings.TrimRight(Scope(), "\n") + "\n\n# Authoring guide\n" + strings.TrimRight(SpecGuide(), "\n") + "\n"

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

func Investigate() string { return investigateSystem }

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
