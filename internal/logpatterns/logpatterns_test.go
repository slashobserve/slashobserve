package logpatterns

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

func TestTokenizeMasksVariableTokens(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"status 200", []string{"status", "<*>"}},
		{"took 1.5 or -3e2", []string{"took", "<*>", "or", "<*>"}},
		{"hex deadbeef01", []string{"hex", "<*>"}},
		{"hex 0xDEADBEEF", []string{"hex", "<*>"}},
		{"short abc123", []string{"short", "abc123"}},
		{"id 123e4567-e89b-12d3-a456-426614174000", []string{"id", "<*>"}},
		{"from 10.0.0.1", []string{"from", "<*>"}},
		{"from 2001:db8::1", []string{"from", "<*>"}},
		{"at 2026-11-06T10:00:01Z", []string{"at", "<*>"}},
		{"at 2026-11-06T10:00:01.123+02:00", []string{"at", "<*>"}},
		{"at 2026-11-06 10:00:01", []string{"at", "<*>", "<*>"}},
		{`msg "` + strings.Repeat("x", 40) + `"`, []string{"msg", "<*>"}},
		{`msg "short"`, []string{"msg", `"short"`}},
		{"duration_ms=31 user=alice", []string{"duration_ms=<*>", "user=alice"}},
		{"client=10.0.0.9 ts=2026-11-06T10:00:01Z", []string{"client=<*>", "ts=<*>"}},
		{"=5 a= b", []string{"=5", "a=", "b"}},
		{"GET\t/checkout  status=200", []string{"GET", "/checkout", "status=<*>"}},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := Tokenize(tt.line); !slices.Equal(got, tt.want) {
				t.Fatalf("Tokenize(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func lines(texts ...string) []Line {
	out := make([]Line, len(texts))
	for i, text := range texts {
		out[i] = Line{Time: base.Add(-time.Duration(i) * time.Second), Text: text}
	}
	return out
}

func TestClusterJoinsLinesThatDifferInOneToken(t *testing.T) {
	got, truncated := Cluster(lines("POST /checkout status=200 for alice", "POST /checkout status=500 for bob", "POST /checkout status=200 for carol"), 10)
	if truncated || len(got) != 1 {
		t.Fatalf("patterns = %+v, truncated %v", got, truncated)
	}
	p := got[0]
	if p.Pattern != "POST /checkout status=<*> for <*>" || p.Count != 3 {
		t.Fatalf("pattern = %+v", p)
	}
	if p.Sample != "POST /checkout status=200 for alice" {
		t.Fatalf("sample = %q, want the newest line", p.Sample)
	}
	if !p.Last.Equal(base) || !p.First.Equal(base.Add(-2*time.Second)) {
		t.Fatalf("first %v last %v", p.First, p.Last)
	}
}

func TestClusterNeverMergesDifferentTokenCounts(t *testing.T) {
	got, _ := Cluster(lines("GET /a", "GET /a extra", "GET /a"), 10)
	if len(got) != 2 {
		t.Fatalf("patterns = %+v, want two", got)
	}
	if got[0].Pattern != "GET /a" || got[0].Count != 2 || got[1].Pattern != "GET /a extra" || got[1].Count != 1 {
		t.Fatalf("patterns = %+v", got)
	}
}

func TestClusterKeepsLinesBelowTheSimilarityFloorApart(t *testing.T) {
	got, _ := Cluster(lines("a b c d e", "a x y z w"), 10)
	if len(got) != 2 {
		t.Fatalf("patterns = %+v, want two clusters below 60%% similarity", got)
	}
	got, _ = Cluster(lines("a b c d e", "a b c x y"), 10)
	if len(got) != 1 || got[0].Pattern != "a b c <*> <*>" {
		t.Fatalf("patterns = %+v, want one cluster at 60%% similarity", got)
	}
}

func TestClusterOrdersByCountThenPatternAndTruncatesAtTheLimit(t *testing.T) {
	got, truncated := Cluster(lines("zeta one", "alpha one", "beta", "beta", "zeta one"), 2)
	if !truncated || len(got) != 2 {
		t.Fatalf("patterns = %+v, truncated %v", got, truncated)
	}
	if got[0].Pattern != "beta" || got[0].Count != 2 || got[1].Pattern != "zeta one" || got[1].Count != 2 {
		t.Fatalf("patterns = %+v", got)
	}
	got, truncated = Cluster(lines("zeta one", "alpha one", "beta", "beta", "zeta one"), 3)
	if truncated || got[2].Pattern != "alpha one" {
		t.Fatalf("patterns = %+v, truncated %v", got, truncated)
	}
}

func TestClusterFoldsNewLinesIntoTheNearestClusterPastTheCap(t *testing.T) {
	var texts []string
	for i := range MaxClusters {
		texts = append(texts, "kind"+strconv.Itoa(i)+" fixed")
	}
	texts = append(texts, "kind7 other", "wholly different line here")
	got, _ := Cluster(lines(texts...), MaxClusters+10)
	if len(got) != MaxClusters {
		t.Fatalf("%d clusters, want the cap %d", len(got), MaxClusters)
	}
	var folded []string
	for _, p := range got {
		if p.Count > 1 {
			folded = append(folded, p.Pattern)
		}
	}
	slices.Sort(folded)
	if !slices.Equal(folded, []string{"<*> <*>", "kind7 <*>"}) {
		t.Fatalf("folded = %v, want kind7 to take its near line and the unrelated line to fold into a cluster", folded)
	}
}

func TestClusterReadsAtMostMaxLines(t *testing.T) {
	texts := make([]string, MaxLines+5)
	for i := range texts {
		texts[i] = "same line"
	}
	got, _ := Cluster(lines(texts...), 1)
	if got[0].Count != MaxLines {
		t.Fatalf("count = %d, want %d", got[0].Count, MaxLines)
	}
}

func TestClusterSkipsBlankLinesAndKeepsTheNewestSampleWhateverTheOrder(t *testing.T) {
	in := []Line{
		{Time: base.Add(-time.Minute), Text: "alpha line old"},
		{Time: base, Text: "  "},
		{Time: base.Add(time.Minute), Text: "alpha line new"},
		{Time: base, Text: "alpha line mid"},
	}
	got, _ := Cluster(in, 5)
	if len(got) != 1 || got[0].Count != 3 || got[0].Sample != "alpha line new" || !got[0].First.Equal(base.Add(-time.Minute)) || !got[0].Last.Equal(base.Add(time.Minute)) {
		t.Fatalf("patterns = %+v", got)
	}
}
