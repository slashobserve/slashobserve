import { z } from "zod"
import { parseTime } from "../grounding/time"
import type { RunTools } from "../mcp"
import type { PatternTrend } from "./events"
import type { Series, TraceBucket, TraceGroup } from "./rates"
import { stamp } from "./render"
import { type Event, eventAnnotation, type Origin, type Window } from "./types"

export type TraceStatsInput = {
  query: string
  groupBy: string
  range: Window
  buckets: number
}

export type TraceStats = { groups: TraceGroup[]; buckets: TraceBucket[] }

export interface Reader {
  services(
    uid: string,
    signal: string,
    abort: AbortSignal
  ): Promise<{ field: string; names: string[] }>
  metricNames(
    uid: string,
    search: string,
    abort: AbortSignal
  ): Promise<string[]>
  range(
    uid: string,
    query: string,
    rng: Window,
    points: number,
    abort: AbortSignal
  ): Promise<Series[]>
  traceStats(
    uid: string,
    input: TraceStatsInput,
    abort: AbortSignal
  ): Promise<TraceStats>
  errorOrigins(uid: string, rng: Window, abort: AbortSignal): Promise<Origin[]>
  patterns(
    uid: string,
    query: string,
    rng: Window,
    abort: AbortSignal
  ): Promise<PatternTrend[]>
  annotations(rng: Window, abort: AbortSignal): Promise<Event[]>
}

export type CallTool = (
  name: string,
  args: Record<string, unknown>,
  abort: AbortSignal
) => Promise<{ content: string; isError: boolean }>

export function callToolOf(tools: RunTools, timeoutMs: number): CallTool {
  return async (name, args, abort) => {
    const reply = await tools.callTool(name, args, undefined, timeoutMs, abort)
    return { content: reply.content, isError: reply.isError }
  }
}

export class ReadError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "ReadError"
  }
}

const toolError = z.object({
  code: z.string().optional(),
  message: z.string().optional(),
})

const servicesOut = z.object({
  sources: z
    .array(
      z.object({
        field: z.string().optional(),
        services: z.array(z.string()).nullish(),
        unavailable: z.string().optional(),
      })
    )
    .nullish(),
})

const metricsOut = z.object({
  metrics: z.array(z.object({ name: z.string().optional() })).nullish(),
})

const seriesOut = z.object({
  series: z
    .array(
      z.object({
        labels: z.record(z.string(), z.string()).nullish(),
        points: z.array(z.tuple([z.number(), z.number()])).nullish(),
      })
    )
    .nullish(),
})

const nullableNumber = z.number().nullish()

const statsOut = z.object({
  buckets: z
    .array(
      z.object({
        start: z.string().optional(),
        spans: z.number().optional(),
        errors: z.number().optional(),
        p95_ms: nullableNumber,
      })
    )
    .nullish(),
  groups: z
    .array(
      z.object({
        value: z.string().optional(),
        spans: z.number().optional(),
        errors: z.number().optional(),
        p95_ms: nullableNumber,
      })
    )
    .nullish(),
})

const originsOut = z.object({
  groups: z
    .array(
      z.object({
        service: z.string().optional(),
        operation: z.string().optional(),
        status_message: z.string().optional(),
        traces: z.number().optional(),
        share: z.number().optional(),
        example_trace_id: z.string().optional(),
        relayed_by: z
          .array(z.object({ service: z.string().optional() }))
          .nullish(),
      })
    )
    .nullish(),
})

const patternsOut = z.object({
  patterns: z
    .array(
      z.object({
        pattern: z.string().optional(),
        level: z.string().optional(),
        trend: z.array(z.number()).nullish(),
      })
    )
    .nullish(),
})

const annotationsOut = z.object({
  annotations: z
    .array(
      z.object({
        time_from: z.string().optional(),
        text: z.string().optional(),
        tags: z.array(z.string()).nullish(),
      })
    )
    .nullish(),
})

function parsed<T>(schema: z.ZodType<T>, content: string): T {
  let raw: unknown
  try {
    raw = JSON.parse(content)
  } catch {
    throw new ReadError("the result is not JSON")
  }
  const out = schema.safeParse(raw)
  if (!out.success) {
    throw new ReadError("the result has an unexpected shape")
  }
  return out.data
}

export class ToolReader implements Reader {
  constructor(private readonly call: CallTool) {}

  private async read<T>(
    name: string,
    args: Record<string, unknown>,
    schema: z.ZodType<T>,
    abort: AbortSignal
  ): Promise<T> {
    const reply = await this.call(name, args, abort)
    if (reply.isError) {
      let code = ""
      let message = ""
      try {
        const body = toolError.safeParse(JSON.parse(reply.content))
        if (body.success) {
          code = body.data.code ?? ""
          message = body.data.message ?? ""
        }
      } catch {
        code = ""
      }
      code ||= "tool_error"
      throw new ReadError(message === "" ? code : code + ": " + message)
    }
    return parsed(schema, reply.content)
  }

  async services(uid: string, signal: string, abort: AbortSignal) {
    const out = await this.read(
      "list_services",
      { datasource: uid, signal, window: "6h" },
      servicesOut,
      abort
    )
    const first = out.sources?.[0]
    if (!first) {
      return { field: "", names: [] }
    }
    if (first.unavailable) {
      throw new ReadError("the store did not answer: " + first.unavailable)
    }
    return { field: first.field ?? "", names: first.services ?? [] }
  }

  async metricNames(uid: string, search: string, abort: AbortSignal) {
    const out = await this.read(
      "list_metrics",
      { datasource: uid, search, limit: 200 },
      metricsOut,
      abort
    )
    return (out.metrics ?? []).map((m) => m.name ?? "")
  }

  async range(
    uid: string,
    query: string,
    rng: Window,
    points: number,
    abort: AbortSignal
  ) {
    const out = await this.read(
      "read_series",
      {
        datasource: uid,
        query,
        from: stamp(rng.from),
        to: stamp(rng.to),
        max_data_points: points,
      },
      seriesOut,
      abort
    )
    return (out.series ?? []).map((s) => ({
      labels: s.labels ?? {},
      points: (s.points ?? []).map(([at, value]) => ({ at: at * 1000, value })),
    }))
  }

  async traceStats(uid: string, input: TraceStatsInput, abort: AbortSignal) {
    const args: Record<string, unknown> = {
      datasource: uid,
      query: input.query,
      from: stamp(input.range.from),
      to: stamp(input.range.to),
    }
    if (input.buckets > 0) {
      args["buckets"] = input.buckets
    }
    if (input.groupBy !== "") {
      args["group_by"] = input.groupBy
    }
    const out = await this.read("trace_stats", args, statsOut, abort)
    const stats: TraceStats = { groups: [], buckets: [] }
    for (const b of out.buckets ?? []) {
      const start = parseTime(b.start ?? "")
      if (start !== undefined) {
        stats.buckets.push({
          start,
          spans: b.spans ?? 0,
          errors: b.errors ?? 0,
          p95ms: b.p95_ms ?? undefined,
        })
      }
    }
    for (const g of out.groups ?? []) {
      stats.groups.push({
        value: g.value ?? "",
        spans: g.spans ?? 0,
        errors: g.errors ?? 0,
        p95ms: g.p95_ms ?? undefined,
      })
    }
    return stats
  }

  async errorOrigins(uid: string, rng: Window, abort: AbortSignal) {
    const out = await this.read(
      "trace_error_origins",
      { datasource: uid, from: stamp(rng.from), to: stamp(rng.to) },
      originsOut,
      abort
    )
    return (out.groups ?? []).map((g) => ({
      source: uid,
      service: g.service ?? "",
      operation: g.operation ?? "",
      message: g.status_message ?? "",
      traces: g.traces ?? 0,
      share: g.share ?? 0,
      example: g.example_trace_id ?? "",
      relayedBy: (g.relayed_by ?? []).map((r) => r.service ?? ""),
    }))
  }

  async patterns(uid: string, query: string, rng: Window, abort: AbortSignal) {
    const out = await this.read(
      "log_patterns",
      {
        datasource: uid,
        query,
        from: stamp(rng.from),
        to: stamp(rng.to),
        limit: 30,
      },
      patternsOut,
      abort
    )
    return (out.patterns ?? []).map((p) => ({
      text: p.pattern ?? "",
      level: p.level ?? "",
      trend: p.trend ?? [],
    }))
  }

  async annotations(rng: Window, abort: AbortSignal) {
    const out = await this.read(
      "list_annotations",
      { from: stamp(rng.from), to: stamp(rng.to), limit: 100 },
      annotationsOut,
      abort
    )
    const events: Event[] = []
    for (const a of out.annotations ?? []) {
      const at = parseTime(a.time_from ?? "")
      if (at === undefined) {
        continue
      }
      const tags = a.tags ?? []
      const text =
        tags.length > 0 ? `[${tags.join(" ")}] ${a.text ?? ""}` : (a.text ?? "")
      events.push({ kind: eventAnnotation, source: "", service: "", at, text })
    }
    return events
  }
}
