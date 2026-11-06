package prompts

import (
	_ "embed"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

//go:embed spec_guide.md
var specGuide string

//go:embed guide/prometheus.md
var prometheusGuide string

//go:embed guide/clickhouse.md
var clickhouseGuide string

//go:embed guide/loki.md
var lokiGuide string

//go:embed guide/tempo.md
var tempoGuide string

//go:embed system.md
var system string

//go:embed scope.md
var scope string

var sections = []struct {
	datasource spec.DatasourceType
	text       string
}{{spec.Prometheus, prometheusGuide}, {spec.ClickHouse, clickhouseGuide}, {spec.Loki, lokiGuide}, {spec.Tempo, tempoGuide}}

func GuideFor(types []spec.DatasourceType) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(specGuide, "\n"))
	for _, s := range sections {
		if slices.Contains(types, s.datasource) {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimRight(s.text, "\n"))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func SpecGuide() string {
	return GuideFor([]spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo})
}

func System() string { return system }

func Scope() string { return scope }
