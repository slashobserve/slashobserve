import { log } from "./log"
import type { ClaimApi } from "./mcp"
import {
  type BuildProgress,
  type ReportAnswer,
  type Usage,
  ToolError,
} from "./protocol"
import { type RetryPolicy, retryWithin } from "./retry"

export const heartbeatMs = 5_000
export const textFlushMs = 1_000
export const maxTextCodePoints = 4_000
export const usageRetryMs: readonly number[] = [250, 1_000]
export const usageReportBudgetMs = 20_000

export class RunEnded extends Error {
  readonly state: string

  constructor(state: string) {
    super(`the run ended: ${state}`)
    this.name = "RunEnded"
    this.state = state
  }
}

export type ReporterOptions = {
  heartbeatMs: number
  textFlushMs: number
  usageRetryMs?: readonly number[]
  usageBudgetMs?: number
  clock?: Clock
}

export type Clock = { wall: () => number; monotonic: () => number }

const systemClock: Clock = {
  wall: Date.now,
  monotonic: () => performance.now(),
}

type Instant = { wall: number; monotonic: number }

type Reported = { answer: ReportAnswer; sent: Instant }

type Lease =
  | { state: "renewed"; monotonicEnd: number | undefined }
  | { state: "stop_told"; ended: RunEnded }
  | { state: "lost"; ended: RunEnded }

function timeOf(text: string | null | undefined): number | undefined {
  const time = Date.parse(text ?? "")
  return Number.isNaN(time) ? undefined : time
}

function leaseEndFrom(sent: Instant, answer: ReportAnswer): number | undefined {
  const until = timeOf(answer.lease_until)
  return until === undefined
    ? undefined
    : sent.monotonic + until - (timeOf(answer.server_time) ?? sent.wall)
}

export type ReporterHooks = {
  onBuild: (build: BuildProgress) => void
  onEnd: (ended: RunEnded) => void
}

export function chunkText(text: string, size: number): string[] {
  const points = Array.from(text)
  const chunks: string[] = []
  for (let at = 0; at < points.length; at += size) {
    chunks.push(points.slice(at, at + size).join(""))
  }
  return chunks
}

export class Reporter {
  private readonly api: ClaimApi
  private readonly run: { run_id: string; attempt: number }
  private readonly options: ReporterOptions
  private readonly hooks: ReporterHooks
  private readonly signal: AbortSignal
  private readonly clock: Clock
  private buffered = ""
  private flushTimer: ReturnType<typeof setTimeout> | undefined
  private beatTimer: ReturnType<typeof setInterval> | undefined
  private beating = false
  private wroteText = false
  private messageBreak = false
  private lease: Lease = { state: "renewed", monotonicEnd: undefined }

  constructor(
    api: ClaimApi,
    run: { run_id: string; attempt: number },
    options: ReporterOptions,
    hooks: ReporterHooks,
    signal: AbortSignal
  ) {
    this.api = api
    this.run = run
    this.options = options
    this.hooks = hooks
    this.signal = signal
    this.clock = options.clock ?? systemClock
  }

  startHeartbeat(): void {
    this.beatTimer = setInterval(
      () => void this.beat(),
      this.options.heartbeatMs
    )
  }

  stop(): void {
    clearInterval(this.beatTimer)
    clearTimeout(this.flushTimer)
    this.beatTimer = undefined
    this.flushTimer = undefined
  }

  startMessage(): void {
    this.messageBreak = this.wroteText
  }

  push(text: string): void {
    if (text !== "") {
      if (this.messageBreak) {
        this.buffered += "\n\n"
        this.messageBreak = false
      }
      this.wroteText = true
    }
    this.buffered += text
    if (this.flushTimer === undefined && this.buffered !== "") {
      this.flushTimer = setTimeout(
        () => void this.flush(),
        this.options.textFlushMs
      )
    }
  }

  async report(usage: Usage): Promise<void> {
    this.throwIfEnded()
    const chunks = this.takeText()
    const [first, ...rest] = chunks
    await this.send({ usage, ...(first === undefined ? {} : { text: first }) })
    for (const chunk of rest) {
      await this.send({ text: chunk })
    }
    this.throwIfEnded()
  }

  private takeText(): string[] {
    clearTimeout(this.flushTimer)
    this.flushTimer = undefined
    const chunks = chunkText(this.buffered, maxTextCodePoints)
    this.buffered = ""
    return chunks
  }

  private throwIfEnded(): void {
    const lease = this.leaseNow()
    if (lease.state !== "renewed") {
      throw lease.ended
    }
  }

  private leaseNow(): Lease {
    const { lease } = this
    if (
      lease.state === "renewed" &&
      lease.monotonicEnd !== undefined &&
      this.clock.monotonic() >= lease.monotonicEnd
    ) {
      log("warn", "lease lapsed without a successful report", {
        run_id: this.run.run_id,
      })
      this.end({ state: "lost", ended: new RunEnded("lost") })
    }
    return this.lease
  }

  async flush(): Promise<void> {
    this.flushTimer = undefined
    if (this.leaseNow().state === "lost") {
      return
    }
    try {
      for (const chunk of this.takeText()) {
        await this.send({ text: chunk })
      }
    } catch (error) {
      this.noteFailure("live text report failed", error)
    }
  }

  async beat(): Promise<void> {
    if (this.leaseNow().state !== "renewed" || this.beating) {
      return
    }
    this.beating = true
    try {
      await this.send({})
    } catch (error) {
      this.noteFailure("lease report failed", error)
    } finally {
      this.beating = false
    }
  }

  private noteFailure(msg: string, error: unknown): void {
    if (!this.signal.aborted) {
      log("warn", msg, {
        run_id: this.run.run_id,
        error: error instanceof Error ? error.message : "unknown",
      })
    }
  }

  private async send(body: { text?: string; usage?: Usage }): Promise<void> {
    let reported: Reported
    try {
      reported = await this.attempt(body)
    } catch (error) {
      if (error instanceof ToolError && error.code === "not_your_run") {
        this.end({ state: "lost", ended: new RunEnded("lost") })
        return
      }
      throw error
    }
    const { answer, sent } = reported
    if (answer.build != null) {
      this.hooks.onBuild(answer.build)
    }
    if (answer.stop) {
      this.end({ state: "stop_told", ended: new RunEnded(answer.state) })
      return
    }
    const monotonicEnd = leaseEndFrom(sent, answer)
    if (this.lease.state === "renewed" && monotonicEnd !== undefined) {
      this.lease = { state: "renewed", monotonicEnd }
    }
  }

  private attempt(body: { text?: string; usage?: Usage }): Promise<Reported> {
    const input = { ...this.run, ...body }
    const policy: RetryPolicy =
      body.usage === undefined
        ? { waitsMs: [0], budgetMs: Infinity }
        : {
            waitsMs: this.options.usageRetryMs ?? usageRetryMs,
            budgetMs: this.options.usageBudgetMs ?? usageReportBudgetMs,
          }
    return retryWithin(
      async (signal) => {
        const sent = {
          wall: this.clock.wall(),
          monotonic: this.clock.monotonic(),
        }
        return { answer: await this.api.reportRun(input, signal), sent }
      },
      this.signal,
      policy
    )
  }

  private end(lease: Exclude<Lease, { state: "renewed" }>): void {
    if (this.lease.state === "renewed") {
      this.lease = lease
      this.hooks.onEnd(lease.ended)
    }
  }
}
