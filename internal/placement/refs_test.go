package placement

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestSnapshotRefsKeepTheStoredSourceDatasources(t *testing.T) {
	got, err := SnapshotRefs([]byte(`{"type":"timeseries","datasource":"panel"}`), []byte(`[{"query":"","datasource":"query"}]`), []string{"from-variable", "panel"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"panel", "query", "from-variable"}; !slices.Equal(got, want) {
		t.Fatalf("refs = %v, want %v", got, want)
	}
}

func TestPanelRefsFollowTheBlockVariables(t *testing.T) {
	vars := []spec.Variable{{Name: "ds", Type: spec.DatasourceVariable, Default: &spec.Selection{Values: []string{"payroll"}}}}
	got, err := PanelRefs([]byte(`{"type":"timeseries","datasource":"${ds}","queries":[{"query":"up"}]}`), vars)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"payroll"}; !slices.Equal(got, want) {
		t.Fatalf("refs = %v, want %v", got, want)
	}
}

func TestSpecRefsRefuseADocumentThatIsNotJSON(t *testing.T) {
	if _, err := SpecRefs([]byte(`{`)); err == nil {
		t.Fatal("SpecRefs took a broken document")
	}
}
