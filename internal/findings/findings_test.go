package findings_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestTitleIsAShortCleanFirstLine(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"plain", "why is checkout slow", "why is checkout slow"},
		{"blank leading lines", "\n  \n  why is checkout slow\nsecond line", "why is checkout slow"},
		{"mention at the start", "@observe why is checkout slow", "why is checkout slow"},
		{"mention with a colon", "@Observe: why is checkout slow", "why is checkout slow"},
		{"mention in the middle", "why is @observe checkout slow", "why is checkout slow"},
		{"a longer name is not a mention", "ask @observer", "ask @observer"},
		{"cut at a word", "which pod is responsible for the elevated server errors in checkout today", "which pod is responsible for the elevated server errors in..."},
		{"one long word", strings.Repeat("a", 70), strings.Repeat("a", 60) + "..."},
		{"multi-byte cut", strings.Repeat("é", 90), strings.Repeat("é", 60) + "..."},
		{"too short to name anything", "x", "Finding"},
		{"only a mention", "@observe", "Finding"},
		{"empty", " \n ", "Finding"},
	}
	for _, c := range cases {
		if got := findings.Title(c.text); got != c.want {
			t.Errorf("%s: Title(%q) = %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

func TestTitleFromPrefersTheTitleTheAgentGaveAndNeverUsesThePrompt(t *testing.T) {
	long := "which pod is responsible for the elevated server errors in checkout today"
	cases := []struct {
		name, given   string
		investigation bool
		want          string
	}{
		{"given", "Checkout 5xx by pod", false, "Checkout 5xx by pod"},
		{"given is cleaned too", "@observe " + long, true, "which pod is responsible for the elevated server errors in..."},
		{"an ask without a title", "  ", false, "Answer"},
		{"an investigation without a title", "", true, "Finding"},
	}
	for _, c := range cases {
		if got := findings.TitleFrom(c.given, c.investigation); got != c.want {
			t.Errorf("%s: TitleFrom(%q, %v) = %q, want %q", c.name, c.given, c.investigation, got, c.want)
		}
	}
}

func TestFirstLineStripsMarkdownMarkers(t *testing.T) {
	cases := []struct {
		name, markdown, want string
	}{
		{"heading", "## Checkout errors come from checkout-0\nmore", "Checkout errors come from checkout-0"},
		{"bullet", "\n- checkout-0 returns 500s", "checkout-0 returns 500s"},
		{"quote", "> the queue is backed up", "the queue is backed up"},
		{"long", strings.Repeat("x", 250), strings.Repeat("x", 200)},
		{"only markers", "## \n", "Finding"},
	}
	for _, c := range cases {
		if got := findings.FirstLine(c.markdown); got != c.want {
			t.Errorf("%s: FirstLine(%q) = %q, want %q", c.name, c.markdown, got, c.want)
		}
	}
}

func TestWordsCountsWhitespaceSeparatedWords(t *testing.T) {
	for _, n := range []int{400, 401} {
		if got := findings.Words(strings.TrimSpace(strings.Repeat("word ", n))); got != n {
			t.Errorf("Words of %d words = %d", n, got)
		}
	}
}

func TestWordsIgnoresTableSyntax(t *testing.T) {
	table := "| Service | Errors | Share |\n|---|:---:|---:|\n| checkout | 42 | 8% |\n|payments|7|1%|"
	if got, want := findings.Words(table), 9; got != want {
		t.Errorf("Words of a table = %d, want %d", got, want)
	}
	prose := "Checkout errors rose.\n\n" + table + "\n\n---\n\nSee the panel."
	if got, want := findings.Words(prose), 9+3+3; got != want {
		t.Errorf("Words of prose around a table = %d, want %d", got, want)
	}
}

func evidencePanel(id string) json.RawMessage {
	return json.RawMessage(`{"id":"` + id + `","type":"timeseries","title":"Errors","datasource":"prom","gridPos":{"x":3,"y":40,"w":5,"h":2},"queries":[{"query":"sum(rate(http_requests_total[5m]))"}]}`)
}

func TestEvidenceSpecRenumbersAndLaysOutPanels(t *testing.T) {
	from := time.UnixMilli(1_700_000_000_000)
	to := from.Add(time.Hour)
	d, err := findings.EvidenceSpec("Why", from, to, []json.RawMessage{evidencePanel("p"), evidencePanel("p"), evidencePanel("x"), evidencePanel("y")})
	if err != nil {
		t.Fatal(err)
	}
	if d.Time.From != "2023-11-14T22:13:20Z" || d.Time.To != "2023-11-14T23:13:20Z" {
		t.Errorf("time = %+v", d.Time)
	}
	want := []struct {
		id   string
		grid spec.GridPos
	}{
		{"evidence-1", spec.GridPos{X: 0, Y: 0, W: 12, H: 8}},
		{"evidence-2", spec.GridPos{X: 12, Y: 0, W: 12, H: 8}},
		{"evidence-3", spec.GridPos{X: 0, Y: 8, W: 12, H: 8}},
		{"evidence-4", spec.GridPos{X: 12, Y: 8, W: 12, H: 8}},
	}
	if len(d.Panels) != len(want) {
		t.Fatalf("panels = %d", len(d.Panels))
	}
	for i, w := range want {
		if d.Panels[i].ID != w.id || d.Panels[i].GridPos != w.grid {
			t.Errorf("panel %d = %s %+v, want %s %+v", i, d.Panels[i].ID, d.Panels[i].GridPos, w.id, w.grid)
		}
	}
}

func TestEvidenceSpecRejectsBadPanels(t *testing.T) {
	from := time.UnixMilli(1_700_000_000_000)
	if _, err := findings.EvidenceSpec("Why", from, from.Add(time.Hour), []json.RawMessage{json.RawMessage(`[1]`)}); err == nil || !strings.Contains(err.Error(), "evidence[0]") {
		t.Errorf("a non-object panel: %v", err)
	}
	unknown := json.RawMessage(`{"type":"sparkles","title":"x"}`)
	_, err := findings.EvidenceSpec("Why", from, from.Add(time.Hour), []json.RawMessage{unknown})
	var invalid *spec.ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("an unknown panel type: %v, want a *spec.ValidationError", err)
	}
}

func TestSpecOfAStoredFindingUsesItsWindow(t *testing.T) {
	from := time.UnixMilli(1_700_000_000_000)
	f := findings.Finding{Title: "Why", Markdown: "## Answer\nmore", From: from, To: from.Add(time.Hour), Evidence: []spec.Panel{{ID: "evidence-1", Type: spec.Timeseries}}}
	d := f.Spec()
	if d.Title != "Why" || d.Description != "Answer" || d.Time.From != "2023-11-14T22:13:20Z" || d.Time.To != "2023-11-14T23:13:20Z" || len(d.Panels) != 1 {
		t.Fatalf("spec = %+v", d)
	}
}
