package logpatterns

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var base = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

var window = struct{ from, to time.Time }{base.Add(-time.Hour), base.Add(time.Hour)}

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
	got, truncated := Cluster(lines("POST /checkout status=200 for alice", "POST /checkout status=500 for bob", "POST /checkout status=200 for carol"), 10, window.from, window.to)
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
	got, _ := Cluster(lines("GET /a", "GET /a extra", "GET /a"), 10, window.from, window.to)
	if len(got) != 2 {
		t.Fatalf("patterns = %+v, want two", got)
	}
	if got[0].Pattern != "GET /a" || got[0].Count != 2 || got[1].Pattern != "GET /a extra" || got[1].Count != 1 {
		t.Fatalf("patterns = %+v", got)
	}
}

func TestClusterKeepsLinesBelowTheSimilarityFloorApart(t *testing.T) {
	got, _ := Cluster(lines("a b c d e", "a x y z w"), 10, window.from, window.to)
	if len(got) != 2 {
		t.Fatalf("patterns = %+v, want two clusters below 60%% similarity", got)
	}
	got, _ = Cluster(lines("a b c d e", "a b c x y"), 10, window.from, window.to)
	if len(got) != 1 || got[0].Pattern != "a b c <*> <*>" {
		t.Fatalf("patterns = %+v, want one cluster at 60%% similarity", got)
	}
}

func TestClusterOrdersByCountThenPatternAndTruncatesAtTheLimit(t *testing.T) {
	got, truncated := Cluster(lines("zeta one", "alpha one", "beta", "beta", "zeta one"), 2, window.from, window.to)
	if !truncated || len(got) != 2 {
		t.Fatalf("patterns = %+v, truncated %v", got, truncated)
	}
	if got[0].Pattern != "beta" || got[0].Count != 2 || got[1].Pattern != "zeta one" || got[1].Count != 2 {
		t.Fatalf("patterns = %+v", got)
	}
	got, truncated = Cluster(lines("zeta one", "alpha one", "beta", "beta", "zeta one"), 3, window.from, window.to)
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
	got, _ := Cluster(lines(texts...), MaxClusters+10, window.from, window.to)
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
	got, _ := Cluster(lines(texts...), 1, window.from, window.to)
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
	got, _ := Cluster(in, 5, window.from, window.to)
	if len(got) != 1 || got[0].Count != 3 || got[0].Sample != "alpha line new" || !got[0].First.Equal(base.Add(-time.Minute)) || !got[0].Last.Equal(base.Add(time.Minute)) {
		t.Fatalf("patterns = %+v", got)
	}
}

func TestClusterBucketsLinesIntoThirtyTrendBuckets(t *testing.T) {
	from := base
	to := base.Add(30 * time.Minute)
	var in []Line
	for i := range 40 {
		in = append(in, Line{Time: from.Add(time.Duration(i) * 45 * time.Second), Text: "tick"})
	}
	in = append(in, Line{Time: to, Text: "tick"}, Line{Time: from.Add(-time.Second), Text: "tick"})
	got, _ := Cluster(in, 5, from, to)
	if len(got) != 1 {
		t.Fatalf("patterns = %+v", got)
	}
	trend := got[0].Trend
	if len(trend) != 30 {
		t.Fatalf("%d buckets, want 30", len(trend))
	}
	var sum int64
	for _, b := range trend {
		sum += b.Count
	}
	if sum != 40 {
		t.Fatalf("buckets sum to %d, want the 40 lines inside [from, to)", sum)
	}
	if !trend[0].Start.Equal(from) || !trend[1].Start.Equal(from.Add(time.Minute)) || !trend[29].Start.Equal(from.Add(29*time.Minute)) {
		t.Fatalf("bucket starts = %v %v %v", trend[0].Start, trend[1].Start, trend[29].Start)
	}
	if trend[0].Count != 2 || trend[1].Count != 1 {
		t.Fatalf("first buckets = %+v %+v, want 2 lines at 0s and 45s then 1 at 90s", trend[0], trend[1])
	}
}

func TestTrendKeepsEmptyBucketsAndDropsLinesWithoutATime(t *testing.T) {
	trend := NewTrend(base, base.Add(30*time.Minute))
	trend.Add(base.Add(29*time.Minute+59*time.Second), 3)
	trend.Add(time.Time{}, 5)
	buckets := trend.Buckets()
	if len(buckets) != 30 || buckets[29].Count != 3 || buckets[0].Count != 0 {
		t.Fatalf("buckets = %+v", buckets)
	}
}

func TestClusterTakesTheLevelTheMostLinesCarry(t *testing.T) {
	cases := []struct {
		name   string
		levels []string
		want   string
	}{
		{"majority after normalising", []string{"ERROR", "error", "info"}, "error"},
		{"aliases vote for their kind", []string{"WARN", "warning", "fatal"}, "warning"},
		{"tie goes to the more severe kind", []string{"info", "error"}, "error"},
		{"no levels", []string{"", ""}, ""},
		{"blank lines do not vote", []string{"", "", "info"}, "info"},
		{"unknown words vote for other", []string{"verbose", "verbose", "info"}, "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in []Line
			for _, level := range tc.levels {
				in = append(in, Line{Time: base, Text: "request finished", Level: level})
			}
			got, _ := Cluster(in, 5, window.from, window.to)
			if len(got) != 1 || got[0].Level != tc.want {
				t.Fatalf("patterns = %+v, want level %q", got, tc.want)
			}
		})
	}
}

func TestPatternLevelReadsTheFirstLevelWord(t *testing.T) {
	cases := map[string]string{
		"<*> level=error msg=<*>":         "error",
		"2026 [WARN] cache miss":          "warning",
		`level="info" took <*>`:           "info",
		"level=debug then error":          "error",
		"GET /checkout status=<*>":        "",
		"time=<*> lvl=critical something": "error",
		"":                                "",
		"notice: restarting <*>":          "info",
	}
	for pattern, want := range cases {
		if got := PatternLevel(pattern); got != want {
			t.Errorf("PatternLevel(%q) = %q, want %q", pattern, got, want)
		}
	}
}
