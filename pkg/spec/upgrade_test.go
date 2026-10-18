package spec_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const upgradeDir = "testdata/upgrade"

func readUpgradeFile(t *testing.T, rel string) []byte {
	t.Helper()
	root, err := os.OpenRoot(upgradeDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func upgradeGoldenNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(upgradeDir, "legacy", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob legacy goldens: %v (%d files)", err, len(files))
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	return names
}

func decodeJSON(t *testing.T, data []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

func TestUpgradeGolden(t *testing.T) {
	for _, name := range upgradeGoldenNames(t) {
		t.Run(name, func(t *testing.T) {
			out, changed, err := spec.Upgrade(readUpgradeFile(t, filepath.Join("legacy", name)))
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				t.Fatal("changed = false, want true")
			}
			want := decodeJSON(t, readUpgradeFile(t, filepath.Join("evolved", name)))
			if got := decodeJSON(t, out); !reflect.DeepEqual(got, want) {
				t.Fatalf("upgraded document differs from golden\ngot:  %s\nwant: %v", out, want)
			}
			again, changed, err := spec.Upgrade(out)
			if err != nil {
				t.Fatal(err)
			}
			if changed || !bytes.Equal(again, out) {
				t.Fatalf("upgrading the output again changed it\ngot:  %s\nwant: %s", again, out)
			}
		})
	}
}

func TestUpgradeIsIdempotent(t *testing.T) {
	for _, name := range upgradeGoldenNames(t) {
		t.Run(name, func(t *testing.T) {
			in := readUpgradeFile(t, filepath.Join("evolved", name))
			out, changed, err := spec.Upgrade(in)
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				t.Fatal("changed = true, want false")
			}
			if !bytes.Equal(out, in) {
				t.Fatalf("output differs from input\ngot:  %s\nwant: %s", out, in)
			}
		})
	}
}

func TestUpgradeKeepsNumbersAndUnknownMembers(t *testing.T) {
	in := []byte(`{"specVersion":"0","title":"T","timeRange":{"from":"now-1h","to":"now"},
"panels":[{"id":"p1","type":"timeseries","title":"A","gridPos":{"x":0,"y":0,"w":12,"h":8},
"datasource":{"type":"prometheus","uid":"prom"},"query":"rate(up[5m]) > 0 && 1",
"min":0.1000,"decimals":2,"y":10000,"bogus":{"a":[1,2]}}]}`)
	out, changed, err := spec.Upgrade(in)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	for _, want := range []string{"0.1000", `"decimals":2`, `"y":10000`, `"bogus":{"a":[1,2]}`, `rate(up[5m]) > 0 && 1`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output lacks %s: %s", want, out)
		}
	}
}

func TestUpgradeRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"not an object", `[]`, "not a JSON object"},
		{"malformed", `{"a":`, "spec: upgrade:"},
		{"trailing data", `{} {}`, "unexpected data after"},
		{"both time fields", `{"timeRange":{},"time":{}}`, `both "timeRange" and "time"`},
		{"both query fields", `{"panels":[{"query":"up","queries":[]}]}`, `both "query" and "queries"`},
		{"legend without query", `{"panels":[{"legend":"x"}]}`, `"legend" without "query"`},
		{"datasource without uid", `{"panels":[{"datasource":{"type":"prometheus"},"query":"up"}]}`, "panels/0/datasource has no string uid"},
		{"variable datasource without uid", `{"variables":[{"name":"a","datasource":{"type":"prometheus"}}]}`, "variables/0/datasource has no string uid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := spec.Upgrade([]byte(tc.in))
			if err == nil {
				t.Fatalf("err = nil, out = %s", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to contain %q", err, tc.want)
			}
			if changed || out != nil {
				t.Fatalf("changed = %v, out = %s, want false and nil on error", changed, out)
			}
		})
	}
}

func TestUpgradeLegendIsCopiedOnlyWhenPresent(t *testing.T) {
	out, _, err := spec.Upgrade([]byte(`{"panels":[{"query":"up"},{"query":"up","legend":"{{pod}}"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Panels []struct {
			Queries []map[string]any `json:"queries"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Panels) != 2 {
		t.Fatalf("panels = %d, want 2", len(doc.Panels))
	}
	if got := doc.Panels[0].Queries; len(got) != 1 || len(got[0]) != 1 || got[0]["query"] != "up" {
		t.Errorf("panel without legend: queries = %v, want one object with only query", got)
	}
	if got := doc.Panels[1].Queries; len(got) != 1 || got[0]["legend"] != "{{pod}}" {
		t.Errorf("panel with legend: queries = %v, want legend kept", got)
	}
}
