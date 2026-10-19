package spec_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func draftBase(t *testing.T) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(`{
		"specVersion": "` + spec.Version + `",
		"title": "Draft",
		"time": {"from": "now-15m", "to": "now"},
		"panels": [{
			"id": "p1", "type": "stat", "title": "One",
			"gridPos": {"x": 0, "y": 0, "w": 12, "h": 4},
			"datasource": "ch",
			"queries": [{"query": "SELECT 1"}]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWithPanelRejectsMalformedDrafts(t *testing.T) {
	const good = `"id":"p1","type":"stat","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":"ch","queries":[{"query":"SELECT 1"}]`
	tests := []struct {
		name  string
		draft string
		path  string
	}{
		{"duplicate key", `{"title":"a","title":"b",` + good + `}`, "/panels/0/title"},
		{"invalid utf-8", "{\"title\":\"a\xff\"," + good + "}", "/panels/0"},
		{"lone surrogate", `{"title":"\ud800",` + good + `}`, "/panels/0"},
		{"trailing garbage", `{"title":"a",` + good + `} garbage`, "/panels/0"},
		{"extra closing brace", `{"title":"a",` + good + `}}`, "/panels/0"},
		{"trailing comma", `{"title":"a",` + good + `},`, "/panels/0"},
		{"not json", `{"title":`, "/panels/0"},
		{"too deep", `{"title":"a",` + good + `,"x":` + strings.Repeat("[", 98) + strings.Repeat("]", 98) + `}`, "/panels/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spec.WithPanel(draftBase(t), "p1", json.RawMessage(tt.draft))
			var ve *spec.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			if len(ve.Issues) == 0 {
				t.Fatal("no issues")
			}
			for _, i := range ve.Issues {
				if i.Code != spec.CodeMalformedJSON && i.Code != spec.CodeDuplicateKey {
					t.Errorf("issue %+v: want malformed_json or duplicate_key", i)
				}
			}
			if got := ve.Issues[0].Path; got != tt.path {
				t.Errorf("path = %q, want %q", got, tt.path)
			}
		})
	}
}

func TestWithPanelReplacesPanel(t *testing.T) {
	d := draftBase(t)
	draft := `{"id":"p1","type":"stat","title":"a<b&c>","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":"ch","queries":[{"query":"SELECT 2"}]}`
	got, err := spec.WithPanel(d, "p1", json.RawMessage(draft))
	if err != nil {
		t.Fatal(err)
	}
	if got.Panels[0].Title != "a<b&c>" || got.Panels[0].Queries[0].Query != "SELECT 2" {
		t.Errorf("panel = %+v", got.Panels[0])
	}
	if d.Panels[0].Title != "One" {
		t.Error("base dashboard was mutated")
	}
	if _, err := spec.WithPanel(d, "missing", json.RawMessage(draft)); !errors.Is(err, spec.ErrNoPanel) {
		t.Errorf("err = %v, want ErrNoPanel", err)
	}
}
