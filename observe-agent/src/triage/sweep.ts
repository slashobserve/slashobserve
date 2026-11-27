import { compareBytes, formatDuration } from "../brief/gofmt"
import { cluster, type Move, movesOf } from "./cluster"
import {
  burstStarts,
  changedPatterns,
  flagEvent,
  flagFirstSeen,
  topGroups,
} from "./events"
import { leads } from "./leads"
import {
  attrEquals,
  basisLogLevels,
  basisTraceSpans,
  defaultLabel,
  deployLineQuery,
  errorQuery,
  flagKeyAttr,
  flagQuery,
  flagVariants,
  labelName,
  latencyQuery,
  levelLabel,
  logFlagQuery,
  logLevelQuery,
  lokiSelector,
  type MetricPlan,
  pickMetrics,
  planNames,
  rateWindow,
  requestQuery,
  traceAll,
  traceService,
  traceServiceKey,
} from "./queries"
import {
  bucketOnsets,
  byService,
  errorLines,
  logRows,
  metricRows,
  type Series,
  topRates,
  traceRows,
} from "./rates"
import { type Reader, type TraceStats } from "./reader"
import { clipText, maxBlockBytes, render, roundDuration, short } from "./render"
import {
  type Event,
  type Findings,
  type Lead,
  type Origin,
  type Report,
  type Row,
  rungs,
  type ServiceSource,
  type Skip,
  signalErrors,
  signalLatency,
  signalRequests,
  type Window,
} from "./types"

export type Limits = { calls: number; time: number; concurrency: number }

export const defaultLimits: Limits = { calls: 40, time: 20_000, concurrency: 4 }

export type Source = { uid: string; type: string }

export type Input = {
  sources: Source[]
  end: number
  question?: Window | undefined
}

export type Outcome = {
  block: string
  leads: Lead[]
  calls: number
  duration: number
}

const rungPoints = 120
const eventPoints = 144
const maxMetricStores = 2
const maxFocus = 3
const maxFlagKeys = 3
const maxFlagPairs = 4
const traceBuckets = 60

const comparedSignals = [signalRequests, signalErrors, signalLatency]

const signalMetrics = "metrics"
const signalLogs = "logs"
const signalTraces = "traces"

export function serviceSignals(type: string): string[] {
  switch (type) {
    case "loki":
    case "elasticsearch":
    case "opensearch":
      return [signalLogs]
    case "tempo":
    case "jaeger":
      return [signalTraces]
    case "prometheus":
      return [signalMetrics]
    case "clickhouse":
      return [signalLogs, signalTraces]
  }
  return []
}

export const auditUID = "slashobserve-audit"

export function sourcesOf(
  datasources: readonly {
    uid: string
    type: string
    everyone?: boolean | undefined
  }[]
): Source[] {
  return datasources
    .filter(
      (d) =>
        d.everyone === true &&
        d.uid !== auditUID &&
        serviceSignals(d.type).length > 0
    )
    .map((d) => ({ uid: d.uid, type: d.type }))
    .sort((a, b) => compareBytes(a.uid, b.uid))
}

class Slots {
  private free: number
  private readonly waiting: (() => void)[] = []

  constructor(n: number) {
    this.free = n
  }

  acquire(abort: AbortSignal): Promise<boolean> {
    if (abort.aborted) {
      return Promise.resolve(false)
    }
    if (this.free > 0) {
      this.free--
      return Promise.resolve(true)
    }
    return new Promise((resolve) => {
      const wake = () => {
        abort.removeEventListener("abort", stop)
        resolve(true)
      }
      const stop = () => {
        const i = this.waiting.indexOf(wake)
        if (i >= 0) {
          this.waiting.splice(i, 1)
        }
        resolve(false)
      }
      this.waiting.push(wake)
      abort.addEventListener("abort", stop, { once: true })
    })
  }

  release(): void {
    const next = this.waiting.shift()
    if (next) {
      next()
    } else {
      this.free++
    }
  }
}

const timeLimitFirst = "the sweep's time limit was reached first"
const budgetFirst = "the sweep's call budget was spent first"
const timeLimitDuring = "the sweep's time limit ran out during the read"

function finest(m: Move): number {
  let w = rungs[rungs.length - 1] as number
  for (const r of m.rows) {
    w = Math.min(w, r.rung)
  }
  return w
}

class Sweep {
  calls = 0
  readonly fields = new Map<string, string>()
  readonly services: ServiceSource[] = []
  readonly signals: string[] = []
  readonly skipped: Skip[] = []
  readonly findings: Findings = {
    rows: [],
    origins: [],
    events: [],
    patterns: [],
  }
  readonly rates: Row[] = []
  private readonly slots: Slots

  constructor(
    readonly r: Reader,
    readonly lim: Limits,
    readonly input: Input,
    readonly abort: AbortSignal
  ) {
    this.slots = new Slots(lim.concurrency)
  }

  private take(): boolean {
    if (this.calls >= this.lim.calls) {
      return false
    }
    this.calls++
    return true
  }

  skip(
    step: string,
    uid: string,
    reason: string,
    signals: readonly string[] = []
  ): void {
    this.skipped.push({ step, source: uid, reason, signals: [...signals] })
  }

  async callFor(
    signals: readonly string[],
    step: string,
    uid: string,
    read: (abort: AbortSignal) => Promise<void>
  ): Promise<boolean> {
    if (this.abort.aborted || !(await this.slots.acquire(this.abort))) {
      this.skip(step, uid, timeLimitFirst, signals)
      return false
    }
    try {
      if (!this.take()) {
        this.skip(step, uid, budgetFirst, signals)
        return false
      }
      try {
        await read(this.abort)
        return true
      } catch (err) {
        if (this.abort.aborted) {
          this.skip(step, uid, timeLimitDuring, signals)
        } else {
          const message = err instanceof Error ? err.message : String(err)
          this.skip(step, uid, "the read failed: " + clipText(message), signals)
        }
        return false
      }
    } finally {
      this.slots.release()
    }
  }

  call(
    step: string,
    uid: string,
    read: (abort: AbortSignal) => Promise<void>
  ): Promise<boolean> {
    return this.callFor([], step, uid, read)
  }

  async serviceMap(): Promise<void> {
    const tasks: Promise<void>[] = []
    for (const src of this.input.sources) {
      for (const signal of serviceSignals(src.type)) {
        tasks.push(
          (async () => {
            let found = { field: "", names: [] as string[] }
            if (
              !(await this.call("service map", src.uid, async (abort) => {
                found = await this.r.services(src.uid, signal, abort)
              }))
            ) {
              return
            }
            this.fields.set(src.uid + "/" + signal, found.field)
            this.services.push({
              source: src.uid,
              type: src.type,
              signal,
              field: found.field,
              services: [...found.names].sort(compareBytes),
            })
          })()
        )
      }
    }
    await Promise.all(tasks)
  }

  field(uid: string, signal: string): string {
    return this.fields.get(uid + "/" + signal) || defaultLabel
  }

  holds(src: Source, signal: string): boolean {
    return this.services.some(
      (s) =>
        s.source === src.uid && s.signal === signal && s.services.length > 0
    )
  }

  sources(signal: string, ...types: string[]): Source[] {
    const out: Source[] = []
    for (const t of types) {
      for (const src of this.input.sources) {
        if (src.type === t && this.holds(src, signal)) {
          out.push(src)
        }
      }
    }
    return out
  }

  addRows(rows: readonly Row[]): void {
    for (const r of rows) {
      if (r.signal === signalRequests) {
        this.rates.push(r)
        continue
      }
      this.findings.rows.push(r)
      if (r.rung === rungs[0]) {
        this.rates.push(r)
      }
    }
  }

  rung(w: number): { rng: Window; windowStart: number } {
    return {
      rng: { from: this.input.end - 2 * w, to: this.input.end },
      windowStart: this.input.end - w,
    }
  }

  async measure(): Promise<void> {
    await this.metricRates()
    if (this.findings.rows.length === 0) {
      await this.traceRates()
    }
    if (this.findings.rows.length === 0) {
      await this.logRates()
    }
  }

  focus(): Move[] {
    return movesOf(this.findings.rows).slice(0, maxFocus)
  }

  async metricRates(): Promise<void> {
    const srcs = this.sources(signalMetrics, "prometheus")
    if (srcs.length === 0) {
      this.skip("metric rates", "", "no Prometheus store lists services")
      return
    }
    for (const src of srcs.slice(0, maxMetricStores)) {
      const plan = await this.metricPlan(src)
      if (!plan) {
        continue
      }
      this.signals.push(
        `requests, errors and p95 latency per service on ${src.uid} from ${plan.basis} ${planNames(plan)}`
      )
      await Promise.all(rungs.map((w) => this.metricRung(src, plan, w)))
    }
  }

  async metricPlan(src: Source): Promise<MetricPlan | undefined> {
    let calls: string[] = []
    let durations: string[] = []
    const [okCalls, okDurations] = await Promise.all([
      this.callFor(comparedSignals, "metric names", src.uid, async (abort) => {
        calls = await this.r.metricNames(src.uid, "calls_total", abort)
      }),
      this.callFor(comparedSignals, "metric names", src.uid, async (abort) => {
        durations = await this.r.metricNames(src.uid, "duration", abort)
      }),
    ])
    if (!okCalls && !okDurations) {
      return undefined
    }
    const plan = pickMetrics(
      calls,
      durations,
      this.field(src.uid, signalMetrics)
    )
    if (!plan) {
      this.skip(
        "metric rates",
        src.uid,
        "no span-metric calls_total counter or HTTP server duration histogram was found"
      )
    }
    return plan
  }

  async metricRung(src: Source, plan: MetricPlan, w: number): Promise<void> {
    const { rng, windowStart } = this.rung(w)
    const rate = rateWindow(Math.trunc((2 * w) / rungPoints))
    const reads: {
      signals: string[]
      query: string
      got: Series[]
      ok: boolean
    }[] = [
      {
        signals: comparedSignals,
        query: requestQuery(plan, rate),
        got: [],
        ok: false,
      },
      {
        signals: [signalErrors],
        query: errorQuery(plan, rate),
        got: [],
        ok: false,
      },
    ]
    if (plan.bucket !== "") {
      reads.push({
        signals: [signalLatency],
        query: latencyQuery(plan, rate),
        got: [],
        ok: false,
      })
    }
    const step = "metric rates over " + short(w)
    await Promise.all(
      reads.map(async (read) => {
        read.ok = await this.callFor(
          read.signals,
          step,
          src.uid,
          async (abort) => {
            read.got = await this.r.range(
              src.uid,
              read.query,
              rng,
              rungPoints,
              abort
            )
          }
        )
      })
    )
    const base: Row = {
      service: "",
      signal: "",
      rung: w,
      window: 0,
      baseline: 0,
      unit: "",
      source: src.uid,
      basis: plan.basis,
      moved: false,
    }
    const [req, errs] = reads as [(typeof reads)[0], (typeof reads)[0]]
    this.addRows(
      metricRows(
        base,
        windowStart,
        byService(req.got, plan.label),
        byService(errs.got, plan.label),
        byService(reads[2]?.got ?? [], plan.label),
        plan.scale,
        errs.ok
      )
    )
  }

  async traceRates(): Promise<void> {
    const srcs = this.sources(signalTraces, "tempo", "clickhouse")
    const src = srcs[0]
    if (!src) {
      this.skip(
        "trace rates",
        "",
        "no metric rates were read and no Tempo or ClickHouse traces store lists services"
      )
      return
    }
    this.signals.push(
      `span counts, errors and p95 duration per service on ${src.uid} from ${basisTraceSpans}`
    )
    await Promise.all(rungs.map((w) => this.traceRung(src, w)))
    await this.traceOnsets(src)
  }

  async traceRung(src: Source, w: number): Promise<void> {
    let now: TraceStats = { groups: [], buckets: [] }
    let before: TraceStats = { groups: [], buckets: [] }
    const step = "trace rates over " + short(w)
    const end = this.input.end
    const [nowOK, beforeOK] = await Promise.all([
      this.callFor(comparedSignals, step, src.uid, async (abort) => {
        now = await this.r.traceStats(
          src.uid,
          {
            query: traceAll(src.type),
            groupBy: traceServiceKey,
            range: { from: end - w, to: end },
            buckets: 1,
          },
          abort
        )
      }),
      this.callFor(comparedSignals, step, src.uid, async (abort) => {
        before = await this.r.traceStats(
          src.uid,
          {
            query: traceAll(src.type),
            groupBy: traceServiceKey,
            range: { from: end - 2 * w, to: end - w },
            buckets: 1,
          },
          abort
        )
      }),
    ])
    if (nowOK && beforeOK) {
      this.addRows(
        traceRows(
          {
            service: "",
            signal: "",
            rung: w,
            window: 0,
            baseline: 0,
            unit: "",
            source: src.uid,
            basis: basisTraceSpans,
            moved: false,
          },
          now.groups,
          before.groups
        )
      )
    }
  }

  async traceOnsets(src: Source): Promise<void> {
    await Promise.all(
      this.focus().map(async (m) => {
        const w = finest(m)
        const { rng, windowStart } = this.rung(w)
        let stats: TraceStats = { groups: [], buckets: [] }
        if (
          !(await this.call("trace onsets", src.uid, async (abort) => {
            stats = await this.r.traceStats(
              src.uid,
              {
                query: traceService(src.type, m.service),
                groupBy: "",
                range: rng,
                buckets: traceBuckets,
              },
              abort
            )
          }))
        ) {
          return
        }
        const onsets = bucketOnsets(stats.buckets, windowStart)
        for (const r of this.findings.rows) {
          if (r.service === m.service && r.rung === w && r.moved) {
            r.onset = onsets[r.signal]
          }
        }
      })
    )
  }

  async logRates(): Promise<void> {
    const src = this.sources(signalLogs, "loki")[0]
    if (!src) {
      this.skip(
        "log rates",
        "",
        "no metric or trace rates were read and no Loki store lists services"
      )
      return
    }
    const label = this.field(src.uid, signalLogs)
    if (!labelName.test(label)) {
      this.skip("log rates", src.uid, "the service field is not a stream label")
      return
    }
    this.signals.push(
      `error-level log lines per service on ${src.uid} from ${basisLogLevels} in ${levelLabel}`
    )
    await Promise.all(
      rungs.map(async (w) => {
        const { rng, windowStart } = this.rung(w)
        const step = Math.trunc((2 * w) / rungPoints)
        let got: Series[] = []
        if (
          await this.callFor(
            [signalErrors],
            "log rates over " + short(w),
            src.uid,
            async (abort) => {
              got = await this.r.range(
                src.uid,
                logLevelQuery(label, step),
                rng,
                rungPoints,
                abort
              )
            }
          )
        ) {
          this.addRows(
            logRows(
              {
                service: "",
                signal: "",
                rung: w,
                window: 0,
                baseline: 0,
                unit: "",
                source: src.uid,
                basis: basisLogLevels,
                moved: false,
              },
              windowStart,
              errorLines(got, label),
              step
            )
          )
        }
      })
    )
  }

  async context(): Promise<void> {
    const focus = this.focus()
    await Promise.all([
      this.origins(focus),
      this.annotations(),
      this.deployLines(),
      this.logFlags(),
      this.patterns(focus),
      this.traceFlags(),
    ])
  }

  async origins(focus: readonly Move[]): Promise<void> {
    const src = this.sources(signalTraces, "tempo", "clickhouse", "jaeger")[0]
    if (!src) {
      this.skip("error attribution", "", "no traces store lists services")
      return
    }
    const first = focus[0]
    const w = first ? finest(first) : rungs[0]
    let origins: Origin[] = []
    if (
      await this.call("error attribution", src.uid, async (abort) => {
        origins = await this.r.errorOrigins(
          src.uid,
          { from: this.input.end - w, to: this.input.end },
          abort
        )
      })
    ) {
      this.signals.push(
        `errors counted at the deepest erroring span on ${src.uid} over the last ${short(w)}`
      )
      this.findings.origins.push(...origins)
    }
  }

  async annotations(): Promise<void> {
    const rng = {
      from: this.input.end - 2 * (rungs[rungs.length - 1] as number),
      to: this.input.end,
    }
    let events: Event[] = []
    if (
      await this.call("annotations", "", async (abort) => {
        events = await this.r.annotations(rng, abort)
      })
    ) {
      this.findings.events.push(...events)
    }
  }

  eventRange(): { rng: Window; step: number } {
    const w = rungs[rungs.length - 1] as number
    return {
      rng: { from: this.input.end - w, to: this.input.end },
      step: Math.trunc(w / eventPoints),
    }
  }

  lokiSource(step: string): Source | undefined {
    const src = this.sources(signalLogs, "loki")[0]
    if (!src) {
      this.skip(
        step,
        "",
        "no Loki store lists services, and this step reads Loki only"
      )
    }
    return src
  }

  async deployLines(): Promise<void> {
    const name = "deploy, rollout, version and restart lines"
    const src = this.lokiSource(name)
    if (!src) {
      return
    }
    const label = this.field(src.uid, signalLogs)
    const { rng, step } = this.eventRange()
    let got: Series[] = []
    if (
      await this.call(name, src.uid, async (abort) => {
        got = await this.r.range(
          src.uid,
          deployLineQuery(label, step),
          rng,
          eventPoints,
          abort
        )
      })
    ) {
      this.signals.push(
        `deploy, rollout, version and restart log lines at every level on ${src.uid}`
      )
      this.findings.events.push(...burstStarts(got, label, src.uid))
    }
  }

  async logFlags(): Promise<void> {
    const name = "feature flag evaluations in logs"
    const src = this.lokiSource(name)
    if (!src) {
      return
    }
    const label = this.field(src.uid, signalLogs)
    const { rng, step } = this.eventRange()
    let got: Series[] = []
    if (
      await this.call(name, src.uid, async (abort) => {
        got = await this.r.range(
          src.uid,
          logFlagQuery(label, step),
          rng,
          eventPoints,
          abort
        )
      })
    ) {
      const events = flagFirstSeen(got, src.uid, rng.from)
      if (events.length > 0) {
        this.signals.push(
          `feature flag evaluations in log records on ${src.uid}`
        )
      }
      this.findings.events.push(...events)
    }
  }

  async traceFlags(): Promise<void> {
    const src = this.sources(signalTraces, "tempo")[0]
    if (!src) {
      this.skip(
        "feature flag evaluations in spans",
        "",
        "no Tempo store lists services, and span events are read on Tempo only"
      )
      return
    }
    const { rng } = this.eventRange()
    const step = "feature flag evaluations in spans"
    let keys: TraceStats = { groups: [], buckets: [] }
    if (
      !(await this.call(step, src.uid, async (abort) => {
        keys = await this.r.traceStats(
          src.uid,
          { query: flagQuery(), groupBy: flagKeyAttr, range: rng, buckets: 1 },
          abort
        )
      }))
    ) {
      return
    }
    const pairs: { key: string; attr: string; variant: string }[] = []
    for (const k of topGroups(keys.groups, maxFlagKeys)) {
      for (const attr of flagVariants) {
        let variants: TraceStats = { groups: [], buckets: [] }
        if (
          !(await this.call(step, src.uid, async (abort) => {
            variants = await this.r.traceStats(
              src.uid,
              {
                query: flagQuery(attrEquals(flagKeyAttr, k)),
                groupBy: attr,
                range: rng,
                buckets: 1,
              },
              abort
            )
          }))
        ) {
          return
        }
        const found = topGroups(variants.groups, maxFlagPairs)
        for (const v of found) {
          pairs.push({ key: k, attr, variant: v })
        }
        if (found.length > 0) {
          break
        }
      }
    }
    if (pairs.length > 0) {
      this.signals.push(`feature flag evaluations in span events on ${src.uid}`)
    }
    for (const p of pairs.slice(0, maxFlagPairs)) {
      let stats: TraceStats = { groups: [], buckets: [] }
      if (
        !(await this.call(step, src.uid, async (abort) => {
          stats = await this.r.traceStats(
            src.uid,
            {
              query: flagQuery(
                attrEquals(flagKeyAttr, p.key),
                attrEquals(p.attr, p.variant)
              ),
              groupBy: "",
              range: rng,
              buckets: traceBuckets,
            },
            abort
          )
        }))
      ) {
        return
      }
      const first = stats.buckets.find((b) => b.spans > 0)
      if (first) {
        this.findings.events.push(
          flagEvent(src.uid, p.key, p.variant, first.start, rng.from)
        )
      }
    }
  }

  async patterns(focus: readonly Move[]): Promise<void> {
    if (focus.length === 0) {
      return
    }
    const src = this.lokiSource("log patterns")
    if (!src) {
      return
    }
    const label = this.field(src.uid, signalLogs)
    if (!labelName.test(label)) {
      this.skip(
        "log patterns",
        src.uid,
        "the service field is not a stream label"
      )
      return
    }
    this.signals.push(
      `log patterns per candidate service at every level on ${src.uid}, the window's half against the half before it`
    )
    await Promise.all(
      focus.map(async (m) => {
        const { rng } = this.rung(finest(m))
        let got: { text: string; level: string; trend: number[] }[] = []
        if (
          await this.call("log patterns", src.uid, async (abort) => {
            got = await this.r.patterns(
              src.uid,
              lokiSelector(label, m.service),
              rng,
              abort
            )
          })
        ) {
          this.findings.patterns.push(
            ...changedPatterns(got, src.uid, m.service)
          )
        }
      })
    )
  }

  report(took: number): Report {
    const { candidates, background } = cluster(this.findings)
    this.services.sort(
      (a, b) =>
        compareBytes(a.source, b.source) || compareBytes(a.signal, b.signal)
    )
    this.signals.sort(compareBytes)
    this.skipped.sort(
      (a, b) =>
        compareBytes(a.step, b.step) ||
        compareBytes(a.source, b.source) ||
        compareBytes(a.reason, b.reason)
    )
    return {
      end: this.input.end,
      question: this.input.question,
      services: this.services,
      signals: this.signals,
      rates: topRates(this.rates),
      candidates,
      background,
      skipped: this.skipped,
      calls: this.calls,
      duration: took,
    }
  }
}

export async function sweep(
  reader: Reader,
  input: Input,
  limits: Limits = defaultLimits,
  now: () => number = () => performance.now()
): Promise<Outcome> {
  const start = now()
  const lim =
    limits.calls <= 0 || limits.time <= 0 || limits.concurrency <= 0
      ? defaultLimits
      : limits
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), lim.time)
  try {
    const sw = new Sweep(reader, lim, input, controller.signal)
    await sw.serviceMap()
    await sw.measure()
    await sw.context()
    const report = sw.report(now() - start)
    return {
      block: render(report, maxBlockBytes),
      leads: leads(report.candidates),
      calls: report.calls,
      duration: report.duration,
    }
  } finally {
    clearTimeout(timer)
  }
}

export function sweptStep(out: Pick<Outcome, "calls" | "duration">): string {
  return `Swept the datasources before the first model call: ${out.calls} reads in ${formatDuration(roundDuration(out.duration))}`
}
