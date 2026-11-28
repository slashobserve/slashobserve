package runsession

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestCallMetaCarriesTheSideFieldsOfAResult(t *testing.T) {
	from := time.Date(2026, 3, 1, 10, 0, 0, 0, time.FixedZone("x", 3600))
	res := tools.Result{
		Refusal:    tools.RefusalOnePanel,
		Unchecked:  "query_timeout",
		Edits:      tools.Edits{Panels: []string{"a"}, Added: []string{"a"}},
		Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"up"}},
	}
	got, err := json.Marshal(callMeta(res, []datasource.TimeRange{{From: from, To: from.Add(time.Hour)}}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"edits":{"panels":["a"],"added":["a"],"deleted":[],"settings":false},"refusal":"one_panel","unchecked":"query_timeout",` +
		`"discovered":{"datasource":"prom","of":"metrics","names":["up"]},"ranges":[{"from":"2026-03-01T09:00:00Z","to":"2026-03-01T10:00:00Z"}]}`
	if string(got) != want {
		t.Fatalf("call meta = %s\nwant %s", got, want)
	}
}

func TestCallMetaOfAPlainResultHoldsNullsAndNoRanges(t *testing.T) {
	got, err := json.Marshal(callMeta(tools.Result{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"edits":null,"refusal":null,"unchecked":null,"discovered":null,"ranges":[]}` {
		t.Fatalf("call meta = %s", got)
	}
}
