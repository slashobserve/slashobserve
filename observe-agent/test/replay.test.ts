import { afterAll, describe, expect, test } from "bun:test"
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { configureLog, createLogger } from "../src/log"
import {
  ReplayProvider,
  estimateOutputTokens,
  promptKeyHash,
  replayProvider,
} from "../src/session/replay/replay"
import {
  CallError,
  NoFixtureError,
  SessionError,
  type Session,
  type Start,
  type ToolResult,
  type Turn,
} from "../src/session/types"

const roots: string[] = []

configureLog("error")

afterAll(async () => {
  await Promise.all(roots.map((root) => rm(root, { recursive: true })))
})

async function fixtureDir(files: Record<string, string>): Promise<string> {
  const dir = await mkdtemp(join(tmpdir(), "observe-agent-replay-"))
  roots.push(dir)
  for (const [name, content] of Object.entries(files)) {
    await writeFile(join(dir, name), content)
  }
  return dir
}

function capture() {
  const lines: Array<Record<string, unknown>> = []
  const raw: string[] = []
  const logger = createLogger((line) => {
    raw.push(line)
    lines.push(JSON.parse(line) as Record<string, unknown>)
  }, "debug")
  return {
    logger,
    lines,
    raw,
    find(msg: string): Record<string, unknown> {
      const found = lines.find((line) => line.msg === msg)
      if (found === undefined) {
        throw new Error(`no log line ${msg}`)
      }
      return found
    },
  }
}

async function loadProvider(
  content: string,
  maxTokens = 0
): Promise<ReplayProvider> {
  const dir = await fixtureDir({ "f.json": content })
  return ReplayProvider.load(dir, maxTokens, capture().logger)
}

const noSignal = new AbortController().signal

function startOf(buildSaves = 0, prompt = "p"): Start {
  return {
    system: "",
    tools: [],
    prompt,
    userMessage: "",
    buildSaves,
    pruning: { keepTurns: 4, stepBytes: 65536 },
  }
}

const io = { signal: noSignal, onText: () => {} }

async function newSession(
  maxTokens: number,
  content: string
): Promise<Session> {
  const provider = await loadProvider(content, maxTokens)
  return provider.start(startOf(), noSignal)
}

function result(callId: string, extra: Partial<ToolResult> = {}): ToolResult {
  return { callId, content: "", isError: false, prunable: false, ...extra }
}

async function failureOf(promise: Promise<unknown>): Promise<unknown> {
  try {
    await promise
  } catch (error) {
    return error
  }
  throw new Error("the promise resolved")
}

const compaction = {
  message: "shortened",
  tailTurns: 4,
  tailBytes: 1 << 20,
  resultBytes: 100,
}

describe("injected call errors", () => {
  const rows = [
    { kind: "rate_limited", failure: "rate_limited", status: 429 },
    { kind: "server_error", failure: "provider_error", status: 500 },
    { kind: "call_timeout", failure: "call_timeout", status: 0 },
  ] as const
  for (const row of rows) {
    test(`${row.kind} fails once then the turn answers`, async () => {
      const session = await newSession(
        0,
        `{"prompt":"p","turns":[{"fault":{"kind":"${row.kind}"},"text":"recovered"}]}`
      )
      const error = await failureOf(session.next(undefined, io))
      expect(error).toBeInstanceOf(CallError)
      expect((error as CallError).failure).toBe(row.failure)
      expect((error as CallError).status).toBe(row.status)
      const { turn, attempts } = await session.next(undefined, io)
      expect(turn.text).toBe("recovered")
      expect(turn.stop).toBe("end_turn")
      expect(attempts).toBe(1)
    })
  }

  test("a spent fault does not carry to the next turn", async () => {
    const session = await newSession(
      0,
      `{"prompt":"p","turns":[` +
        `{"fault":{"kind":"server_error"},"tool_calls":[{"name":"t"}]},` +
        `{"fault":{"kind":"server_error"},"text":"done"}]}`
    )
    await failureOf(session.next(undefined, io))
    const { turn: first } = await session.next(undefined, io)
    expect(first.toolCalls).toHaveLength(1)
    const error = await failureOf(
      session.next([result(first.toolCalls[0]?.id ?? "")], io)
    )
    expect(error).toBeInstanceOf(CallError)
  })
})

describe("injected stop reasons", () => {
  for (const stop of ["max_tokens", "refusal"] as const) {
    test(stop, async () => {
      const session = await newSession(
        0,
        `{"prompt":"p","turns":[{"fault":{"kind":"${stop}"},"tool_calls":[{"name":"t"}]}]}`
      )
      const { turn } = await session.next(undefined, io)
      expect(turn.stop).toBe(stop)
      expect(turn.toolCalls).toHaveLength(0)
      expect(turn.usage.calls).toBe(1)
    })
  }
})

describe("delay", () => {
  test("waits then answers", async () => {
    const session = await newSession(
      0,
      `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":50},"text":"late"}]}`
    )
    const started = performance.now()
    const { turn } = await session.next(undefined, io)
    expect(turn.text).toBe("late")
    expect(performance.now() - started).toBeGreaterThanOrEqual(45)
  })

  test("stops when the signal aborts", async () => {
    const session = await newSession(
      0,
      `{"prompt":"p","turns":[{"fault":{"kind":"delay","delay_ms":60000},"text":"late"}]}`
    )
    const controller = new AbortController()
    setTimeout(() => controller.abort(new Error("stopped")), 20)
    const error = await failureOf(
      session.next(undefined, { signal: controller.signal, onText: () => {} })
    )
    expect((error as Error).message).toBe("stopped")
  })
})

function inputOf3200Bytes(): string {
  return `{"v":"${"a".repeat(3192)}"}`
}

describe("output tokens", () => {
  test("are estimated from tool call input", async () => {
    const input = inputOf3200Bytes()
    expect(Buffer.byteLength(input)).toBe(3200)
    const session = await newSession(
      0,
      `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":${input}},{"name":"t","input":{"k":1}}]}]}`
    )
    const { turn } = await session.next(undefined, io)
    expect(turn.usage.outputTokens).toBe(1003)
  })

  test("count utf-8 bytes", () => {
    expect(estimateOutputTokens(["éééé"])).toBe(3)
    expect(estimateOutputTokens([])).toBe(0)
  })

  test("an oversized turn stops at the cap without running calls", async () => {
    const content = `{"prompt":"p","turns":[{"tool_calls":[{"name":"t","input":${inputOf3200Bytes()}}]}]}`
    const { turn } = await (await newSession(999, content)).next(undefined, io)
    expect(turn.stop).toBe("max_tokens")
    expect(turn.toolCalls).toHaveLength(0)
    expect(turn.usage.outputTokens).toBe(999)
    const { turn: atCap } = await (
      await newSession(1000, content)
    ).next(undefined, io)
    expect(atCap.stop).toBe("tool_use")
    expect(atCap.toolCalls).toHaveLength(1)
  })
})

const resumeFixture = `{"prompt":"p",
"turns":[{"tool_calls":[{"name":"save"}]},{"expect":[{"is_error":false}],"text":"first segment done"}],
"resumes":{
 "1":{"turns":[{"text":"after one save"}]},
 "3":{"turns":[{"fault":{"kind":"server_error"},"tool_calls":[{"name":"save"}]},{"tool_calls":[{"name":"save"}],"expect":[{"is_error":false}]}]}
}}`

const saveAfterFixture =
  `{"prompt":"p","resume_after_saves":true,"turns":[` +
  `{"tool_calls":[{"name":"create_dashboard"}]},` +
  `{"tool_calls":[{"name":"list_metrics"}]},` +
  `{"tool_calls":[{"name":"update_panels"}]},` +
  `{"tool_calls":[{"name":"update_panels"}]}]}`

describe("resume segments", () => {
  test("the build save count picks the segment", async () => {
    const provider = await loadProvider(resumeFixture)
    const rows = [
      { buildSaves: 0, text: "", calls: 1 },
      { buildSaves: 1, text: "after one save", calls: 0 },
    ]
    for (const row of rows) {
      const session = provider.start(startOf(row.buildSaves), noSignal)
      const { turn } = await session.next(undefined, io)
      expect(turn.text).toBe(row.text)
      expect(turn.toolCalls).toHaveLength(row.calls)
    }
  })

  test("a missing segment is no fixture", async () => {
    const provider = await loadProvider(resumeFixture)
    for (const buildSaves of [2, 4, -1]) {
      expect(() => provider.start(startOf(buildSaves), noSignal)).toThrow(
        NoFixtureError
      )
    }
    const noResumes = await loadProvider(
      `{"prompt":"p","turns":[{"text":"x"}]}`
    )
    expect(() => noResumes.start(startOf(1), noSignal)).toThrow(NoFixtureError)
  })

  test("a fault in a segment fires", async () => {
    const provider = await loadProvider(resumeFixture)
    const session = provider.start(startOf(3), noSignal)
    const error = await failureOf(session.next(undefined, io))
    expect((error as CallError).failure).toBe("provider_error")
    const { turn } = await session.next(undefined, io)
    expect(turn.toolCalls).toHaveLength(1)
  })

  test("past the last turn of a segment the session ends the turn", async () => {
    const provider = await loadProvider(resumeFixture)
    const session = provider.start(startOf(1), noSignal)
    await session.next(undefined, io)
    await session.compact(compaction)
    const { turn } = await session.next(undefined, io)
    expect(turn.stop).toBe("end_turn")
    expect(turn.toolCalls).toHaveLength(0)
  })

  test("resume after saves serves the resumed session", async () => {
    const provider = await loadProvider(saveAfterFixture)
    const session = provider.start(startOf(2), noSignal)
    const { turn } = await session.next(undefined, io)
    expect(turn.toolCalls.map((c) => c.name)).toEqual(["update_panels"])
    expect(() => provider.start(startOf(4), noSignal)).toThrow(NoFixtureError)
  })
})

describe("past the last turn", () => {
  const single = `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"},{"name":"t"}]}]}`

  test("the session ends the turn counting one call", async () => {
    const session = await newSession(0, single)
    const { turn: first } = await session.next(undefined, io)
    expect(first.toolCalls).toHaveLength(2)
    const { turn: end } = await session.next(
      first.toolCalls.map((c) => result(c.id)),
      io
    )
    expect(end.stop).toBe("end_turn")
    expect(end.toolCalls).toHaveLength(0)
    expect(end.text).toBe("")
    expect(end.usage.calls).toBe(1)
  })

  const rows: Array<{
    name: string
    results: (first: Turn) => ToolResult[]
    want: string
  }> = [
    {
      name: "missing result",
      results: () => [],
      want: "got 0 tool results for 1 tool calls",
    },
    {
      name: "wrong id",
      results: () => [result("other")],
      want: `tool result 0 answers "other"`,
    },
    {
      name: "unexpected error",
      results: (first) => [
        result(first.toolCalls[0]?.id ?? "", { isError: true }),
      ],
      want: "is_error was true, expected false",
    },
  ]
  for (const row of rows) {
    test(`still checks results: ${row.name}`, async () => {
      const session = await newSession(
        0,
        `{"prompt":"p","turns":[{"tool_calls":[{"name":"t"}]}]}`
      )
      const { turn: first } = await session.next(undefined, io)
      const error = await failureOf(session.next(row.results(first), io))
      expect((error as Error).message).toContain(row.want)
    })
  }
})

describe("sessions after a turn without tool calls", () => {
  const nudgeFixture = `{"prompt":"p","turns":[{"tool_calls":[{"name":"save"}]},{"text":"that is all"},{"tool_calls":[{"name":"save"}]}]}`

  test("need a compaction first", async () => {
    const session = await newSession(0, nudgeFixture)
    const { turn: first } = await session.next(undefined, io)
    const { turn: answer } = await session.next(
      [result(first.toolCalls[0]?.id ?? "")],
      io
    )
    expect(answer.text).toBe("that is all")
    expect(answer.toolCalls).toHaveLength(0)
    const error = await failureOf(session.next(undefined, io))
    expect((error as Error).message).toContain("the session has ended")
    await session.compact(compaction)
    const { turn: third } = await session.next(undefined, io)
    expect(third.toolCalls).toHaveLength(1)
    expect(third.stop).toBe("tool_use")
  })

  test("compaction keeps the replay position", async () => {
    const provider = await loadProvider(resumeFixture)
    const session = provider.start(startOf(), noSignal)
    const { turn: first } = await session.next(undefined, io)
    await session.compact(compaction)
    const { turn: second } = await session.next(
      [result(first.toolCalls[0]?.id ?? "", { content: "{}" })],
      io
    )
    expect(second.text).toBe("first segment done")
  })
})

describe("live text", () => {
  test("a turn's text goes to onText once", async () => {
    const session = await newSession(
      0,
      `{"prompt":"p","turns":[{"text":"hello","tool_calls":[{"name":"t"}]}]}`
    )
    const texts: string[] = []
    await session.next(undefined, {
      signal: noSignal,
      onText: (text) => texts.push(text),
    })
    expect(texts).toEqual(["hello"])
  })
})

const loggingFixture = `{
  "prompt": "show checkout health",
  "turns": [
    {"tool_calls": [{"name": "run_query", "input": {"q": "up"}}]},
    {"text": "done", "expect": [{"is_error": false, "contains": "ok"}]}
  ]
}`

async function loggedProvider() {
  const dir = await fixtureDir({ "checkout.json": loggingFixture })
  const logs = capture()
  const provider = await ReplayProvider.load(dir, 0, logs.logger)
  return { provider, logs }
}

describe("logging", () => {
  test("a fixture miss logs length and hash, not the prompt", async () => {
    const { provider, logs } = await loggedProvider()
    const prompt = "secret prompt about db.internal"
    expect(() => provider.start(startOf(0, prompt), noSignal)).toThrow(
      NoFixtureError
    )
    const record = logs.find("replay found no fixture for the prompt")
    expect(record.prompt_length).toBe(prompt.length)
    expect(String(record.prompt_key_sha256)).toHaveLength(12)
    const text = logs.raw.join("\n")
    expect(text).not.toContain("secret")
    expect(text).not.toContain("db.internal")
  })

  test("a failed expectation logs the shape, not the result", async () => {
    const { provider, logs } = await loggedProvider()
    const session = provider.start(startOf(0, "Show checkout health"), noSignal)
    const { turn } = await session.next(undefined, io)
    const content = `{"code":"query_invalid","message":"leaked-detail up{pod=\\"x\\"}"}`
    const error = await failureOf(
      session.next(
        [result(turn.toolCalls[0]?.id ?? "", { isError: true, content })],
        io
      )
    )
    expect((error as Error).message).toContain(
      "is_error was true, expected false"
    )
    expect(logs.find("replay expectation failed")).toMatchObject({
      fixture: "checkout.json",
      turn: 1,
      call: 0,
      tool: "run_query",
      is_error: true,
      expected_is_error: false,
      result_length: content.length,
      tool_error_code: "query_invalid",
    })
    const text = logs.raw.join("\n")
    expect(text).not.toContain("leaked-detail")
    expect(text).not.toContain("pod=")
  })

  test("a missing text expectation is logged at warn with the expected text", async () => {
    const { provider, logs } = await loggedProvider()
    const session = provider.start(startOf(0, "Show checkout health"), noSignal)
    const { turn } = await session.next(undefined, io)
    const error = await failureOf(
      session.next(
        [result(turn.toolCalls[0]?.id ?? "", { content: "leaked-detail" })],
        io
      )
    )
    expect((error as Error).message).toContain(`does not contain "ok"`)
    expect(logs.find("replay expectation failed")).toMatchObject({
      level: "warn",
      expected_contains: "ok",
      tool: "run_query",
    })
    expect(logs.raw.join("\n")).not.toContain("leaked-detail")
  })

  test("loaded fixtures log the hash a miss is matched against", async () => {
    const { logs } = await loggedProvider()
    expect(logs.find("replay fixture loaded")).toMatchObject({
      fixture: "checkout.json",
      prompt_key_sha256: promptKeyHash("Show   checkout HEALTH"),
    })
  })

  test("loading warns that replay is for tests only", async () => {
    const { logs } = await loggedProvider()
    expect(logs.find("replay fixtures loaded").count).toBe(1)
    expect(
      logs.lines.some(
        (line) =>
          line.level === "warn" &&
          String(line.msg).includes("use it for tests only")
      )
    ).toBe(true)
  })
})

describe("loading", () => {
  test("an unset directory is an error", async () => {
    const error = await failureOf(ReplayProvider.load("", 0, capture().logger))
    expect(error).toBeInstanceOf(SessionError)
    expect((error as Error).message).toBe("replay fixture directory is not set")
  })

  test("a missing directory is an error", async () => {
    const dir = join(tmpdir(), "observe-agent-no-such-dir")
    const error = await failureOf(ReplayProvider.load(dir, 0, capture().logger))
    expect((error as Error).message).toBe(
      `read replay fixture directory: open ${dir}: no such file or directory`
    )
  })

  test("two fixtures with the same prompt key are an error", async () => {
    const dir = await fixtureDir({
      "a.json": `{"prompt":"Hello  World","turns":[{"text":"x"}]}`,
      "b.json": `{"prompt":"hello world","turns":[{"text":"y"}]}`,
    })
    const error = await failureOf(ReplayProvider.load(dir, 0, capture().logger))
    expect((error as Error).message).toBe(
      "replay fixtures a.json and b.json have the same prompt"
    )
  })

  test("every invalid fixture is reported, one per line, in file order", async () => {
    const dir = await fixtureDir({
      "b.json": `{"prompt":"p","turns":[{}]}`,
      "a.json": `{`,
    })
    const error = await failureOf(ReplayProvider.load(dir, 0, capture().logger))
    expect((error as Error).message.split("\n")).toEqual([
      "replay fixture a.json: unexpected EOF",
      "replay fixture b.json: turns: turn 0: needs text or tool_calls",
    ])
  })

  test("only .json files are read and subdirectories are skipped", async () => {
    const dir = await fixtureDir({
      "ok.json": `{"prompt":"p","turns":[{"text":"x"}]}`,
      "notes.txt": "not a fixture",
    })
    await mkdir(join(dir, "nested.json"))
    const logs = capture()
    await ReplayProvider.load(dir, 0, logs.logger)
    expect(logs.find("replay fixtures loaded").count).toBe(1)
  })

  test("a started session needs a live signal", async () => {
    const provider = await loadProvider(`{"prompt":"p","turns":[{"text":"x"}]}`)
    const controller = new AbortController()
    controller.abort(new Error("gone"))
    expect(() => provider.start(startOf(), controller.signal)).toThrow("gone")
  })

  test("the provider cache shares one load per directory and token cap", async () => {
    const dir = await fixtureDir({
      "ok.json": `{"prompt":"p","turns":[{"text":"x"}]}`,
    })
    const first = replayProvider(dir, 5)
    expect(replayProvider(dir, 5)).toBe(first)
    expect(replayProvider(dir, 6)).not.toBe(first)
    await first
  })

  test("a failed load is not cached", async () => {
    const dir = await fixtureDir({ "bad.json": "{" })
    const failed = replayProvider(dir, 0)
    await failureOf(failed)
    await writeFile(
      join(dir, "bad.json"),
      `{"prompt":"p","turns":[{"text":"x"}]}`
    )
    const retried = replayProvider(dir, 0)
    expect(retried).not.toBe(failed)
    await retried
  })
})

describe("recorded fixtures", () => {
  const repoRoot = resolve(import.meta.dir, "..", "..")
  const directories = [
    "e2e/fixtures/llm",
    "evals/fixtures/llm",
    "deploy/compose/replicas/llm",
  ]
  for (const directory of directories) {
    test(`${directory} loads without error`, async () => {
      const provider = await ReplayProvider.load(
        join(repoRoot, directory),
        0,
        capture().logger
      )
      expect(provider).toBeInstanceOf(ReplayProvider)
    })
  }
})
