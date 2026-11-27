package route

import (
	"slices"
	"strings"
	"unicode"
)

type Task string

const (
	Full      Task = ""
	Measure   Task = "measure"
	Logs      Task = "logs"
	Traces    Task = "traces"
	Incident  Task = "incident"
	Inventory Task = "inventory"
	Explain   Task = "explain"
)

var Tasks = []Task{Measure, Logs, Traces, Incident, Inventory, Explain}

func Parse(raw string) (Task, bool) {
	t := Task(raw)
	return t, t == Full || slices.Contains(Tasks, t)
}

func (t Task) Name() string {
	if t == Full {
		return "full"
	}
	return string(t)
}

type Question struct {
	Investigation bool
	Dashboard     bool
	Panel         bool
	Observation   bool
	Types         []string
	Text          string
}

func Classify(q Question) Task {
	w := words(q.Text)
	if len(w.list) == 0 || w.any(writeWords) || w.phrase(writePhrases) {
		return Full
	}
	if w.any(incidentWords) || w.phrase(incidentPhrases) {
		return Incident
	}
	if (q.Dashboard || q.Panel) && w.any(changeWords) {
		return Explain
	}
	if w.any(explainWords) || w.phrase(explainPhrases) || w.asksMeaning() {
		return Explain
	}
	return bySignal(w, signalsOf(w, q.Types))
}

func bySignal(w text, found signals) Task {
	switch {
	case found == signals{}:
		if w.phrase(inventoryPhrases) {
			return Inventory
		}
	case found.logs && found.traces:
		return Full
	case found.traces:
		return Traces
	case found.logs:
		return Logs
	case found.measure:
		return Measure
	}
	return Full
}

type signals struct{ logs, traces, measure bool }

func signalsOf(w text, types []string) signals {
	return signals{
		logs:    w.any(logWords) && hasAny(types, logTypes),
		traces:  (w.any(traceWords) || w.phrase(tracePhrases)) && hasAny(types, traceTypes),
		measure: w.any(measureWords) || w.phrase(measurePhrases),
	}
}

func hasAny(types, of []string) bool {
	return len(types) == 0 || slices.ContainsFunc(types, func(t string) bool { return slices.Contains(of, t) })
}

var (
	logTypes   = []string{"loki", "clickhouse", "elasticsearch", "opensearch"}
	traceTypes = []string{"tempo", "jaeger", "clickhouse"}
)

var (
	writeWords   = []string{"create", "save", "pin", "unpin", "publish", "resolve", "reopen", "archive", "annotate", "bookmark"}
	writePhrases = []string{
		"add a block", "add block", "add a note", "add an annotation", "create an annotation", "create an observation",
		"save a search", "save this search", "save the search", "move this", "move the observation", "move it to",
		"rename the", "rename this", "write it up", "write up", "file it", "set as home", "set my home",
	}

	incidentWords = []string{
		"why", "rca", "outage", "incident", "incidents", "broken", "broke", "failing", "down",
		"healthy", "unhealthy", "health", "degraded", "degradation", "regression", "investigate", "cause", "caused", "causes",
		"wrong", "happened", "happening", "anomaly", "anomalies", "alerting", "culprit", "postmortem", "outages",
	}
	incidentPhrases = []string{"root cause", "what changed", "what is going on", "what's going on", "going on with"}

	changeWords = []string{"remove", "delete", "change", "fix", "replace", "reorder", "resize", "add", "edit", "update", "convert"}

	explainWords   = []string{"explain", "summarize", "summarise", "summary", "rewrite", "meaning", "define", "definition", "describe"}
	explainPhrases = []string{
		"what is a", "what is an", "what is the difference", "difference between a",
		"how is this calculated", "how is it calculated", "how is the", "what is this panel", "what is this dashboard",
		"what am i looking at", "tell me about this",
	}

	inventoryPhrases = []string{
		"which services", "what services", "list the", "list all", "list every", "list services", "list metrics",
		"what metrics", "which metrics", "what dashboards", "which dashboards", "what datasources", "which datasources",
		"what data sources", "what labels", "which labels", "what exists", "what do we have", "what do we collect",
		"how many services", "which endpoints exist", "what endpoints", "which routes", "what routes", "show me all",
		"inventory", "what tables", "which tables", "what indices", "which indexes", "which indices",
	}

	logWords = []string{
		"log", "logs", "logged", "logging", "line", "lines", "message", "messages", "exception", "exceptions",
		"stacktrace", "stack", "panic", "panics", "warn", "warning", "warnings", "loglevel", "level",
	}

	traceWords   = []string{"trace", "traces", "traced", "span", "spans", "traceid", "downstream", "upstream", "hop", "hops"}
	tracePhrases = []string{
		"trace id", "slowest request", "slowest requests", "slow requests", "slow request", "call path", "critical path",
		"where do requests spend", "where does the time go", "where is the time spent", "which call", "self time",
		"first failed", "first fail", "originate", "originated",
	}

	measureWords = []string{
		"many", "much", "p50", "p90", "p95", "p99", "p999", "percentile", "quantile", "median", "rate", "rates", "peak",
		"peaked", "maximum", "max", "highest", "lowest", "minimum", "min", "average", "avg", "mean", "total", "sum", "count",
		"share", "percent", "percentage", "ratio", "throughput", "rps", "qps", "latency", "cpu", "memory", "utilization",
		"usage", "compare", "compared", "versus", "vs", "increase", "decrease", "grew", "drop", "dropped", "bytes",
		"requests", "duration", "long", "fastest", "slowest", "top", "error", "errors", "failed", "failures", "5xx", "4xx",
	}
	measurePhrases = []string{"error rate", "how fast", "how slow", "per second", "per minute", "right now", "an hour ago"}
)

var meaningEnds = []string{"mean", "do", "show", "shows", "measure", "represent", "track"}

func (t text) asksMeaning() bool {
	return t.phrase([]string{"what does", "what do"}) && slices.Contains(meaningEnds, t.list[len(t.list)-1])
}

type text struct {
	list   []string
	joined string
}

func words(s string) text {
	lower := strings.ToLower(s)
	list := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	})
	for i, w := range list {
		list[i] = strings.Trim(w, "'")
	}
	return text{list: list, joined: " " + strings.Join(list, " ") + " "}
}

func (t text) any(of []string) bool {
	return slices.ContainsFunc(t.list, func(w string) bool { return slices.Contains(of, w) })
}

func (t text) phrase(of []string) bool {
	return slices.ContainsFunc(of, func(p string) bool {
		return strings.Contains(t.joined, " "+p+" ")
	})
}
