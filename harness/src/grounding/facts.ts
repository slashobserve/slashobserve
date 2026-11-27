import { byteLength, clipBytes, parseFloat, trimSpace } from "../brief/gofmt"
import { encode, get, type Node, parseNode, set, str } from "./node"
import {
  addFact,
  type Fact,
  type FactKind,
  type GroundingRecord,
  maxNameBytes,
  type Range,
  traceID,
} from "./record"
import { formatTime, parseTime } from "./time"

const maxFactsOfRead = 120
const maxFactLabels = 12
const maxFactQuery = 400
const maxFactText = 64
const factsKey = "facts"
const logsStore = "loki"
const serverPatterns = "server"

export const signalMetrics = "metrics"
export const signalLogs = "logs"
export const signalTraces = "traces"

type FactSource = Record<string, string[]>

export const factSources: Record<string, FactSource> = {
  measure_window: { "values[]": ["value", "seconds_after_from"] },
  measure_aggregate: { "values[]": ["value", "numerator", "denominator"] },
  trace_stats: {
    "": ["spans", "errors", "quantile_spans"],
    window: ["p50_ms", "p95_ms", "p99_ms"],
    "groups[]": [
      "spans",
      "errors",
      "p50_ms",
      "p95_ms",
      "p99_ms",
      "quantile_spans",
    ],
  },
  trace_error_origins: {
    "": ["failing_traces", "examined", "unattributed"],
    "routes[]": ["failing_traces", "examined"],
    "groups[]": ["traces", "share", "example_trace_id"],
    "groups[].relayed_by[]": ["traces"],
  },
  trace_slow_path: {
    "": ["listed_traces"],
    "routes[]": ["threshold_ms", "p50_ms", "slow_traces", "examined"],
    "routes[].top_self_time[]": ["self_time_ms", "share", "traces"],
    "routes[].services[]": ["traces"],
    "routes[].example": ["trace_id", "duration_ms"],
  },
  get_trace: {
    "": ["trace_id", "spans", "duration_ms"],
    "services[]": ["spans", "errors"],
    "slowest[]": ["duration_ms", "self_time_ms"],
  },
  query_range: { "series[]": ["min", "max", "last"] },
  query_instant: { "series[]": ["last"] },
  log_facets: {
    "": ["lines_sampled"],
    "fields[].values[]": ["count"],
  },
  log_patterns: {
    "": ["lines_matched"],
    "patterns[]": ["count"],
  },
}

export function issuesFacts(tool: string): boolean {
  return Object.hasOwn(factSources, tool)
}

const labelFields = new Set([
  "route",
  "service",
  "operation",
  "name",
  "trace_id",
  "status_message",
  "span_id",
])

const countFields = new Set([
  "spans",
  "errors",
  "traces",
  "failing_traces",
  "slow_traces",
  "examined",
  "listed_traces",
  "unattributed",
  "quantile_spans",
  "count",
  "lines_sampled",
  "lines_matched",
])

const factQueries: Record<string, string> = {
  numerator: "numerator_query",
  denominator: "denominator_query",
}

function kindOf(key: string): [FactKind, string] {
  if (key === "trace_id" || key === "example_trace_id") {
    return ["trace_id", ""]
  }
  if (key.endsWith("_ms")) {
    return ["duration", "ms"]
  }
  if (key === "seconds_after_from") {
    return ["duration", "s"]
  }
  if (key === "share") {
    return ["ratio", ""]
  }
  if (countFields.has(key)) {
    return ["count", ""]
  }
  return ["number", ""]
}

type Scope = {
  labels: Record<string, string> | undefined
  sampled: boolean
  unit: string
}

type ObjectNode = Node & { kind: "object" }

class Issuing {
  from = ""
  to = ""
  query = ""
  asked = ""
  qualify = ""
  signal = ""
  issued = 0

  constructor(
    readonly record: GroundingRecord,
    readonly tool: string,
    readonly source: FactSource,
    readonly top: ObjectNode
  ) {}

  window(ranges: readonly Range[]): void {
    const from = str(this.top, "from")
    const to = str(this.top, "to")
    if (parseTime(from) !== undefined && parseTime(to) !== undefined) {
      this.from = from
      this.to = to
      return
    }
    const first = ranges[0]
    if (first) {
      this.from = formatTime(first.from)
      this.to = formatTime(first.to)
    }
  }

  visit(n: ObjectNode, path: string, inherited: Scope): void {
    const at = this.scopeOf(n, path, inherited)
    const keys = Object.hasOwn(this.source, path)
      ? this.source[path]
      : undefined
    if (keys && get(n, factsKey) === undefined) {
      const ids: ObjectNode = { kind: "object", keys: [], vals: [] }
      for (const key of keys) {
        const id = this.fact(n, path, key, at)
        if (id !== "") {
          set(ids, key, { kind: "string", text: id })
        }
      }
      if (ids.keys.length > 0) {
        set(n, factsKey, ids)
      }
    }
    n.keys.forEach((key, i) => {
      const child = n.vals[i] as Node
      const under = path === "" ? key : path + "." + key
      if (child.kind === "object") {
        this.visit(child, under, at)
      } else if (child.kind === "array") {
        for (const item of child.vals) {
          if (item.kind === "object") {
            this.visit(item, under + "[]", at)
          }
        }
      }
    })
  }

  scopeOf(n: ObjectNode, path: string, inherited: Scope): Scope {
    const out: Scope = {
      ...inherited,
      labels: this.labelsOf(n, path, inherited.labels),
    }
    if (this.tool === "log_facets" && path === "fields[]") {
      const exact = get(n, "exact")
      const text =
        exact && exact.kind !== "object" && exact.kind !== "array"
          ? exact.text
          : ""
      out.sampled = text !== "true"
      out.unit = str(n, "unit")
    } else if (this.tool === "log_patterns" && path === "") {
      out.sampled = str(n, "source") === serverPatterns
    }
    return out
  }

  labelsOf(
    n: ObjectNode,
    path: string,
    inherited: Record<string, string> | undefined
  ): Record<string, string> | undefined {
    let labels = inherited ? { ...inherited } : undefined
    const add = (key: string, value: string) => {
      labels ??= {}
      if (
        Object.keys(labels).length < maxFactLabels &&
        value !== "" &&
        byteLength(value) <= maxNameBytes
      ) {
        labels[key] = value
      }
    }
    const own = get(n, "labels")
    if (own?.kind === "object") {
      own.keys.forEach((key, i) => {
        const v = own.vals[i] as Node
        if (v.kind === "string") {
          add(key, v.text)
        }
      })
    }
    n.keys.forEach((key, i) => {
      const v = n.vals[i] as Node
      if (v.kind !== "string") {
        return
      }
      const label = this.labelOf(path, key, v.text)
      if (label) {
        add(label[0], label[1])
      }
    })
    return labels
  }

  labelOf(
    path: string,
    key: string,
    text: string
  ): [string, string] | undefined {
    if (labelFields.has(key)) {
      return [key, text]
    }
    if (
      key === "value" &&
      path === "groups[]" &&
      str(this.top, "group_by") !== ""
    ) {
      return [str(this.top, "group_by"), text]
    }
    if (key === "value" && path === "fields[].values[]") {
      return [key, text]
    }
    if ((key === "pattern" || key === "level") && path === "patterns[]") {
      return [key, clipBytes(text, maxNameBytes)]
    }
    return undefined
  }

  fact(n: ObjectNode, path: string, key: string, at: Scope): string {
    const value = get(n, key)
    if (value === undefined || this.issued >= maxFactsOfRead) {
      return ""
    }
    const [kind, ownUnit] = kindOf(key)
    const unit = at.unit !== "" ? at.unit : ownUnit
    let labels = at.labels
    let query = clipBytes(this.query, maxFactQuery)
    const q = factQueries[key]
    if (q !== undefined) {
      query = clipBytes(str(this.top, q), maxFactQuery)
    }
    let text: string
    let v: number | undefined
    if (
      kind === "trace_id" &&
      value.kind === "string" &&
      traceID.test(value.text)
    ) {
      text = value.text.toLowerCase()
      labels = withoutLabel(labels, "trace_id")
    } else if (
      kind !== "trace_id" &&
      value.kind === "number" &&
      value.text.length <= maxFactText
    ) {
      const parsed = parseFloat(value.text)
      if (parsed === undefined) {
        return ""
      }
      text = value.text
      v = parsed
    } else {
      return ""
    }
    const f: Omit<Fact, "id"> = { text, kind, of: this.of(path, key) }
    if (v !== undefined && v !== 0) {
      f.value = v
    }
    if (unit !== "") {
      f.unit = unit
    }
    if (labels && Object.keys(labels).length > 0) {
      f.labels = labels
    }
    const stamp = str(n, "at")
    if (stamp !== "") {
      f.at = stamp
    }
    if (this.from !== "") {
      f.from = this.from
    }
    if (this.to !== "") {
      f.to = this.to
    }
    if (query !== "") {
      f.query = query
    }
    if (this.asked !== "") {
      f.asked = this.asked
    }
    if (at.sampled && kind === "count") {
      f.sampled = true
    }
    if (this.signal !== "") {
      f.signal = this.signal
    }
    this.issued++
    return addFact(this.record, f)
  }

  of(path: string, key: string): string {
    const parts = [this.tool]
    if (this.qualify !== "") {
      parts.push(this.qualify)
    }
    if (path !== "") {
      parts.push(path)
    }
    parts.push(key)
    return parts.join(" ")
  }
}

function withoutLabel(
  labels: Record<string, string> | undefined,
  key: string
): Record<string, string> | undefined {
  if (!labels || !Object.hasOwn(labels, key)) {
    return labels
  }
  const out = { ...labels }
  delete out[key]
  return Object.keys(out).length === 0 ? undefined : out
}

export function issueFacts(
  r: GroundingRecord | undefined,
  tool: string,
  input: string,
  content: string,
  ranges: readonly Range[]
): string {
  const source = Object.hasOwn(factSources, tool)
    ? factSources[tool]
    : undefined
  if (!r || !source) {
    return content
  }
  const top = parseNode(content)
  if (top?.kind !== "object") {
    return content
  }
  const is = new Issuing(r, tool, source, top)
  is.window(ranges)
  const parsedInput = parseNode(input)
  if (parsedInput?.kind === "object") {
    is.asked = clipBytes(str(parsedInput, "query"), maxFactQuery)
  }
  is.query = str(top, "query")
  if (is.query === "") {
    const executed = get(top, "executed")
    const first = executed && "vals" in executed ? executed.vals[0] : undefined
    if (first) {
      is.query = str(first, "text")
    }
  }
  if (is.query === "") {
    is.query = is.asked
  }
  is.qualify = trimSpace(str(top, "stat") + " " + str(top, "op"))
  if (tool === "measure_aggregate" && str(top, "store") === logsStore) {
    is.signal = signalLogs
  }
  is.visit(top, "", { labels: undefined, sampled: false, unit: "" })
  return is.issued === 0 ? content : encode(top)
}
