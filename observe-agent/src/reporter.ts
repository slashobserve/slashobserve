import { log } from "./log"
import type { ClaimApi } from "./mcp"
import {
  type BuildProgress,
  type ReportAnswer,
  type Usage,
  ToolError,
} from "./protocol"

export const heartbeatMs = 5_000
export const textFlushMs = 1_000
export const maxTextCodePoints = 4_000

export class RunEnded extends Error {
  readonly state: string

  constructor(state: string) {
    super(`the run ended: ${state}`)
    this.name = "RunEnded"
    this.state = state
  }
}

export type ReporterOptions = { heartbeatMs: number; textFlushMs: number }

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
  private buffered = ""
  private flushTimer: ReturnType<typeof setTimeout> | undefined
  private beatTimer: ReturnType<typeof setInterval> | undefined
  private beating = false
  private ended: RunEnded | undefined

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

  push(text: string): void {
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
    if (this.ended !== undefined) {
      throw this.ended
    }
  }

  async flush(): Promise<void> {
    this.flushTimer = undefined
    try {
      for (const chunk of this.takeText()) {
        await this.send({ text: chunk })
      }
    } catch (error) {
      this.noteFailure("live text report failed", error)
    }
  }

  private async beat(): Promise<void> {
    if (this.beating || this.ended !== undefined) {
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
    let answer: ReportAnswer
    try {
      answer = await this.attempt(body)
    } catch (error) {
      if (error instanceof ToolError && error.code === "not_your_run") {
        this.end(new RunEnded("lost"))
        return
      }
      throw error
    }
    if (answer.build != null) {
      this.hooks.onBuild(answer.build)
    }
    if (answer.stop) {
      this.end(new RunEnded(answer.state))
    }
  }

  private async attempt(body: {
    text?: string
    usage?: Usage
  }): Promise<ReportAnswer> {
    const input = { ...this.run, ...body }
    try {
      return await this.api.reportRun(input, this.signal)
    } catch (error) {
      if (error instanceof ToolError || this.signal.aborted) {
        throw error
      }
      return this.api.reportRun(input, this.signal)
    }
  }

  private end(ended: RunEnded): void {
    if (this.ended === undefined) {
      this.ended = ended
      this.hooks.onEnd(ended)
    }
  }
}
