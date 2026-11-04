package agent

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestDigestKeepsTheNewestEntryPerSourceAndOrdersNewestFirst(t *testing.T) {
	var d digest
	d.add(tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"up", "http_requests_total"}})
	d.add(tools.Discovery{Datasource: "prom", Of: "labels of http_requests_total", Names: []string{"code", "pod"}})
	d.add(tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"node_load1"}})
	d.add(tools.Discovery{Datasource: "ch", Of: "tables", Names: []string{"otel_logs"}})

	want := "<discovered>\n" +
		"Names this run already found; use them without listing them again.\n" +
		"- ch tables: otel_logs\n" +
		"- prom metrics: node_load1\n" +
		"- prom labels of http_requests_total: code, pod\n" +
		"</discovered>"
	if got := d.text(); got != want {
		t.Fatalf("text:\n%s\nwant:\n%s", got, want)
	}
}

func TestDigestStopsBeforeItsByteCap(t *testing.T) {
	var d digest
	for _, source := range []string{"first", "second", "third"} {
		names := make([]string, 120)
		for i := range names {
			names[i] = "metric_" + source + "_0123456789"
		}
		d.add(tools.Discovery{Datasource: "prom", Of: source, Names: names})
	}

	got := d.text()
	if len(got) > maxDigestBytes || !strings.HasSuffix(got, "</discovered>") {
		t.Fatalf("digest is %d bytes and ends %q", len(got), got[max(0, len(got)-20):])
	}
	if !strings.Contains(got, "- prom third:") || strings.Contains(got, "- prom first:") {
		t.Fatalf("the newest entry must come first and the oldest be dropped:\n%.200s", got)
	}
}

func TestDigestIsEmptyWithoutEntries(t *testing.T) {
	var d digest
	if got := d.text(); got != "" {
		t.Fatalf("text = %q", got)
	}
	d.add(tools.Discovery{Datasource: "prom", Of: "metrics"})
	if got := d.text(); got != "" {
		t.Fatalf("an entry without names must be ignored: %q", got)
	}
	d.add(tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{strings.Repeat("x", maxDigestBytes)}})
	if got := d.text(); got != "" {
		t.Fatalf("an entry that cannot fit leaves no digest: %d bytes", len(got))
	}
}
