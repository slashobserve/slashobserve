package observations

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func hasKeyAnywhere(t *testing.T, raw json.RawMessage, keys ...string) bool {
	t.Helper()
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch n := v.(type) {
		case map[string]any:
			for k, child := range n {
				if slices.Contains(keys, k) || walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range n {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(doc)
}

func TestWithoutQueriesLeavesNoQueryTextInAnyPanelType(t *testing.T) {
	panels := map[string]string{
		"timeseries": `{"type":"timeseries","title":"Rate","datasource":"prom","queries":[{"ref":"A","query":"sum(rate(http_requests_total[5m]))","legend":"{{code}}"},{"ref":"B","expression":{"type":"math","math":"$A * 2"}}],"unit":"reqps"}`,
		"logs":       `{"type":"logs","title":"Errors","datasource":"loki","queries":[{"ref":"A","query":"{app=\"checkout\"} |= \"error\"","format":"logs"}]}`,
		"traces":     `{"type":"traces","title":"Slow","datasource":"tempo","queries":[{"ref":"A","query":"{ duration > 2s }"}]}`,
	}
	for name, panel := range panels {
		got, err := withoutQueries(json.RawMessage(panel))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if hasKeyAnywhere(t, got, "query", "queries") {
			t.Errorf("%s keeps query text: %s", name, got)
		}
		var kept struct {
			Type       string `json:"type"`
			Title      string `json:"title"`
			Datasource string `json:"datasource"`
		}
		if err := json.Unmarshal(got, &kept); err != nil || kept.Type != name || kept.Title == "" || kept.Datasource == "" {
			t.Errorf("%s lost its other fields: %s", name, got)
		}
	}
}

func TestWithoutQueriesKeepsTheQueryReferencesThatDrawThePanel(t *testing.T) {
	panel := json.RawMessage(`{"type":"timeseries","title":"Rate","queries":[{"ref":"A","query":"up"}],"overrides":[{"match":{"query":"A"},"unit":"ms"}]}`)
	got, err := withoutQueries(panel)
	if err != nil {
		t.Fatal(err)
	}
	var kept struct {
		Overrides []struct {
			Match struct {
				Query string `json:"query"`
			} `json:"match"`
		} `json:"overrides"`
	}
	if err := json.Unmarshal(got, &kept); err != nil || len(kept.Overrides) != 1 || kept.Overrides[0].Match.Query != "A" {
		t.Errorf("override reference changed: %s", got)
	}
}

func TestWithoutQueriesRefusesAPanelThatIsNotAnObject(t *testing.T) {
	if _, err := withoutQueries(json.RawMessage(`[1]`)); err == nil {
		t.Error("an array was accepted")
	}
}

func uuidOf(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: n}, Valid: true}
}

func block(n byte, kind Kind, state BlockState, position int32, width Width, body string) dbsqlc.ObservationBlock {
	return dbsqlc.ObservationBlock{ID: uuidOf(n), Kind: string(kind), State: string(state), Position: position, Width: string(width), Version: 1, Body: []byte(body)}
}

func TestPublishableKeepsLiveTextPanelAndSnapshotBlocksInOrder(t *testing.T) {
	rows := []dbsqlc.ObservationBlock{
		block(1, KindText, BlockLive, 0, WidthFull, `{"markdown":"first"}`),
		block(2, KindText, BlockRequested, 1, WidthFull, `{"markdown":"asked for"}`),
		block(3, KindRun, BlockLive, 2, WidthFull, `{}`),
		block(4, KindPanel, BlockLive, 3, WidthHalf, `{}`),
		block(5, KindSnapshot, BlockLive, 4, WidthHalf, `{}`),
	}
	var got []byte
	for _, b := range publishable(rows) {
		got = append(got, b.ID.Bytes[15])
	}
	if !slices.Equal(got, []byte{1, 4, 5}) {
		t.Errorf("published blocks %v, want [1 4 5]", got)
	}
}

func TestRCABlocksKeepOrderWidthsAndTakeSnapshotTitlesFromThePanel(t *testing.T) {
	live := []dbsqlc.ObservationBlock{
		block(1, KindText, BlockLive, 0, WidthFull, `{"markdown":"Started after the deploy"}`),
		block(2, KindPanel, BlockLive, 1, WidthHalf, `{}`),
		block(3, KindSnapshot, BlockLive, 2, WidthHalf, `{}`),
	}
	refs := map[pgtype.UUID]snapshotRef{
		uuidOf(2): {id: uuidOf(20), panel: json.RawMessage(`{"type":"timeseries","title":"Rate"}`)},
		uuidOf(3): {id: uuidOf(30), panel: json.RawMessage(`{"type":"stat","title":"Errors"}`)},
	}
	got, err := rcaBlocks(live, refs)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"kind":"text","markdown":"Started after the deploy","width":"full"},` +
		`{"kind":"snapshot","snapshot_id":"` + uuidOf(20).String() + `","title":"Rate","panel":{"type":"timeseries","title":"Rate"},"width":"half"},` +
		`{"kind":"snapshot","snapshot_id":"` + uuidOf(30).String() + `","title":"Errors","panel":{"type":"stat","title":"Errors"},"width":"half"}]`
	if string(raw) != want {
		t.Errorf("blocks\n got %s\nwant %s", raw, want)
	}
}

func TestRCABlocksRefuseAPanelWithoutASnapshot(t *testing.T) {
	live := []dbsqlc.ObservationBlock{block(2, KindPanel, BlockLive, 0, WidthFull, `{}`)}
	if _, err := rcaBlocks(live, nil); err == nil {
		t.Error("a panel block with no snapshot was accepted")
	}
}

func TestSameBlocksComparesIdsAndVersionsInOrder(t *testing.T) {
	a := []dbsqlc.ObservationBlock{block(1, KindText, BlockLive, 0, WidthFull, `{}`), block(2, KindText, BlockLive, 1, WidthFull, `{}`)}
	edited := slices.Clone(a)
	edited[1].Version = 2
	reordered := []dbsqlc.ObservationBlock{a[1], a[0]}
	cases := []struct {
		name string
		b    []dbsqlc.ObservationBlock
		want bool
	}{
		{"the same", slices.Clone(a), true},
		{"an edited block", edited, false},
		{"a removed block", a[:1], false},
		{"a reordered pair", reordered, false},
	}
	for _, c := range cases {
		if got := sameBlocks(a, c.b); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
