import { afterEach, describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import type { Hypothesis } from "../src/brief/hypotheses"
import { parseTime } from "../src/grounding/time"
import { McpClient } from "../src/mcp"
import { parseTask, type Task } from "../src/route/route"
import { cluster } from "../src/triage/cluster"
import {
  candidateProblems,
  checkCandidates,
  concludedOf,
  leadsOf,
  noteTransient,
  seedLedger,
} from "../src/triage/conclude"
import {
  burstStarts,
  changedPatterns,
  flagFirstSeen,
} from "../src/triage/events"
import { leads } from "../src/triage/leads"
import { median, onset } from "../src/triage/onset"
import {
  errorQuery,
  latencyQuery,
  pickMetrics,
  rateWindow,
  spanKinds,
} from "../src/triage/queries"
import { errorLines, metricRows } from "../src/triage/rates"
import { callToolOf, type Reader, ToolReader } from "../src/triage/reader"
import { clipText, render } from "../src/triage/render"
import { defaultLimits, sourcesOf, sweep, sweptStep } from "../src/triage/sweep"
import type {
  Candidate,
  Event,
  Lead,
  Origin,
  Pattern,
  Report,
  Row,
} from "../src/triage/types"
import { type FakeServer, startMcpServer } from "./mcpServer"

const fixtures = resolve(
  import.meta.dir,
  "..",
  "..",
  "spec",
  "examples",
  "agent",
  "triage"
)

function fixture<T>(name: string): T {
  return JSON.parse(readFileSync(resolve(fixtures, name), "utf8")) as T
}

function ms(text: string): number {
  return parseTime(text) as number
}

type RowJSON = {
  service: string
  signal: string
  rung: number
  window: number
  baseline: number
  unit: string
  source: string
  basis: string
  moved: boolean
  onset: string | null
}
type OriginJSON = Omit<Origin, "relayedBy"> & { relayed_by: string[] | null }
type EventJSON = Omit<Event, "at"> & { at: string }
type CandidateJSON = {
  id: string
  service: string
  onset: string | null
  rows: RowJSON[]
  knock_on: string[] | null
  origins: OriginJSON[] | null
  events: EventJSON[] | null
  patterns: Pattern[] | null
}

function rowOf(r: RowJSON): Row {
  return {
    ...r,
    rung: r.rung * 1000,
    onset: r.onset === null ? undefined : ms(r.onset),
  }
}

function rowJSON(r: Row): RowJSON {
  return {
    service: r.service,
    signal: r.signal,
    rung: r.rung / 1000,
    window: r.window,
    baseline: r.baseline,
    unit: r.unit,
    source: r.source,
    basis: r.basis,
    moved: r.moved,
    onset: r.onset === undefined ? null : stampMs(r.onset),
  }
}

function stampMs(at: number): string {
  return new Date(at).toISOString().replace(".000Z", "Z")
}

function originOf(o: OriginJSON): Origin {
  const { relayed_by, ...rest } = o
  return { ...rest, relayedBy: relayed_by ?? [] }
}

function originJSON(o: Origin): OriginJSON {
  const { relayedBy, ...rest } = o
  return { ...rest, relayed_by: relayedBy }
}

function eventOf(e: EventJSON): Event {
  return { ...e, at: ms(e.at) }
}

function eventJSON(e: Event): EventJSON {
  return { ...e, at: stampMs(e.at) }
}

function candidateOf(c: CandidateJSON): Candidate {
  return {
    id: c.id,
    service: c.service,
    onset: c.onset === null ? undefined : ms(c.onset),
    rows: (c.rows ?? []).map(rowOf),
    knockOn: c.knock_on ?? [],
    origins: (c.origins ?? []).map(originOf),
    events: (c.events ?? []).map(eventOf),
    patterns: c.patterns ?? [],
  }
}

function candidateJSON(c: Candidate): CandidateJSON {
  return {
    id: c.id,
    service: c.service,
    onset: c.onset === undefined ? null : stampMs(c.onset),
    rows: c.rows.map(rowJSON),
    knock_on: c.knockOn,
    origins: c.origins.map(originJSON),
    events: c.events.map(eventJSON),
    patterns: c.patterns,
  }
}

describe("onset", () => {
  type Fixture = {
    start: string
    onsets: {
      name: string
      values: (number | null)[]
      window_start: number
      rule: { factor: number; floor: number; sustain: number }
      want: number | null
    }[]
    medians: { in: number[]; want: number }[]
  }
  const fx = fixture<Fixture>("onset.json")
  const start = ms(fx.start)

  test("finds the onset Go finds", () => {
    for (const c of fx.onsets) {
      const points = c.values.map((v, i) => ({
        at: start + i * 60000,
        value: v ?? Number.NaN,
      }))
      const at = onset(points, start + c.window_start * 60000, c.rule)
      expect({
        name: c.name,
        got: at === undefined ? null : (at - start) / 60000,
      }).toEqual({
        name: c.name,
        got: c.want,
      })
    }
  })

  test("takes the median Go takes", () => {
    for (const c of fx.medians) {
      expect(median(c.in)).toBe(c.want)
    }
  })
})

describe("cluster and leads", () => {
  type Case = {
    name: string
    findings: {
      rows: RowJSON[] | null
      origins?: OriginJSON[] | null
      events?: EventJSON[] | null
      patterns?: Pattern[] | null
    }
    want: { candidates: CandidateJSON[]; background: RowJSON[]; leads: Lead[] }
  }
  for (const c of fixture<Case[]>("cluster.json")) {
    test(c.name, () => {
      const got = cluster({
        rows: (c.findings.rows ?? []).map(rowOf),
        origins: (c.findings.origins ?? []).map(originOf),
        events: (c.findings.events ?? []).map(eventOf),
        patterns: c.findings.patterns ?? [],
      })
      expect(got.candidates.map(candidateJSON)).toEqual(c.want.candidates)
      expect(got.background.map(rowJSON)).toEqual(c.want.background)
      expect(leads(got.candidates)).toEqual(c.want.leads)
    })
  }
})

type ReportJSON = {
  end: string
  question: { from: string; to: string } | null
  services: {
    source: string
    type: string
    signal: string
    field: string
    services: string[]
  }[]
  signals: string[]
  rates: RowJSON[]
  candidates: CandidateJSON[]
  background: RowJSON[]
  skipped: { step: string; source: string; reason: string; signals: string[] }[]
  calls: number
  duration_ms: number
}

function reportOf(r: ReportJSON): Report {
  return {
    end: ms(r.end),
    question:
      r.question === null
        ? undefined
        : { from: ms(r.question.from), to: ms(r.question.to) },
    services: r.services,
    signals: r.signals,
    rates: r.rates.map(rowOf),
    candidates: r.candidates.map(candidateOf),
    background: r.background.map(rowOf),
    skipped: r.skipped,
    calls: r.calls,
    duration: r.duration_ms,
  }
}

describe("render", () => {
  for (const c of fixture<
    { name: string; report: ReportJSON; budget: number; want: string }[]
  >("render.json")) {
    test(c.name, () => {
      expect(render(reportOf(c.report), c.budget)).toBe(c.want)
    })
  }

  test("clips text on whole runes", () => {
    const got = clipText("é".repeat(100))
    expect(got.endsWith("...")).toBe(true)
    expect([...got.slice(0, -3)].every((r) => r === "é")).toBe(true)
  })
})

describe("conclude", () => {
  type Fixture = {
    leads: Lead[]
    seed: {
      name: string
      loop: boolean
      task: string
      swept: boolean
      ledger: Hypothesis[]
      want: Hypothesis[]
    }[]
    problems: {
      name: string
      ledger: Hypothesis[]
      transient_at: Record<string, number> | null
      finding: unknown
      want: {
        problems: string[]
        open: string[]
        unclosed: string
        unverified: { kind: string; text: string }[]
      }
    }[]
    transient: {
      name: string
      steps: { reads: number; ledger: Hypothesis[] }[]
      want: Record<string, number>
    }[]
  }
  const fx = fixture<Fixture>("conclude.json")

  test("seeds the ledger as Go does", () => {
    for (const c of fx.seed) {
      const task = parseTask(c.task) as Task
      const got =
        seedLedger(
          leadsOf(c.loop, task, c.swept ? fx.leads : undefined),
          c.ledger
        ) ?? c.ledger
      expect({ name: c.name, got }).toEqual({ name: c.name, got: c.want })
    }
  })

  test("names what is unaccounted for as Go does", () => {
    for (const c of fx.problems) {
      const transientAt = c.transient_at ?? {}
      const { problems, open } = candidateProblems(
        fx.leads,
        c.ledger,
        transientAt,
        concludedOf(c.finding)
      )
      const check = checkCandidates(fx.leads, c.ledger, transientAt, c.finding)
      expect({
        name: c.name,
        got: {
          problems,
          open: open.map((l) => l.id),
          unclosed: check.unclosed,
          unverified: check.unverified,
        },
      }).toEqual({ name: c.name, got: c.want })
    }
  })

  test("marks a transient closing as Go does", () => {
    for (const c of fx.transient) {
      let ledger: Hypothesis[] = []
      let transientAt: Record<string, number> = {}
      for (const step of c.steps) {
        transientAt = noteTransient(
          fx.leads,
          ledger,
          step.ledger,
          transientAt,
          step.reads
        )
        ledger = step.ledger
      }
      expect({ name: c.name, got: transientAt }).toEqual({
        name: c.name,
        got: c.want,
      })
    }
  })
})

describe("rates and events", () => {
  const t0 = ms("2026-11-20T10:00:00Z")
  const pts = (start: number, step: number, ...values: number[]) =>
    values.map((value, i) => ({ at: start + i * step, value }))

  test("an error read that failed gives no error row", () => {
    const windowStart = t0 + 5 * 60000
    const req = new Map([
      ["cart", pts(t0, 60000, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)],
    ])
    const rising = new Map([
      ["cart", pts(t0, 60000, 0, 0, 0, 0, 0, 5, 5, 5, 5, 5)],
    ])
    const base: Row = {
      service: "",
      signal: "",
      rung: 3600000,
      window: 0,
      baseline: 0,
      unit: "",
      source: "",
      basis: "",
      moved: false,
    }
    expect(
      metricRows(base, windowStart, req, new Map(), new Map(), 1, false).map(
        (r) => r.signal
      )
    ).toEqual(["requests"])
    const rows = metricRows(base, windowStart, req, rising, new Map(), 1, true)
    expect(rows.map((r) => [r.signal, r.moved])).toEqual([
      ["requests", false],
      ["errors", true],
    ])
  })

  test("picks the metrics Go picks and writes its queries", () => {
    const span = pickMetrics(
      ["calls_total", "traces_span_metrics_calls_total"],
      [
        "traces_span_metrics_duration_seconds_bucket",
        "http_server_request_duration_seconds_bucket",
      ],
      "service_name"
    )
    expect(span).toMatchObject({
      basis: "span metrics",
      requests: "traces_span_metrics_calls_total{" + spanKinds + "}",
      scale: 1000,
    })
    expect(pickMetrics(["calls_total"], [], "service.name")).toBeUndefined()
    const http = pickMetrics(
      [],
      ["http_server_request_duration_seconds_bucket"],
      "service_name"
    )
    expect(errorQuery(http!, 120000)).toBe(
      `sum by (service_name) (rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[120s])) or sum by (service_name) (rate(http_server_request_duration_seconds_count{http_status_code=~"5.."}[120s]))`
    )
    expect(latencyQuery(http!, 360000)).toBe(
      "histogram_quantile(0.95, sum by (service_name, le) (rate(http_server_request_duration_seconds_bucket[360s])))"
    )
    expect(rateWindow(30000)).toBe(120000)
  })

  test("finds changed patterns, bursts, flags and error lines as Go does", () => {
    const changed = changedPatterns(
      [
        { text: "new error", level: "error", trend: [0, 0, 0, 2, 3] },
        { text: "steady", level: "info", trend: [5, 5, 5, 5, 5, 5] },
        { text: "grew", level: "warn", trend: [1, 1, 10, 10] },
        { text: "too few to be new", level: "", trend: [0, 0, 1, 1] },
      ],
      "loki",
      "payment"
    )
    expect(changed.map((p) => [p.text, p.baseline === 0])).toEqual([
      ["new error", true],
      ["grew", false],
    ])
    const sparse = [
      ...pts(t0, 60000, 3, 2),
      ...pts(t0 + 600000, 60000, 4, 1, 1),
    ]
    expect(
      burstStarts(
        [{ labels: { service_name: "cart" }, points: sparse }],
        "service_name",
        "loki"
      ).map((e) => e.at)
    ).toEqual([t0, t0 + 600000])
    const flags = flagFirstSeen(
      [
        {
          labels: {
            feature_flag_key: "paymentFailure",
            feature_flag_result_variant: "50%",
          },
          points: pts(t0, 600000, 0, 0, 0, 4, 5),
        },
        {
          labels: {
            feature_flag_key: "paymentFailure",
            feature_flag_variant: "off",
          },
          points: pts(t0, 600000, 9, 9, 9, 5, 4),
        },
        { labels: { other: "x" }, points: pts(t0, 60000, 1) },
      ],
      "loki",
      t0
    )
    expect(flags.map((e) => e.text)).toEqual([
      "paymentFailure variant off already evaluated when the read began",
      "paymentFailure variant 50% first seen",
    ])
    const lines = errorLines(
      [
        {
          labels: { service_name: "cart", detected_level: "info" },
          points: pts(t0, 60000, 5, 5, 5),
        },
        {
          labels: { service_name: "cart", detected_level: "ERROR" },
          points: [{ at: t0 + 120000, value: 7 }],
        },
      ],
      "service_name"
    )
    expect(lines.get("cart")).toEqual(pts(t0, 60000, 0, 0, 7))
  })
})

describe("sources", () => {
  test("sweeps only what everyone reads, never the audit datasource", () => {
    expect(
      sourcesOf([
        { uid: "tempo", type: "tempo", everyone: true },
        { uid: "prom", type: "prometheus", everyone: true },
        { uid: "slashobserve-audit", type: "elasticsearch", everyone: true },
        { uid: "secret-logs", type: "loki", everyone: false },
        { uid: "pg", type: "postgres", everyone: true },
      ])
    ).toEqual([
      { uid: "prom", type: "prometheus" },
      { uid: "tempo", type: "tempo" },
    ])
  })
})

type Transcript = {
  name: string
  sources: { uid: string; type: string }[]
  end: string
  question?: { from: string; to: string }
  calls: { tool: string; input: unknown; content: string; is_error?: boolean }[]
  want: { block: string; leads: Lead[]; calls: number }
}

function canonical(v: unknown): string {
  if (Array.isArray(v)) {
    return "[" + v.map(canonical).join(",") + "]"
  }
  if (v !== null && typeof v === "object") {
    const entries = Object.entries(v as Record<string, unknown>).sort(
      ([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)
    )
    return (
      "{" +
      entries
        .map(([k, x]) => JSON.stringify(k) + ":" + canonical(x))
        .join(",") +
      "}"
    )
  }
  return JSON.stringify(v)
}

const masked = (block: string) =>
  block.replace(/Sweep: (\d+) reads in [^\n]*\./, "Sweep: $1 reads in TIME.")

const running: FakeServer[] = []
const clients: McpClient[] = []

afterEach(async () => {
  await Promise.all(clients.splice(0).map((c) => c.close()))
  for (const server of running.splice(0)) {
    server.stop()
  }
})

describe("sweep", () => {
  for (const c of fixture<Transcript[]>("sweep.json")) {
    test(c.name, async () => {
      const answers = new Map(
        c.calls.map((call) => [call.tool + " " + canonical(call.input), call])
      )
      const missing: string[] = []
      const server = startMcpServer("sot_sweep", (seen) => {
        const key = seen.name + " " + canonical(seen.args)
        const call = answers.get(key)
        if (!call) {
          missing.push(key)
          return { text: `{"code":"not_recorded"}`, isError: true }
        }
        return { text: call.content, isError: call.is_error === true }
      })
      running.push(server)
      const client = new McpClient(server.url, "sot_sweep")
      clients.push(client)
      const out = await sweep(new ToolReader(callToolOf(client, 5000)), {
        sources: c.sources,
        end: ms(c.end),
        question: c.question
          ? { from: ms(c.question.from), to: ms(c.question.to) }
          : undefined,
      })
      expect(missing).toEqual([])
      expect(masked(out.block)).toBe(c.want.block)
      expect(out.leads).toEqual(c.want.leads)
      expect(out.calls).toBe(c.want.calls)
      expect(server.calls.length).toBe(c.calls.length)
      expect(sweptStep(out)).toMatch(
        /^Swept the datasources before the first model call: \d+ reads in \S+$/
      )
    })
  }
})

function slowReader(delay: number, fail = ""): Reader & { calls: number } {
  const wait = (abort: AbortSignal) =>
    new Promise<void>((done, reject) => {
      reader.calls++
      const timer = setTimeout(done, delay)
      abort.addEventListener("abort", () => {
        clearTimeout(timer)
        reject(new Error("aborted"))
      })
    })
  const reader = {
    calls: 0,
    async services(uid: string, _signal: string, abort: AbortSignal) {
      if (uid === fail) {
        reader.calls++
        throw new Error("boom")
      }
      await wait(abort)
      return { field: "service_name", names: ["a", "b"] }
    },
    async metricNames(_uid: string, _search: string, abort: AbortSignal) {
      await wait(abort)
      return [
        "traces_span_metrics_calls_total",
        "traces_span_metrics_duration_seconds_bucket",
      ]
    },
    async range(
      _u: string,
      _q: string,
      _r: unknown,
      _p: number,
      abort: AbortSignal
    ) {
      await wait(abort)
      return []
    },
    async traceStats(_u: string, _i: unknown, abort: AbortSignal) {
      await wait(abort)
      return { groups: [{ value: "k", spans: 1, errors: 0 }], buckets: [] }
    },
    async errorOrigins(_u: string, _r: unknown, abort: AbortSignal) {
      await wait(abort)
      return []
    },
    async patterns(_u: string, _q: string, _r: unknown, abort: AbortSignal) {
      await wait(abort)
      return []
    },
    async annotations(_r: unknown, abort: AbortSignal) {
      await wait(abort)
      return []
    },
  }
  return reader
}

function manySources(): { uid: string; type: string }[] {
  return [
    "prometheus",
    "loki",
    "tempo",
    "clickhouse",
    "jaeger",
    "elasticsearch",
    "opensearch",
  ].flatMap((t) => ["1", "2", "3"].map((n) => ({ uid: t + n, type: t })))
}

describe("sweep limits", () => {
  const end = ms("2026-11-20T12:00:00Z")

  test("keeps to its call budget", async () => {
    const reader = slowReader(0)
    const out = await sweep(
      reader,
      { sources: manySources(), end },
      { calls: 9, time: 5000, concurrency: 3 }
    )
    expect(reader.calls).toBeLessThanOrEqual(9)
    expect(out.calls).toBe(9)
    expect(out.block).toContain("call budget was spent first")
  })

  test("keeps to its time limit", async () => {
    const started = performance.now()
    const out = await sweep(
      slowReader(3_600_000),
      { sources: manySources(), end },
      { calls: 40, time: 200, concurrency: 4 }
    )
    expect(performance.now() - started).toBeLessThan(2000)
    expect(out.block).toContain("time limit")
    expect(out.block.endsWith("</triage>")).toBe(true)
  })

  test("skips a failing store and continues", async () => {
    const out = await sweep(slowReader(0, "loki1"), {
      sources: [
        { uid: "loki1", type: "loki" },
        { uid: "prometheus1", type: "prometheus" },
      ],
      end,
    })
    expect(out.block).toContain(
      "Skipped: service map on loki1: the read failed: boom."
    )
    expect(out.block).toContain("Services on prometheus1")
  })

  test("words the sweep's step as runsession does", () => {
    expect(sweptStep({ calls: 17, duration: 3249 })).toBe(
      "Swept the datasources before the first model call: 17 reads in 3.2s"
    )
    expect(sweptStep({ calls: 0, duration: 40 })).toBe(
      "Swept the datasources before the first model call: 0 reads in 0s"
    )
  })

  test("uses the default limits for unset ones", () => {
    expect(defaultLimits).toEqual({ calls: 40, time: 20_000, concurrency: 4 })
  })
})
