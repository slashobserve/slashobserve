import { join } from "node:path"
import type { ClaimApi, RunTools, ToolReply } from "../src/mcp"
import {
  type BriefAnswer,
  type BriefInput,
  type BuildProgress,
  type ClaimAnswer,
  type FinishAnswer,
  type FinishInput,
  type ReportAnswer,
  type ReportInput,
  type RunMeta,
  type TurnMeta,
  ToolError,
  noRunMeta,
} from "../src/protocol"
import type { ClaimedRun } from "../src/run"
import { createSession } from "../src/session/factory"
import type { ToolSpec } from "../src/session/types"

export const fixtureDir = join(import.meta.dir, "fixtures")

export const factory = createSession

export type ToolCallRecord = {
  name: string
  args: Record<string, unknown>
  turn: TurnMeta | undefined
  started: number
}

export type ToolHandler = (
  call: ToolCallRecord,
  nth: number
) => Promise<ToolReply>

export function reply(extra: Partial<ToolReply> = {}): ToolReply {
  return {
    content: "{}",
    isError: false,
    structured: undefined,
    meta: noRunMeta,
    ...extra,
  }
}

export function meta(
  settled: RunMeta["settled"],
  build: BuildProgress | null = null
): RunMeta {
  return { settled, build }
}

export function progress(
  saves: number,
  panels = saves,
  planned = panels
): BuildProgress {
  return { saves, panels, planned_panels: planned }
}

export class FakeTools implements RunTools {
  readonly calls: ToolCallRecord[] = []
  handler: ToolHandler = async () => reply()
  closed = false
  specs: ToolSpec[] = [
    { name: "create_dashboard", description: "", inputSchema: {} },
  ]

  async listTools(): Promise<ToolSpec[]> {
    return this.specs
  }

  async callTool(
    name: string,
    args: Record<string, unknown>,
    turn: TurnMeta | undefined
  ): Promise<ToolReply> {
    const record = { name, args, turn, started: performance.now() }
    this.calls.push(record)
    return this.handler(record, this.calls.length - 1)
  }

  async close(): Promise<void> {
    this.closed = true
  }
}

export class FakeClaim implements ClaimApi {
  readonly tools = new FakeTools()
  readonly briefs: BriefInput[] = []
  readonly reports: ReportInput[] = []
  readonly finishes: FinishInput[] = []
  brief: (input: BriefInput, nth: number) => Partial<BriefAnswer> = () => ({})
  report: (input: ReportInput, nth: number) => Partial<ReportAnswer> =
    () => ({})
  finish: (input: FinishInput, nth: number) => Partial<FinishAnswer> =
    () => ({})
  reportError:
    ((input: ReportInput, nth: number) => Error | undefined) | undefined

  async claimRun(_waitSeconds?: number): Promise<ClaimAnswer> {
    throw new Error("claimRun is not used by the run loop")
  }

  async getRunBrief(input: BriefInput): Promise<BriefAnswer> {
    this.briefs.push(input)
    return {
      system: "system text",
      message: "user message",
      reason: input.reason ?? "",
      build: null,
      ...this.brief(input, this.briefs.length - 1),
    }
  }

  async reportRun(input: ReportInput): Promise<ReportAnswer> {
    this.reports.push(input)
    const failure = this.reportError?.(input, this.reports.length - 1)
    if (failure !== undefined) {
      throw failure
    }
    return {
      state: "running",
      stop: false,
      ...this.report(input, this.reports.length - 1),
    }
  }

  async finishRun(input: FinishInput): Promise<FinishAnswer> {
    this.finishes.push(input)
    return {
      state: "succeeded",
      restarted: false,
      ...this.finish(input, this.finishes.length - 1),
    }
  }

  openRun(): RunTools {
    return this.tools
  }
}

export function claimed(
  prompt: string,
  extra: {
    kind?: string
    source?: string
    dashboard_id?: string | null
    renew?: number
    maxTokens?: number
  } = {}
): ClaimedRun {
  return {
    run: {
      id: "11111111-1111-4111-8111-111111111111",
      attempt: 1,
      kind: extra.kind ?? "build",
      source: extra.source ?? "observe",
      prompt,
      dashboard_id: extra.dashboard_id ?? null,
    },
    session: { url: "http://127.0.0.1:1/mcp", token: "sot_test" },
    model: {
      provider: "replay",
      model: "",
      api_key: "",
      base_url: "",
      effort: "",
      max_tokens: extra.maxTokens ?? 0,
      request_timeout_seconds: 0,
      idle_timeout_seconds: 0,
      replay_dir: fixtureDir,
      log_provider_errors: false,
      session_renew_bytes: extra.renew ?? 0,
    },
  }
}

export function notYourRun(): ToolError {
  return new ToolError("not_your_run", "that run is not yours")
}
