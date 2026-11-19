package grafana

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestConvertedTitlesFollowTheTextRuleOfTheServer(t *testing.T) {
	bad := []string{"CPU\nusage", "Latency\tp95", "Latency\u00ad", "Mem\u200bory", "x\u202ey", "a\u0007b", "\u200b\u200b", "  "}
	for _, title := range bad {
		quoted, _ := json.Marshal(title)
		doc := fmt.Sprintf(`{"title":%s,"panels":[
			{"id":1,"type":"row","title":%s,"collapsed":false,"gridPos":{"x":0,"y":0,"w":24,"h":1}},
			{"id":2,"type":"timeseries","title":%s,"gridPos":{"x":0,"y":1,"w":12,"h":4},"datasource":%s,"targets":[{"refId":"A","expr":"up","datasource":%s}]},
			{"id":3,"type":"no-such-plugin","title":%s,"gridPos":{"x":12,"y":1,"w":12,"h":4}}
		]}`, quoted, quoted, quoted, promRef, promRef, quoted)
		res := convertedSpec(t, doc)
		var walk func(path string, panels []spec.Panel)
		walk = func(path string, panels []spec.Panel) {
			for _, p := range panels {
				if p.Title != "" {
					if _, err := usertext.Clean("title", p.Title, usertext.Line(200)); err != nil || p.Title != trimmed(p.Title) {
						t.Errorf("%q: panel %s title %q: %v", title, p.ID, p.Title, err)
					}
				}
				walk(path, p.Panels)
			}
		}
		walk("", res.Spec.Panels)
		if _, err := usertext.Clean("title", res.Spec.Title, usertext.Line(200)); err != nil {
			t.Errorf("%q: dashboard title %q: %v", title, res.Spec.Title, err)
		}
	}
}

func trimmed(s string) string {
	cleaned, _ := usertext.Clean("title", s, usertext.Line(200))
	return cleaned
}

func TestConvertedTitleKeepsItsWordsAndEmoji(t *testing.T) {
	got := cleanTitle("  CPU\nusage \U0001F468\u200d\U0001F469\u00ad ")
	if want := "CPU usage \U0001F468\u200d\U0001F469"; got != want {
		t.Errorf("cleanTitle = %q, want %q", got, want)
	}
}
