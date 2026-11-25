package elasticsearch_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const (
	emptyHits     = `{"timed_out":false,"_shards":{"failed":0},"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`
	levelValues   = `{"timed_out":false,"_shards":{"failed":0},"hits":{"total":{"value":9,"relation":"eq"},"hits":[]},"aggregations":{"log.level":{"doc_count_error_upper_bound":0,"sum_other_doc_count":0,"buckets":[{"key":"INFO","doc_count":5},{"key":"ERROR","doc_count":3},{"key":"WARN","doc_count":1}]},"log.level__card":{"value":3},"service.name":{"doc_count_error_upper_bound":0,"sum_other_doc_count":0,"buckets":[{"key":"checkout","doc_count":9}]},"service.name__card":{"value":1}}}`
	hintedCase    = "No document has log.level = error in this range, but it has ERROR; values are case sensitive, so seen: INFO, ERROR, WARN."
	unknownNope   = `Field "nope" is not in the mapping; text fields to try: message, error.stack_trace.`
	zeroCounts    = `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[]},"aggregations":{"count":{"buckets":[{"key":1793962800000,"doc_count":0},{"key":1793962860000,"doc_count":0}]}}}`
	someCounts    = `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[]},"aggregations":{"count":{"buckets":[{"key":1793962800000,"doc_count":0},{"key":1793962860000,"doc_count":4}]}}}`
	hintedMissing = "No document has log.level = fatal in this range; values seen: INFO, ERROR, WARN."
)

func hintServer(t *testing.T) (*fakeServer, *elasticsearch.Datasource) {
	t.Helper()
	f := healthyServer(t)
	f.set(http.MethodPost, searchPath, func(r request) reply {
		if strings.Contains(r.body, `"terms"`) {
			return reply{status: http.StatusOK, body: levelValues}
		}
		return reply{status: http.StatusOK, body: emptyHits}
	})
	return f, serve(t, f)
}

func TestAnEmptyInstantQueryNamesTheValueThatDiffersOnlyByCase(t *testing.T) {
	_, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "service.name:checkout and log.level:error"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != hintedCase {
		t.Fatalf("hint = %q", got)
	}
}

func TestAnEmptyPageNamesTheValueThatDiffersOnlyByCase(t *testing.T) {
	_, ds := hintServer(t)

	frames, _, err := ds.QueryPage(context.Background(), datasource.PageRequest{Query: datasource.Query{Text: "log.level:error"}, Range: queryRange, PageSize: 50, Page: 1})
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != hintedCase {
		t.Fatalf("hint = %q", got)
	}
}

func TestAnEmptyQueryListsTheValuesSeenWhenTheValueNeverOccurs(t *testing.T) {
	_, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "log.level:fatal"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != hintedMissing {
		t.Fatalf("hint = %q", got)
	}
}

func TestAQueryThatFindsLinesAsksNothingMore(t *testing.T) {
	f, ds := searchServer(t, hitsOf(2))

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "log.level:error"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != "" || f.count(searchPath) != 1 {
		t.Fatalf("hint = %q after %d searches", got, f.count(searchPath))
	}
}

func TestAnEmptyQueryWithAnExistingValueGivesNoHint(t *testing.T) {
	_, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "log.level:ERROR"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != "" {
		t.Fatalf("hint = %q", got)
	}
}

func TestAnEmptyQueryOnlyHintsAboutValuesItMustMatch(t *testing.T) {
	for _, text := range []string{"not log.level:error", "log.level:error or service.name:other", "log.level:err*", "message:error"} {
		_, ds := hintServer(t)
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, queryRange)
		if err != nil {
			t.Fatal(err)
		}
		if got := datasource.MergeMeta(frames).Hint; got != "" {
			t.Fatalf("%q: hint = %q", text, got)
		}
	}
}

func TestAnEmptyInstantQueryNamesTheFieldThatDiffersOnlyByCase(t *testing.T) {
	_, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "Message:error"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	want := `Field "Message" is not in the mapping; did you mean "message"?`
	if got := datasource.MergeMeta(frames).Hint; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestAnEmptyPageNamesAFieldThatIsNotInTheMapping(t *testing.T) {
	_, ds := hintServer(t)

	frames, _, err := ds.QueryPage(context.Background(), datasource.PageRequest{Query: datasource.Query{Text: "nope:error"}, Range: queryRange, PageSize: 50, Page: 1})
	if err != nil {
		t.Fatal(err)
	}

	want := unknownNope
	if got := datasource.MergeMeta(frames).Hint; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestAnEmptyQueryOnlyNamesUnknownFieldsItMustMatch(t *testing.T) {
	for _, text := range []string{"not nope:error", "log.level:fatal or nope:error"} {
		_, ds := hintServer(t)
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, queryRange)
		if err != nil {
			t.Fatal(err)
		}
		if got := datasource.MergeMeta(frames).Hint; strings.Contains(got, "not in the mapping") {
			t.Fatalf("%q: hint = %q", text, got)
		}
	}
}

func TestAnUnknownFieldHintComesBeforeTheValueHintAndSkipsItsSearch(t *testing.T) {
	f, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "nope:error and log.level:error"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	if got := datasource.MergeMeta(frames).Hint; got != unknownNope || f.count(searchPath) != 1 {
		t.Fatalf("hint = %q after %d searches", got, f.count(searchPath))
	}
}

func TestAnEmptyInstantQueryOnAnUnknownFieldListsTheTextFieldsToTry(t *testing.T) {
	_, ds := hintServer(t)

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "Body:error"}, queryRange)
	if err != nil {
		t.Fatal(err)
	}

	want := `Field "Body" is not in the mapping; text fields to try: message, error.stack_trace.`
	if got := datasource.MergeMeta(frames).Hint; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestAnEmptyCountOverTimeNamesAFieldThatIsNotInTheMapping(t *testing.T) {
	f, ds := searchServer(t, zeroCounts)

	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "Body:error"}, queryRange, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	want := `Field "Body" is not in the mapping; text fields to try: message, error.stack_trace.`
	if got := datasource.MergeMeta(frames).Hint; got != want || f.count(searchPath) != 1 {
		t.Fatalf("hint = %q after %d searches", got, f.count(searchPath))
	}
}

func TestAnEmptyCountOverTimeOffersTheFieldThatDiffersOnlyByCase(t *testing.T) {
	_, ds := searchServer(t, zeroCounts)

	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "Message:error"}, queryRange, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	want := `Field "Message" is not in the mapping; did you mean "message"?`
	if got := datasource.MergeMeta(frames).Hint; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestACountOverTimeWithLinesOrKnownFieldsGivesNoHint(t *testing.T) {
	for _, tc := range []struct{ name, text, reply string }{
		{"lines found", "Body:error", someCounts},
		{"known field", "message:error", zeroCounts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ds := searchServer(t, tc.reply)
			frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: tc.text}, queryRange, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if got := datasource.MergeMeta(frames).Hint; got != "" {
				t.Fatalf("hint = %q", got)
			}
		})
	}
}
