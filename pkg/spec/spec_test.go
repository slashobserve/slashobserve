package spec_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDatasourceVariableTypes(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	with := func(dsType string) []byte {
		variable := fmt.Sprintf(`{"name":"source","type":"datasource","datasourceType":%q}`, dsType)
		return []byte(strings.Replace(minimal, `"panels"`, `"variables": [`+variable+`], "panels"`, 1))
	}
	for _, typ := range []spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo} {
		t.Run(string(typ), func(t *testing.T) {
			d, err := spec.Parse(with(string(typ)))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := d.Variables[0].DatasourceType; got != typ {
				t.Fatalf("datasourceType = %q, want %q", got, typ)
			}
		})
	}
	for _, typ := range []string{"graphite", "Loki", ""} {
		t.Run("refuses "+typ, func(t *testing.T) {
			issues := parseIssues(t, with(typ))
			if len(issues) == 0 || issues[0].Path != "/variables/0/datasourceType" {
				t.Fatalf("issues = %+v, want one at /variables/0/datasourceType", issues)
			}
		})
	}
}
