package route

import "testing"

var allTypes = []string{"prometheus", "loki", "tempo"}

func TestClassifyPicksOneTaskTypeOrFallsBack(t *testing.T) {
	cases := []struct {
		text string
		want Task
	}{
		{"what is the p99 latency of the cart service over the last hour", Measure},
		{"how many requests did frontend serve yesterday", Measure},
		{"peak memory of the payment pods this morning", Measure},
		{"what share of checkout requests returned 5xx", Measure},
		{"compare the request rate of ad and recommendation", Measure},
		{"current error rate of the quote service versus an hour ago", Measure},
		{"total bytes sent by the email service today", Measure},
		{"average cpu utilization of the kafka brokers", Measure},
		{"how much did throughput drop after 14:00", Measure},
		{"which service has the highest error rate", Measure},
		{"top 5 endpoints by median duration", Measure},
		{"p95 of the currency conversion over the window", Measure},
		{"rps on the shipping api right now", Measure},
		{"how long does a checkout take on average", Measure},
		{"which services have errors in the last 15 minutes", Measure},

		{"show the error logs of the cart service", Logs},
		{"how many log lines mention timeout", Logs},
		{"what exceptions does the ad service log", Logs},
		{"count warning messages from fraud detection", Logs},
		{"find the panic in the product catalog logs", Logs},
		{"which log messages are new since the deploy", Logs},
		{"what is in the stack of the last exception", Logs},
		{"give me the logs around 10:42", Logs},

		{"show me the slowest requests to checkout", Traces},
		{"find a trace where cart calls redis twice", Traces},
		{"which span takes the most self time on placeorder", Traces},
		{"where do requests spend their time in the frontend", Traces},
		{"what is the call path of a payment", Traces},
		{"summary of spans for trace 4bf92f3577b34da6a3ce929d0e0e4736", Explain},
		{"get the trace id of a slow request to recommendations", Traces},
		{"which downstream does checkout call most", Traces},
		{"where did the failing requests originate", Incident},
		{"how many spans does a checkout have", Traces},

		{"why is checkout slow", Incident},
		{"why did errors rise after 10:00", Incident},
		{"what happened to payments last night", Incident},
		{"root cause of the latency spike", Incident},
		{"is the cart service healthy", Incident},
		{"what changed in the last 6 hours", Incident},
		{"is anything broken right now", Incident},
		{"investigate the 500s on the frontend", Incident},
		{"what is wrong with the ad service", Incident},
		{"was there an outage this week", Incident},
		{"what's going on with kafka", Incident},
		{"what caused the error logs to spike", Incident},

		{"which services exist", Inventory},
		{"list the datasources", Inventory},
		{"what dashboards do we have", Inventory},
		{"which labels does the prometheus store have", Inventory},
		{"what do we collect from the shop", Inventory},
		{"show me all routes", Inventory},
		{"what tables are in clickhouse", Inventory},

		{"explain this dashboard", Explain},
		{"what does p99 mean", Explain},
		{"what is a span", Explain},
		{"summarize this observation", Explain},
		{"rewrite the conclusion in two sentences", Explain},
		{"what does this panel show", Explain},
		{"describe the checkout dashboard", Explain},
		{"what is the difference between rate and irate", Explain},

		{"pin this observation", Full},
		{"add a block with the error rate", Full},
		{"publish the rca", Full},
		{"resolve this observation and move it to payments", Full},
		{"create an annotation at the deploy", Full},
		{"save this search", Full},
		{"", Full},
		{"hello", Full},
		{"checkout", Full},
		{"tell me something interesting", Full},
		{"the frontend logs and traces since noon", Full},
		{"do the logs agree with the traces for checkout", Full},
	}
	for _, c := range cases {
		if got := Classify(Question{Types: allTypes, Text: c.text}); got != c.want {
			t.Errorf("%q: got %s, want %s", c.text, got.Name(), c.want.Name())
		}
	}
}

func TestClassifyReadsTheRunsShape(t *testing.T) {
	cases := []struct {
		name string
		q    Question
		want Task
	}{
		{"a change on a dashboard is a question about it", Question{Dashboard: true, Types: allTypes, Text: "remove the empty panels"}, Explain},
		{"a change on a panel is a question about it", Question{Dashboard: true, Panel: true, Types: allTypes, Text: "fix the query of this panel"}, Explain},
		{"a change word without a dashboard decides nothing", Question{Types: allTypes, Text: "remove the empty panels"}, Full},
		{"an investigation with no signal falls back", Question{Investigation: true, Types: allTypes, Text: "look at checkout"}, Full},
		{"an investigation keeps its signal", Question{Investigation: true, Types: allTypes, Text: "why did checkout fail at noon"}, Incident},
		{"trace words without a trace store decide nothing", Question{Types: []string{"prometheus"}, Text: "find a trace of checkout"}, Full},
		{"log words without a log store decide nothing", Question{Types: []string{"prometheus"}, Text: "show the cart logs"}, Full},
		{"log words fall to measure without a log store", Question{Types: []string{"prometheus"}, Text: "how many requests per second hit cart lines"}, Measure},
		{"clickhouse holds traces", Question{Types: []string{"clickhouse"}, Text: "find a trace of checkout"}, Traces},
		{"clickhouse holds logs", Question{Types: []string{"clickhouse"}, Text: "show the cart logs"}, Logs},
		{"an unknown store set does not block a type", Question{Text: "show the cart logs"}, Logs},
	}
	for _, c := range cases {
		if got := Classify(c.q); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got.Name(), c.want.Name())
		}
	}
}

func TestParseKnowsEveryTask(t *testing.T) {
	for _, task := range append([]Task{Full}, Tasks...) {
		if got, ok := Parse(string(task)); !ok || got != task {
			t.Errorf("Parse(%q) = %q, %v", task, got, ok)
		}
	}
	if _, ok := Parse("dashboard"); ok {
		t.Error("Parse accepts an unknown task")
	}
}
