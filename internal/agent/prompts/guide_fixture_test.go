package prompts_test

import (
	"flag"
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var updateGuideFixture = flag.Bool("update", false, "rewrite the harness's copy of the Prometheus guide")

const harnessGuideFixture = "../../../harness/test/fixtures/prompts/guide-prometheus.md"

func TestTheHarnessGuideFixtureIsTheServedGuide(t *testing.T) {
	want := prompts.GuideFor([]spec.DatasourceType{spec.Prometheus})
	if *updateGuideFixture {
		if err := os.WriteFile(harnessGuideFixture, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(harnessGuideFixture)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s differs from prompts.GuideFor(prometheus); run go test ./internal/agent/prompts -run HarnessGuide -update, then UPDATE_GOLDENS=1 bun test in harness/", harnessGuideFixture)
	}
}
