package prompts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const goldenDir = "../../../spec/examples/agent/prompts"

var updateGoldens = flag.Bool("update", false, "rewrite spec/examples/agent/prompts")

var goldenGuides = []struct {
	name  string
	types []spec.DatasourceType
}{
	{"none", nil},
	{"prometheus", []spec.DatasourceType{spec.Prometheus}},
	{"builtin-logs-traces", []spec.DatasourceType{spec.Loki, spec.Tempo, AuditTrail}},
	{"all", allTypes},
}

type textHash struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func hashOf(text string) textHash {
	sum := sha256.Sum256([]byte(text))
	return textHash{Bytes: len(text), SHA256: hex.EncodeToString(sum[:])}
}

type investigateGolden struct {
	Task   route.Task `json:"task"`
	Facts  bool       `json:"facts"`
	Ledger bool       `json:"ledger"`
	Guide  string     `json:"guide"`
	textHash
}

type buildGolden struct {
	Guide string `json:"guide"`
	textHash
}

type goldenCases struct {
	Investigate []investigateGolden `json:"investigate"`
	Build       []buildGolden       `json:"build"`
}

func guideFile(name string) string { return "guide-" + name + ".md" }

func runTextGoldens(t *testing.T) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	var cases goldenCases
	for _, g := range goldenGuides {
		files[guideFile(g.name)] = []byte(GuideFor(g.types))
		for _, task := range append([]route.Task{route.Full}, route.Tasks...) {
			for _, v := range []Variant{{Task: task}, {Task: task, Ledger: true}, {Task: task, Facts: true}, {Task: task, Facts: true, Ledger: true}} {
				cases.Investigate = append(cases.Investigate, investigateGolden{Task: task, Facts: v.Facts, Ledger: v.Ledger, Guide: guideFile(g.name), textHash: hashOf(InvestigateFor(v, g.types))})
			}
		}
		cases.Build = append(cases.Build, buildGolden{Guide: guideFile(g.name), textHash: hashOf(Build(g.types))})
	}
	body, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files["cases.json"] = append(body, '\n')
	prometheus := []spec.DatasourceType{spec.Prometheus}
	files["investigate-full-facts-ledger-prometheus.md"] = []byte(InvestigateFor(Variant{Facts: true, Ledger: true}, prometheus))
	files["build-prometheus.md"] = []byte(Build(prometheus))
	return files
}

func TestRunTextGoldensAreCurrent(t *testing.T) {
	files := runTextGoldens(t)
	if *updateGoldens {
		if err := os.RemoveAll(goldenDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(goldenDir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if *updateGoldens {
		for name, body := range files {
			if err := root.WriteFile(name, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(files) {
		t.Errorf("%s holds %d files, want %d", goldenDir, len(entries), len(files))
	}
	for name, want := range files {
		got, err := root.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run go test ./internal/agent/prompts -run TestRunTextGoldensAreCurrent -update", name)
		}
	}
}
