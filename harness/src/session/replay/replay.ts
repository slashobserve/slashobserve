import { createHash } from "node:crypto"
import { readFile, readdir } from "node:fs/promises"
import { join } from "node:path"
import { type Logger, log } from "../../log"
import {
  CallError,
  type Compaction,
  NoFixtureError,
  ReplayError,
  type Session,
  SessionError,
  type Start,
  type ToolResult,
  type Turn,
  type TurnAnswer,
  type TurnIO,
} from "../types"
import {
  type Expectation,
  type Fault,
  type Fixture,
  type FixtureTurn,
  parseFixture,
  promptKey,
  turnsFor,
} from "./fixture"
import { goQuote } from "./goquote"

const name = "replay"
const promptHashLength = 12
const fixtureFileSuffix = ".json"
const bytesPerToken = 3.2
const maxTimerMs = 2 ** 31 - 1
const plainToken = /^[A-Za-z0-9_.:-]{1,128}$/

export function promptKeyHash(prompt: string): string {
  return createHash("sha256")
    .update(promptKey(prompt))
    .digest("hex")
    .slice(0, promptHashLength)
}

function osReason(error: unknown): string {
  const code = (error as { code?: string }).code
  switch (code) {
    case "ENOENT":
      return "no such file or directory"
    case "EACCES":
      return "permission denied"
    case "ENOTDIR":
      return "not a directory"
    default:
      return error instanceof Error ? error.message : String(error)
  }
}

function byteOrder(a: string, b: string): number {
  return Buffer.compare(Buffer.from(a), Buffer.from(b))
}

function utf8(bytes: Uint8Array): string {
  return new TextDecoder("utf-8", { ignoreBOM: true }).decode(bytes)
}

export class ReplayProvider {
  private readonly fixtures: Map<string, Fixture>
  private readonly maxTokens: number
  private readonly logger: Logger

  private constructor(
    fixtures: Map<string, Fixture>,
    maxTokens: number,
    logger: Logger
  ) {
    this.fixtures = fixtures
    this.maxTokens = maxTokens
    this.logger = logger
  }

  static async load(
    dir: string,
    maxTokens: number,
    logger: Logger = log
  ): Promise<ReplayProvider> {
    if (dir === "") {
      throw new SessionError("replay fixture directory is not set")
    }
    let entries
    try {
      entries = await readdir(dir, { withFileTypes: true })
    } catch (error) {
      throw new SessionError(
        `read replay fixture directory: open ${dir}: ${osReason(error)}`
      )
    }
    const files = entries
      .filter(
        (entry) =>
          !entry.isDirectory() && entry.name.endsWith(fixtureFileSuffix)
      )
      .map((entry) => entry.name)
      .sort(byteOrder)
    const fixtures = new Map<string, Fixture>()
    const problems: string[] = []
    for (const file of files) {
      let fixture: Fixture
      try {
        const bytes = await readFile(join(dir, file))
        fixture = parseFixture(file, utf8(bytes))
      } catch (error) {
        problems.push(
          error instanceof Error && error.name === "FixtureError"
            ? error.message
            : `replay fixture ${file}: open ${join(dir, file)}: ${osReason(error)}`
        )
        continue
      }
      const key = promptKey(fixture.prompt)
      const other = fixtures.get(key)
      if (other !== undefined) {
        problems.push(
          `replay fixtures ${other.file} and ${fixture.file} have the same prompt`
        )
        continue
      }
      fixtures.set(key, fixture)
      logger("debug", "replay fixture loaded", {
        fixture: fixture.file,
        prompt_key_sha256: promptKeyHash(fixture.prompt),
      })
    }
    if (problems.length > 0) {
      throw new SessionError(problems.join("\n"))
    }
    logger("info", "replay fixtures loaded", { count: fixtures.size })
    logger(
      "warn",
      "replay LLM provider answers only from recorded fixtures; use it for tests only"
    )
    return new ReplayProvider(fixtures, maxTokens, logger)
  }

  start(start: Start, signal: AbortSignal): Session {
    signal.throwIfAborted()
    const fixture = this.fixtures.get(promptKey(start.prompt))
    if (fixture === undefined) {
      this.logger("debug", "replay found no fixture for the prompt", {
        prompt_length: Buffer.byteLength(start.prompt),
        prompt_key_sha256: promptKeyHash(start.prompt),
      })
      throw new NoFixtureError()
    }
    const turns = turnsFor(fixture, start.buildSaves)
    if (turns === undefined) {
      this.logger("debug", "replay found no resume segment for the build", {
        fixture: fixture.file,
        build_saves: start.buildSaves,
      })
      throw new NoFixtureError()
    }
    return new ReplaySession(fixture, turns, this.maxTokens, this.logger)
  }
}

function waitOrAbort(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason)
      return
    }
    const onAbort = (): void => {
      clearTimeout(timer)
      reject(signal.reason)
    }
    const timer = setTimeout(
      () => {
        signal.removeEventListener("abort", onAbort)
        resolve()
      },
      Math.min(ms, maxTimerMs)
    )
    signal.addEventListener("abort", onAbort, { once: true })
  })
}

export function estimateOutputTokens(inputs: string[]): number {
  let bytes = 0
  for (const input of inputs) {
    bytes += Buffer.byteLength(input)
  }
  return Math.ceil(bytes / bytesPerToken)
}

function toolErrorCode(result: ToolResult): string {
  if (!result.isError) {
    return ""
  }
  try {
    const body: unknown = JSON.parse(result.content)
    const code = (body as { code?: unknown } | null)?.code
    return typeof code === "string" && plainToken.test(code) ? code : ""
  } catch {
    return ""
  }
}

class ReplaySession implements Session {
  private readonly fixture: Fixture
  private readonly turns: FixtureTurn[]
  private readonly maxTokens: number
  private readonly logger: Logger
  private index = 0
  private previous: Turn["toolCalls"] = []
  private faultSpent = false
  private ended = false

  constructor(
    fixture: Fixture,
    turns: FixtureTurn[],
    maxTokens: number,
    logger: Logger
  ) {
    this.fixture = fixture
    this.turns = turns
    this.maxTokens = maxTokens
    this.logger = logger
  }

  async compact(_c: Compaction): Promise<void> {
    this.ended = false
  }

  async next(
    results: ToolResult[] | undefined,
    io: TurnIO
  ): Promise<TurnAnswer> {
    io.signal.throwIfAborted()
    if (this.ended) {
      throw new ReplayError(
        `replay fixture ${this.fixture.file}: the previous turn made no tool calls, so the session has ended`
      )
    }
    const index = this.index
    this.checkResults(index, results ?? [])
    const turn = this.turns[index]
    if (turn === undefined) {
      this.ended = true
      return answer({
        text: "",
        toolCalls: [],
        stop: "end_turn",
        usage: usage(0),
      })
    }
    if (turn.fault !== undefined) {
      const injected = await this.inject(turn.fault, io.signal)
      if (injected !== undefined) {
        return answer(injected)
      }
    }
    const toolCalls = turn.toolCalls.map((call, i) => ({
      id: `replay_${index}_${i}`,
      name: call.name,
      input: call.input,
    }))
    const outputTokens = estimateOutputTokens(toolCalls.map((c) => c.input))
    if (this.maxTokens > 0 && outputTokens > this.maxTokens) {
      return answer({
        text: "",
        toolCalls: [],
        stop: "max_tokens",
        usage: usage(this.maxTokens),
      })
    }
    this.index = index + 1
    this.faultSpent = false
    this.previous = toolCalls
    this.ended = toolCalls.length === 0
    if (turn.text !== "") {
      io.onText(turn.text)
    }
    return answer({
      text: turn.text,
      toolCalls,
      stop: toolCalls.length > 0 ? "tool_use" : "end_turn",
      usage: usage(outputTokens),
    })
  }

  private async inject(
    fault: Fault,
    signal: AbortSignal
  ): Promise<Turn | undefined> {
    switch (fault.kind) {
      case "delay":
        await waitOrAbort(fault.delayMs, signal)
        return undefined
      case "max_tokens":
        return { text: "", toolCalls: [], stop: "max_tokens", usage: usage(0) }
      case "refusal":
        return { text: "", toolCalls: [], stop: "refusal", usage: usage(0) }
    }
    if (this.faultSpent) {
      return undefined
    }
    this.faultSpent = true
    switch (fault.kind) {
      case "rate_limited":
        throw new CallError(name, "rate_limited", 429)
      case "server_error":
        throw new CallError(name, "provider_error", 500)
      default:
        throw new CallError(name, "call_timeout", 0)
    }
  }

  private checkResults(index: number, results: ToolResult[]): void {
    const prefix = `replay fixture ${this.fixture.file}: turn ${index}`
    if (results.length !== this.previous.length) {
      throw new ReplayError(
        `${prefix}: got ${results.length} tool results for ${this.previous.length} tool calls`
      )
    }
    for (const [i, result] of results.entries()) {
      const want = this.previous[i]?.id ?? ""
      if (result.callId !== want) {
        throw new ReplayError(
          `${prefix}: tool result ${i} answers ${goQuote(result.callId)}, want ${goQuote(want)}`
        )
      }
    }
    const expect = this.turns[index]?.expect
    for (const [i, result] of results.entries()) {
      this.checkResult(index, i, result, expect)
    }
  }

  private checkResult(
    index: number,
    i: number,
    result: ToolResult,
    expect: Expectation[] | undefined
  ): void {
    const call = this.previous[i]
    const tool = call?.name ?? ""
    const where = `replay fixture ${this.fixture.file}: turn ${index}: tool call ${i} (${tool})`
    const expectation = expect?.[i]
    const wantError = expectation?.isError ?? false
    if (result.isError !== wantError) {
      this.logExpectationFailure(index, i, tool, result, wantError, "")
      throw new ReplayError(
        `${where}: is_error was ${result.isError}, expected ${wantError}`
      )
    }
    if (
      expectation !== undefined &&
      !result.content.includes(expectation.contains)
    ) {
      this.logExpectationFailure(
        index,
        i,
        tool,
        result,
        wantError,
        expectation.contains
      )
      throw new ReplayError(
        `${where}: result does not contain ${goQuote(expectation.contains)}`
      )
    }
  }

  private logExpectationFailure(
    turn: number,
    call: number,
    tool: string,
    result: ToolResult,
    wantError: boolean,
    missing: string
  ): void {
    const code = toolErrorCode(result)
    this.logger("warn", "replay expectation failed", {
      fixture: this.fixture.file,
      turn,
      call,
      tool,
      is_error: result.isError,
      expected_is_error: wantError,
      result_length: Buffer.byteLength(result.content),
      ...(code === "" ? {} : { tool_error_code: code }),
      ...(missing === "" ? {} : { expected_contains: missing }),
    })
  }
}

function usage(outputTokens: number): Turn["usage"] {
  return {
    inputTokens: 0,
    outputTokens,
    cacheReadTokens: 0,
    cacheWriteTokens: 0,
    calls: 1,
  }
}

function answer(turn: Turn): TurnAnswer {
  return { turn, attempts: 1 }
}

const providers = new Map<string, Promise<ReplayProvider>>()

export function replayProvider(
  dir: string,
  maxTokens: number
): Promise<ReplayProvider> {
  const key = `${maxTokens}\u0000${dir}`
  let provider = providers.get(key)
  if (provider === undefined) {
    provider = ReplayProvider.load(dir, maxTokens)
    providers.set(key, provider)
    provider.catch(() => providers.delete(key))
  }
  return provider
}
