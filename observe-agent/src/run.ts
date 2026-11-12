import { randomUUID } from "node:crypto"
import { log } from "./log"
import type { ClaimApi, RunTools } from "./mcp"
import { isSave } from "./phase"
import {
  type BriefReason,
  type BuildProgress,
  type FailureCode,
  type ModelSettings,
  type OfferedRun,
  type RunSessionOffer,
  type RunMeta,
  type Usage,
  ToolError,
  defaultSessionRenewBytes,
} from "./protocol"
import {
  type ReporterOptions,
  Reporter,
  RunEnded,
  heartbeatMs,
  textFlushMs,
} from "./reporter"
import {
  CallError,
  NoFixtureError,
  SessionError,
  type Session,
  type SessionFactory,
  type ToolResult,
  type ToolSpec,
} from "./session/types"
import { executeTurn } from "./turn"

export const compactTailTurns = 4
export const compactTailBytes = 32 << 10
export const compactResultBytes = 2 << 10
export const pruneKeepTurns = 4
export const pruneStepBytes = 64 << 10
export const maxCutOffsInRow = 2
export const listToolsTimeoutMs = 10_000

export type ClaimedRun = {
  run: OfferedRun
  session: RunSessionOffer
  model: ModelSettings
}

export type RunOptions = Partial<ReporterOptions>

type FinishBody =
  | { outcome: "succeeded"; commit?: boolean }
  | { outcome: "failed"; failure_code?: FailureCode }

type Mode = "create" | "revise" | "investigate" | "suggest"

class Failure extends Error {
  readonly code: FailureCode

  constructor(code: FailureCode) {
    super(`the run failed: ${code}`)
    this.name = "Failure"
    this.code = code
  }
}

type SessionEnd =
  | { kind: "finished" }
  | { kind: "restart" }
  | {
      kind: "compact"
      reason: BriefReason
      results: ToolResult[] | undefined
    }

type TurnStep =
  { kind: "continue"; results: ToolResult[] } | { kind: "end"; end: SessionEnd }

const settledPriority = [
  "finding",
  "suggested",
  "saved",
  "restarted",
  "ended",
] as const

function modeOf(run: OfferedRun): Mode {
  if (run.kind === "investigate" || run.source === "investigate") {
    return "investigate"
  }
  if (run.source === "suggest") {
    return "suggest"
  }
  return run.dashboard_id == null ? "create" : "revise"
}

function sessionFailure(error: unknown): FailureCode {
  if (error instanceof CallError) {
    return error.failure
  }
  if (error instanceof NoFixtureError) {
    return "no_fixture"
  }
  if (error instanceof SessionError) {
    return "internal"
  }
  return "provider_error"
}

function failureOf(error: unknown): FailureCode {
  return error instanceof Failure ? error.code : "internal"
}

function settledOf(metas: RunMeta[]): RunMeta["settled"] {
  for (const kind of settledPriority) {
    if (metas.some((meta) => meta.settled === kind)) {
      return kind
    }
  }
  return null
}

function sizeOfTurn(
  text: string,
  inputs: string[],
  results: ToolResult[]
): number {
  let bytes = Buffer.byteLength(text)
  for (const input of inputs) {
    bytes += Buffer.byteLength(input)
  }
  for (const result of results) {
    bytes += Buffer.byteLength(result.content)
  }
  return bytes
}

class RunLoop {
  private readonly claim: ClaimApi
  private readonly claimed: ClaimedRun
  private readonly factory: SessionFactory
  private readonly outer: AbortSignal
  private readonly options: ReporterOptions
  private readonly tools: RunTools
  private readonly cancel = new AbortController()
  private readonly reporter: Reporter
  private readonly mode: Mode
  private readonly runRef: { run_id: string; attempt: number }
  private readonly renewBytes: number
  private specs: ToolSpec[] = []
  private session: Session | undefined
  private build: BuildProgress | null = null
  private buildSaves = 0
  private sessionBytes = 0
  private cutOffs = 0
  private findingNudged = false
  private answerNudged = false
  private pendingNudged = false
  private modelCalls = 0
  private retries = 0
  private compactions = 0
  private renewals = 0
  private modelMs = 0
  private maxSaveOutputTokens = 0
  private inputTokens = 0
  private outputTokens = 0
  private cacheReadTokens = 0
  private cacheWriteTokens = 0

  constructor(
    claim: ClaimApi,
    claimed: ClaimedRun,
    factory: SessionFactory,
    outer: AbortSignal,
    options: ReporterOptions
  ) {
    this.claim = claim
    this.claimed = claimed
    this.factory = factory
    this.outer = outer
    this.options = options
    this.mode = modeOf(claimed.run)
    this.runRef = { run_id: claimed.run.id, attempt: claimed.run.attempt }
    this.renewBytes =
      claimed.model.session_renew_bytes > 0
        ? claimed.model.session_renew_bytes
        : defaultSessionRenewBytes
    this.tools = claim.openRun(claimed.session)
    this.reporter = new Reporter(
      claim,
      this.runRef,
      options,
      {
        onBuild: (build) => {
          this.build = build
        },
        onEnd: (ended) => this.cancel.abort(ended),
      },
      outer
    )
    outer.addEventListener("abort", () => this.cancel.abort(outer.reason), {
      once: true,
    })
  }

  async run(): Promise<void> {
    log("info", "run started", {
      run_id: this.runRef.run_id,
      attempt: this.runRef.attempt,
      kind: this.claimed.run.kind,
      mode: this.mode,
    })
    this.reporter.startHeartbeat()
    try {
      await this.main()
      log("info", "run finished", this.counters())
    } catch (error) {
      await this.conclude(error)
    } finally {
      this.reporter.stop()
      await this.tools.close()
    }
  }

  private counters() {
    return {
      run_id: this.runRef.run_id,
      model_calls: this.modelCalls,
      retries: this.retries,
      compactions: this.compactions,
      renewals: this.renewals,
      model_ms: this.modelMs,
    }
  }

  private async conclude(thrown: unknown): Promise<void> {
    if (this.outer.aborted) {
      return
    }
    const reason: unknown = this.cancel.signal.reason
    const error =
      this.cancel.signal.aborted && reason instanceof RunEnded ? reason : thrown
    if (error instanceof RunEnded) {
      log("info", "run ended by the server", {
        run_id: this.runRef.run_id,
        state: error.state,
      })
      if (error.state === "canceling") {
        await this.finishQuietly({ outcome: "failed" })
      }
      return
    }
    if (error instanceof ToolError && error.code === "not_your_run") {
      log("info", "run is no longer ours", { run_id: this.runRef.run_id })
      return
    }
    const code = failureOf(error)
    log("warn", "run failed", {
      ...this.counters(),
      failure_code: code,
      error: error instanceof Error ? error.name : "unknown",
    })
    await this.finishQuietly({ outcome: "failed", failure_code: code })
  }

  private async finishQuietly(body: FinishBody): Promise<void> {
    try {
      await this.finish(body)
    } catch (finishError) {
      if (!this.outer.aborted) {
        log("warn", "finish_run failed", {
          run_id: this.runRef.run_id,
          error: finishError instanceof Error ? finishError.name : "unknown",
        })
      }
    }
  }

  private async main(): Promise<void> {
    this.specs = await this.tools.listTools(listToolsTimeoutMs, this.outer)
    let reason: BriefReason | undefined
    let carry: { results: ToolResult[] | undefined } | undefined
    for (;;) {
      const end = await this.pass(reason, carry)
      if (end.kind === "finished") {
        return
      }
      if (end.kind === "restart") {
        this.cutOffs = 0
        this.findingNudged = false
        this.answerNudged = false
        this.pendingNudged = false
        this.session = undefined
        reason = "conflict"
        carry = undefined
        continue
      }
      reason = end.reason
      carry = { results: end.results }
    }
  }

  private async pass(
    reason: BriefReason | undefined,
    carry: { results: ToolResult[] | undefined } | undefined
  ): Promise<SessionEnd> {
    const brief = await this.claim.getRunBrief(
      { ...this.runRef, ...(reason === undefined ? {} : { reason }) },
      this.cancel.signal
    )
    this.build = brief.build ?? null
    this.buildSaves = brief.build?.saves ?? 0
    this.sessionBytes = 0
    log("debug", "run brief loaded", {
      run_id: this.runRef.run_id,
      reason: brief.reason,
    })
    let results: ToolResult[] | undefined
    if (carry === undefined || this.session === undefined) {
      this.session = await this.open(brief.system, brief.message)
    } else {
      await this.compact(this.session, brief.message, reason)
      results = carry.results
    }
    for (;;) {
      const step = await this.turn(this.session, results)
      if (step.kind === "end") {
        return step.end
      }
      results = step.results
    }
  }

  private async open(system: string, message: string): Promise<Session> {
    try {
      return await this.factory(
        this.claimed.model,
        {
          system,
          tools: this.specs,
          prompt: this.claimed.run.prompt,
          userMessage: message,
          buildSaves: this.buildSaves,
          pruning: { keepTurns: pruneKeepTurns, stepBytes: pruneStepBytes },
        },
        this.cancel.signal
      )
    } catch (error) {
      this.throwIfQuiet(error)
      throw new Failure(sessionFailure(error))
    }
  }

  private async compact(
    session: Session,
    message: string,
    reason: BriefReason | undefined
  ): Promise<void> {
    try {
      await session.compact({
        message,
        tailTurns: compactTailTurns,
        tailBytes: compactTailBytes,
        resultBytes: compactResultBytes,
      })
    } catch (error) {
      this.throwIfQuiet(error)
      throw new Failure(sessionFailure(error))
    }
    this.compactions++
    if (reason === "renewed") {
      this.renewals++
    }
  }

  private throwIfQuiet(error: unknown): void {
    const reason: unknown = this.cancel.signal.reason
    if (this.cancel.signal.aborted && reason instanceof RunEnded) {
      throw reason
    }
    if (this.outer.aborted) {
      throw error
    }
  }

  private async turn(
    session: Session,
    results: ToolResult[] | undefined
  ): Promise<TurnStep> {
    await this.reporter.report(this.usageBody())
    this.modelCalls++
    const started = Date.now()
    let answer
    try {
      answer = await session.next(results, {
        signal: this.cancel.signal,
        onText: (text) => this.reporter.push(text),
      })
    } catch (error) {
      this.modelMs += Date.now() - started
      this.throwIfQuiet(error)
      if (
        error instanceof CallError &&
        error.failure === "call_timeout" &&
        !this.cutOff()
      ) {
        return this.compaction("call_timeout", results)
      }
      throw new Failure(sessionFailure(error))
    }
    this.modelMs += Date.now() - started
    this.retries += Math.max(0, answer.attempts - 1)
    const { turn } = answer
    this.inputTokens += turn.usage.inputTokens
    this.outputTokens += turn.usage.outputTokens
    this.cacheReadTokens += turn.usage.cacheReadTokens
    this.cacheWriteTokens += turn.usage.cacheWriteTokens
    if (turn.toolCalls.some((call) => isSave(call.name))) {
      this.maxSaveOutputTokens = Math.max(
        this.maxSaveOutputTokens,
        turn.usage.outputTokens
      )
    }
    if (turn.stop === "refusal") {
      throw new Failure("refused")
    }
    if (turn.stop === "max_tokens") {
      if (this.cutOff()) {
        throw new Failure("output_limit")
      }
      return this.compaction("output_limit", results)
    }
    if (turn.toolCalls.length === 0) {
      return this.finishWithoutCall()
    }
    const executed = await executeTurn(
      this.tools,
      turn.toolCalls,
      randomUUID(),
      this.cancel.signal
    ).catch((error: unknown) => {
      this.throwIfQuiet(error)
      throw error
    })
    this.sessionBytes += sizeOfTurn(
      turn.text,
      turn.toolCalls.map((call) => call.input),
      executed.results
    )
    return this.settle(executed.metas, executed.results)
  }

  private async settle(
    metas: RunMeta[],
    results: ToolResult[]
  ): Promise<TurnStep> {
    let built = false
    for (const meta of metas) {
      if (meta.build === null) {
        continue
      }
      this.build = meta.build
      if (meta.build.saves > this.buildSaves) {
        built = true
        this.cutOffs = 0
        this.buildSaves = meta.build.saves
      }
    }
    switch (settledOf(metas)) {
      case "saved":
      case "finding":
      case "suggested":
        await this.finish({ outcome: "succeeded" })
        return { kind: "end", end: { kind: "finished" } }
      case "restarted":
        return { kind: "end", end: { kind: "restart" } }
      case "ended":
        throw new RunEnded("ended")
    }
    if (built && this.sessionBytes > this.renewBytes) {
      return this.compaction("renewed", results)
    }
    return { kind: "continue", results }
  }

  private compaction(
    reason: BriefReason,
    results: ToolResult[] | undefined
  ): TurnStep {
    return { kind: "end", end: { kind: "compact", reason, results } }
  }

  private cutOff(): boolean {
    this.cutOffs++
    return this.mode === "suggest" || this.cutOffs >= maxCutOffsInRow
  }

  private async finishWithoutCall(): Promise<TurnStep> {
    switch (this.mode) {
      case "suggest":
        throw new Failure("no_tool_call")
      case "investigate":
        if (this.findingNudged) {
          throw new Failure("no_finding")
        }
        this.findingNudged = true
        return this.compaction("no_finding", undefined)
    }
    const build = this.build
    if (build === null) {
      if (this.answerNudged) {
        throw new Failure("no_tool_call")
      }
      this.answerNudged = true
      return this.compaction("no_answer", undefined)
    }
    if (build.planned_panels - build.panels > 0 && !this.pendingNudged) {
      this.pendingNudged = true
      return this.compaction("pending", undefined)
    }
    const finished = await this.finish({ outcome: "succeeded", commit: true })
    return {
      kind: "end",
      end: { kind: finished.restarted ? "restart" : "finished" },
    }
  }

  private async finish(body: FinishBody) {
    await this.reporter.flush()
    return this.claim.finishRun(
      { ...this.runRef, ...body, usage: this.usageBody() },
      this.outer
    )
  }

  private usageBody(): Usage {
    const { provider, model } = this.claimed.model
    return {
      provider,
      model: model !== "" ? model : provider,
      input_tokens: this.inputTokens,
      output_tokens: this.outputTokens,
      cache_read_tokens: this.cacheReadTokens,
      cache_write_tokens: this.cacheWriteTokens,
      model_calls: this.modelCalls,
      retries: this.retries,
      compactions: this.compactions,
      renewals: this.renewals,
      model_ms: this.modelMs,
      max_save_output_tokens: this.maxSaveOutputTokens,
    }
  }
}

export async function runLoop(
  claim: ClaimApi,
  claimed: ClaimedRun,
  factory: SessionFactory,
  signal: AbortSignal,
  options: RunOptions = {}
): Promise<void> {
  await new RunLoop(claim, claimed, factory, signal, {
    heartbeatMs: options.heartbeatMs ?? heartbeatMs,
    textFlushMs: options.textFlushMs ?? textFlushMs,
  }).run()
}
