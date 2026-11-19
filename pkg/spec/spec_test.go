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

func TestParseRefusesTheNULCharacterInAValueAndPropertyName(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	title := strings.Replace(minimal, `"title": "`, `"title": "a\u0000b`, 1)
	if title == minimal {
		t.Fatal("the example has no title to put the character in")
	}

	issues := parseIssues(t, []byte(title))

	if len(issues) != 1 || issues[0].Path != "/title" || !strings.Contains(issues[0].Message, "NUL") {
		t.Fatalf("issues = %+v, want one NUL issue at /title", issues)
	}
}

func TestParseAnswersANumberTooLargeForAFilterAsAValidationIssue(t *testing.T) {
	minimal := string(readExample(t, "minimal.json"))
	doc := strings.Replace(minimal, `"datasource": "ch",`, `"datasource": "ch", "transform": [{"type": "filterByValue", "filters": [{"field": "n", "op": "gt", "value": 1e999}]}],`, 1)
	if doc == minimal {
		t.Fatal("the example has no datasource to put the transform after")
	}

	issues := parseIssues(t, []byte(doc))

	if len(issues) != 1 || !strings.HasPrefix(issues[0].Path, "/panels/0/transform/0/filters/0") {
		t.Fatalf("issues = %+v, want one at the filter", issues)
	}
}
