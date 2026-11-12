package proposals

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func timeseries(id, title string, y int) string {
	return fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`, id, title, y)
}

func document(t *testing.T, panels ...string) *spec.Dashboard {
	t.Helper()
	raw := fmt.Sprintf(`{"specVersion":%q,"title":"t","time":{"from":"now-6h","to":"now"},"panels":[%s]}`, spec.Version, strings.Join(panels, ","))
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func canonical(t *testing.T, d *spec.Dashboard) string {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestFold(t *testing.T) {
	p1 := timeseries("p1", "p1", 0)
	p2 := timeseries("p2", "p2", 8)
	p3 := timeseries("p3", "p3", 16)
	base := document(t, p1, p2)

	tests := []struct {
		name                         string
		headBase, head               *spec.Dashboard
		saveBase, save               *spec.Dashboard
		headBaseVersion, saveVersion int
		want                         *spec.Dashboard
		wantVersion                  int
	}{
		{
			name:     "a second save on the same base that changes another panel keeps both changes",
			headBase: base, head: document(t, timeseries("p1", "first", 0), p2),
			saveBase: base, save: document(t, p1, timeseries("p2", "second", 8)),
			headBaseVersion: 1, saveVersion: 1,
			want:        document(t, timeseries("p1", "first", 0), timeseries("p2", "second", 8)),
			wantVersion: 1,
		},
		{
			name:     "a second save that changes the same panel keeps the second",
			headBase: base, head: document(t, timeseries("p1", "first", 0), p2),
			saveBase: base, save: document(t, timeseries("p1", "second", 0), p2),
			headBaseVersion: 1, saveVersion: 1,
			want:        document(t, timeseries("p1", "second", 0), p2),
			wantVersion: 1,
		},
		{
			name:     "a second save on a newer base keeps the head's change and main's newer change",
			headBase: base, head: document(t, timeseries("p1", "first", 0), p2),
			saveBase: document(t, p1, timeseries("p2", "main", 8)), save: document(t, p1, timeseries("p2", "main", 8), p3),
			headBaseVersion: 1, saveVersion: 2,
			want:        document(t, timeseries("p1", "first", 0), timeseries("p2", "main", 8), p3),
			wantVersion: 2,
		},
		{
			name:     "a second save on an older base keeps all three",
			headBase: document(t, p1, timeseries("p2", "main", 8)), head: document(t, timeseries("p1", "first", 0), timeseries("p2", "main", 8)),
			saveBase: base, save: document(t, p1, p2, p3),
			headBaseVersion: 2, saveVersion: 1,
			want:        document(t, timeseries("p1", "first", 0), timeseries("p2", "main", 8), p3),
			wantVersion: 2,
		},
		{
			name:     "a head that cannot follow gives way to the save as it is",
			headBase: base, head: document(t, p1, p2, timeseries("p4", "head", 30)),
			saveBase: document(t, p1, p2, timeseries("p5", "main", 30)), save: document(t, p1, p2, timeseries("p5", "main", 30), timeseries("p6", "save", 40)),
			headBaseVersion: 1, saveVersion: 2,
			want:        document(t, p1, p2, timeseries("p5", "main", 30), timeseries("p6", "save", 40)),
			wantVersion: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, version, err := fold(tc.headBase, tc.head, tc.saveBase, tc.save, tc.headBaseVersion, tc.saveVersion)
			if err != nil {
				t.Fatal(err)
			}
			if canonical(t, got) != canonical(t, tc.want) {
				t.Errorf("folded spec = %s, want %s", canonical(t, got), canonical(t, tc.want))
			}
			if version != tc.wantVersion {
				t.Errorf("base version = %d, want %d", version, tc.wantVersion)
			}
		})
	}
}

func TestFoldFallsBackToTheSavesOwnBaseWhenItCannotCombine(t *testing.T) {
	p1 := timeseries("p1", "p1", 0)
	older := document(t, p1)
	newer := document(t, p1, timeseries("p5", "main", 30))
	head := document(t, p1, timeseries("p5", "head", 30), timeseries("p6", "head", 40))
	save := document(t, p1, timeseries("p7", "save", 30))
	got, version, err := fold(newer, head, older, save, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if canonical(t, got) != canonical(t, save) || version != 1 {
		t.Errorf("fold = %s at base %d, want the save at base 1", canonical(t, got), version)
	}
}
