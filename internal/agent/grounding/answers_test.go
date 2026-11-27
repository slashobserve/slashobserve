package grounding

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func answerLedger() *Ledger {
	window := func(f Fact) Fact {
		f.From, f.To, f.Asked = "2026-11-20T09:00:00Z", "2026-11-20T11:00:00Z", "p99"
		return f
	}
	return &Ledger{Facts: []Fact{
		window(Fact{ID: "f1", Text: "0.5234", Value: 0.5234, Kind: FactNumber, Of: "measure_window max values[] value", Labels: map[string]string{"service": "checkout"}, Query: "max_over_time(p99[2h:30s])"}),
		window(Fact{ID: "f2", Text: "0.61", Value: 0.61, Kind: FactNumber, Of: "measure_window max values[] value", Labels: map[string]string{"service": "checkout"}, Query: "max_over_time(p99[2h:15s])"}),
		window(Fact{ID: "f3", Text: "0.5235", Value: 0.5235, Kind: FactNumber, Of: "measure_window max values[] value", Labels: map[string]string{"service": "checkout"}, Query: "max_over_time(p99[2h:10s])"}),
		window(Fact{ID: "f4", Text: "0.287", Value: 0.287, Kind: FactNumber, Of: "measure_window max values[] value", Labels: map[string]string{"service": "payment"}}),
		{ID: "f5", Text: "1312.75", Value: 1312.75, Kind: FactDuration, Unit: "ms", Of: "trace_slow_path routes[].example duration_ms", Labels: map[string]string{"route": "GET /cart", "trace_id": "4BF92F3577B34DA6A3CE929D0E0E4736"}},
		{ID: "f6", Text: "1290", Value: 1290, Kind: FactDuration, Unit: "ms", Of: "get_trace duration_ms", Labels: map[string]string{"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"}},
		{ID: "f7", Text: "4bf92f3577b34da6a3ce929d0e0e4736", Kind: FactTraceID, Of: "trace_slow_path routes[].example trace_id"},
	}}
}

func texts(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Kind+" "+p.Text)
	}
	return out
}

func TestOneQuantityTakesOneFact(t *testing.T) {
	cases := []struct {
		name    string
		answers []Answer
		want    []string
	}{
		{"one fact per quantity", []Answer{{"peak p99", "f1"}, {"payment p99", "f4"}, {"example trace", "f7"}}, nil},
		{"two facts with one value", []Answer{{"peak p99", "f1"}, {"Peak P99", "f3"}}, nil},
		{"two facts with different values", []Answer{{"peak p99", "f1"}, {"peak  p99.", "f2"}}, []string{"fact answers: peak  p99. = f1 or f2"}},
		{"an unknown fact", []Answer{{"error rate", "f40"}}, []string{"fact answers: error rate = f40"}},
	}
	for _, c := range cases {
		if got := texts(answerLedger().AnswerProblems(c.answers)); !slices.Equal(got, c.want) {
			t.Errorf("%s: problems = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTheMarkdownCitesOnlyOneValueOfAQuantity(t *testing.T) {
	cases := []struct {
		markdown string
		want     []string
	}{
		{"The peak was {{f1:ms}}, against {{f4:ms}} for payment.", nil},
		{"The peak was {{f1:ms}} or {{f3:ms}}.", nil},
		{"The peak was {{f1:ms}}; at a finer step it was {{f2:ms}}.", []string{"fact {{f1}} and {{f2}}"}},
		{"The example trace {{f7}} took {{f5}}, and {{f6}} in the trace view.", []string{"fact {{f5}} and {{f6}}"}},
	}
	for _, c := range cases {
		if got := texts(answerLedger().conflicts(c.markdown)); !slices.Equal(got, c.want) {
			t.Errorf("%q: problems = %q, want %q", c.markdown, got, c.want)
		}
	}
}

func TestAnAnswerMustBeGivenInTheMarkdown(t *testing.T) {
	answers := []Answer{{"peak p99", "f1"}, {"example trace", "f7"}}
	cases := map[string][]string{
		"Peak p99 was {{f1:ms}} in trace {{f7}}.":                          nil,
		"Peak p99 was 523.4 ms in trace 4BF92F3577B34DA6A3CE929D0E0E4736.": nil,
		"Peak p99 was 0.5 s in trace 4bf92f3577b34da6a3ce929d0e0e4736.":    {"fact answers: peak p99 = f1"},
		"Peak p99 was high, as trace 4bf92f35 shows.":                      {"fact answers: peak p99 = f1", "fact answers: example trace = f7"},
	}
	for markdown, want := range cases {
		got := texts(answerLedger().unstated(answers, markdown))
		if !slices.Equal(got, want) {
			t.Errorf("%q: problems = %q, want %q", markdown, got, want)
		}
	}
}

func TestTheSendBackSaysHowToFixEachItem(t *testing.T) {
	got := SendBackProblems(answerLedger().AnswerProblems([]Answer{{"peak p99", "f1"}, {"peak p99", "f2"}}))
	if !strings.HasPrefix(got, reviewLead+"\n- cited fact \"answers: peak p99 = f1 or f2\": one quantity takes one fact") {
		t.Errorf("send-back =\n%s", got)
	}
}

func TestMissingAnswersNeedsAFigureAndNoAnswers(t *testing.T) {
	for _, c := range []struct {
		input string
		want  bool
	}{
		{`{"markdown":"p99 was 523 ms."}`, true},
		{`{"markdown":"p99 was {{f3}}."}`, true},
		{`{"markdown":"Fine.","impact":"2,950 orders failed"}`, true},
		{`{"markdown":"p99 was {{f3}}.","answers":[{"quantity":"p99","fact":"f3"}]}`, false},
		{`{"markdown":"Checkout is healthy."}`, false},
		{`{"markdown":"The last 15 minutes look fine."}`, false},
		{`not json`, false},
	} {
		p, got := MissingAnswers(json.RawMessage(c.input), "is checkout fine over the last 15 minutes")
		if got != c.want {
			t.Errorf("%s: missing = %v, want %v", c.input, got, c.want)
		}
		if got && (p.Kind != kindAnswers || !strings.Contains(SendBackProblems([]Problem{p}), "- answer ")) {
			t.Errorf("%s: problem = %+v", c.input, p)
		}
	}
}
