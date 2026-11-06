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

//go:embed guide/search.md
var searchGuide string

//go:embed guide/tempo.md
var tempoGuide string

//go:embed system.md
var system string

//go:embed scope.md
var scope string

var sections = []struct {
	datasources []spec.DatasourceType
	text        string
}{
	{[]spec.DatasourceType{spec.Prometheus}, prometheusGuide},
	{[]spec.DatasourceType{spec.ClickHouse}, clickhouseGuide},
	{[]spec.DatasourceType{spec.Loki}, lokiGuide},
	{[]spec.DatasourceType{spec.Tempo}, tempoGuide},
	{[]spec.DatasourceType{spec.Elasticsearch, spec.OpenSearch}, searchGuide},
}

func GuideFor(types []spec.DatasourceType) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(specGuide, "\n"))
	for _, s := range sections {
		if slices.ContainsFunc(s.datasources, func(t spec.DatasourceType) bool { return slices.Contains(types, t) }) {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimRight(s.text, "\n"))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func SpecGuide() string {
	return GuideFor([]spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo, spec.Elasticsearch})
}

func System() string { return system }

func Scope() string { return scope }
