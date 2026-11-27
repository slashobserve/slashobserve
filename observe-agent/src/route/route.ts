export const tasks = [
  "measure",
  "logs",
  "traces",
  "incident",
  "inventory",
  "explain",
] as const

export type Task = "" | (typeof tasks)[number]

export const full = ""

export function parseTask(raw: string): Task | undefined {
  return raw === full || (tasks as readonly string[]).includes(raw)
    ? (raw as Task)
    : undefined
}

export function taskName(task: Task): string {
  return task === full ? "full" : task
}

export type Question = {
  investigation?: boolean
  dashboard?: boolean
  panel?: boolean
  observation?: boolean
  types?: readonly string[]
  text: string
}

export function classify(q: Question): Task {
  const w = new Words(q.text)
  if (w.list.length === 0 || w.any(writeWords) || w.phrase(writePhrases)) {
    return full
  }
  if (w.any(incidentWords) || w.phrase(incidentPhrases)) {
    return "incident"
  }
  if ((q.dashboard || q.panel) && w.any(changeWords)) {
    return "explain"
  }
  if (w.any(explainWords) || w.phrase(explainPhrases) || w.asksMeaning()) {
    return "explain"
  }
  return bySignal(w, signalsOf(w, q.types ?? []))
}

type Signals = { logs: boolean; traces: boolean; measure: boolean }

function bySignal(w: Words, found: Signals): Task {
  if (!found.logs && !found.traces && !found.measure) {
    return w.phrase(inventoryPhrases) ? "inventory" : full
  }
  if (found.logs && found.traces) {
    return full
  }
  if (found.traces) {
    return "traces"
  }
  if (found.logs) {
    return "logs"
  }
  return "measure"
}

function signalsOf(w: Words, types: readonly string[]): Signals {
  return {
    logs: w.any(logWords) && hasAny(types, logTypes),
    traces:
      (w.any(traceWords) || w.phrase(tracePhrases)) &&
      hasAny(types, traceTypes),
    measure: w.any(measureWords) || w.phrase(measurePhrases),
  }
}

function hasAny(types: readonly string[], of: readonly string[]): boolean {
  return types.length === 0 || types.some((t) => of.includes(t))
}

const logTypes = ["loki", "clickhouse", "elasticsearch", "opensearch"]
const traceTypes = ["tempo", "jaeger", "clickhouse"]

const writeWords = [
  "create",
  "save",
  "pin",
  "unpin",
  "publish",
  "resolve",
  "reopen",
  "archive",
  "annotate",
  "bookmark",
]
const writePhrases = [
  "add a block",
  "add block",
  "add a note",
  "add an annotation",
  "create an annotation",
  "create an observation",
  "save a search",
  "save this search",
  "save the search",
  "move this",
  "move the observation",
  "move it to",
  "rename the",
  "rename this",
  "write it up",
  "write up",
  "file it",
  "set as home",
  "set my home",
]

const incidentWords = [
  "why",
  "rca",
  "outage",
  "incident",
  "incidents",
  "broken",
  "broke",
  "failing",
  "down",
  "healthy",
  "unhealthy",
  "health",
  "degraded",
  "degradation",
  "regression",
  "investigate",
  "cause",
  "caused",
  "causes",
  "wrong",
  "happened",
  "happening",
  "anomaly",
  "anomalies",
  "alerting",
  "culprit",
  "postmortem",
  "outages",
]
const incidentPhrases = [
  "root cause",
  "what changed",
  "what is going on",
  "what's going on",
  "going on with",
]

const changeWords = [
  "remove",
  "delete",
  "change",
  "fix",
  "replace",
  "reorder",
  "resize",
  "add",
  "edit",
  "update",
  "convert",
]

const explainWords = [
  "explain",
  "summarize",
  "summarise",
  "summary",
  "rewrite",
  "meaning",
  "define",
  "definition",
  "describe",
]
const explainPhrases = [
  "what is a",
  "what is an",
  "what is the difference",
  "difference between a",
  "how is this calculated",
  "how is it calculated",
  "how is the",
  "what is this panel",
  "what is this dashboard",
  "what am i looking at",
  "tell me about this",
]

const inventoryPhrases = [
  "which services",
  "what services",
  "list the",
  "list all",
  "list every",
  "list services",
  "list metrics",
  "what metrics",
  "which metrics",
  "what dashboards",
  "which dashboards",
  "what datasources",
  "which datasources",
  "what data sources",
  "what labels",
  "which labels",
  "what exists",
  "what do we have",
  "what do we collect",
  "how many services",
  "which endpoints exist",
  "what endpoints",
  "which routes",
  "what routes",
  "show me all",
  "inventory",
  "what tables",
  "which tables",
  "what indices",
  "which indexes",
  "which indices",
]

const logWords = [
  "log",
  "logs",
  "logged",
  "logging",
  "line",
  "lines",
  "message",
  "messages",
  "exception",
  "exceptions",
  "stacktrace",
  "stack",
  "panic",
  "panics",
  "warn",
  "warning",
  "warnings",
  "loglevel",
  "level",
]

const traceWords = [
  "trace",
  "traces",
  "traced",
  "span",
  "spans",
  "traceid",
  "downstream",
  "upstream",
  "hop",
  "hops",
]
const tracePhrases = [
  "trace id",
  "slowest request",
  "slowest requests",
  "slow requests",
  "slow request",
  "call path",
  "critical path",
  "where do requests spend",
  "where does the time go",
  "where is the time spent",
  "which call",
  "self time",
  "first failed",
  "first fail",
  "originate",
  "originated",
]

const measureWords = [
  "many",
  "much",
  "p50",
  "p90",
  "p95",
  "p99",
  "p999",
  "percentile",
  "quantile",
  "median",
  "rate",
  "rates",
  "peak",
  "peaked",
  "maximum",
  "max",
  "highest",
  "lowest",
  "minimum",
  "min",
  "average",
  "avg",
  "mean",
  "total",
  "sum",
  "count",
  "share",
  "percent",
  "percentage",
  "ratio",
  "throughput",
  "rps",
  "qps",
  "latency",
  "cpu",
  "memory",
  "utilization",
  "usage",
  "compare",
  "compared",
  "versus",
  "vs",
  "increase",
  "decrease",
  "grew",
  "drop",
  "dropped",
  "bytes",
  "requests",
  "duration",
  "long",
  "fastest",
  "slowest",
  "top",
  "error",
  "errors",
  "failed",
  "failures",
  "5xx",
  "4xx",
]
const measurePhrases = [
  "error rate",
  "how fast",
  "how slow",
  "per second",
  "per minute",
  "right now",
  "an hour ago",
]

const meaningEnds = [
  "mean",
  "do",
  "show",
  "shows",
  "measure",
  "represent",
  "track",
]

const separator = /[^\p{L}\p{Nd}']+/u

class Words {
  readonly list: string[]
  readonly joined: string

  constructor(s: string) {
    this.list = s
      .toLowerCase()
      .split(separator)
      .filter((w) => w !== "")
      .map((w) => w.replace(/^'+|'+$/g, ""))
    this.joined = " " + this.list.join(" ") + " "
  }

  any(of: readonly string[]): boolean {
    return this.list.some((w) => of.includes(w))
  }

  phrase(of: readonly string[]): boolean {
    return of.some((p) => this.joined.includes(" " + p + " "))
  }

  asksMeaning(): boolean {
    const last = this.list[this.list.length - 1]
    return (
      this.phrase(["what does", "what do"]) &&
      last !== undefined &&
      meaningEnds.includes(last)
    )
  }
}
