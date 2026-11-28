import { describe, expect, test } from "bun:test"

import { draftFailure, draftLoop, runDraft } from "../src/draft"
import type { DraftApi } from "../src/mcp"
import {
  type ClaimDraftAnswer,
  type DraftJob,
  type FinishDraftAnswer,
  type FinishDraftInput,
  modelSettings,
} from "../src/protocol"
import {
  CallError,
  NoFixtureError,
  type Session,
  type SessionFactory,
  type Start,
  type ToolResult,
  type Turn,
} from "../src/session/types"

const job: DraftJob = {
  id: "draft_1",
  system: "You write queries.",
  prompt: "errors from checkout",
  context: "values of service_name: checkout",
  tools: [
    {
      name: "submit_query",
      description: "Answer with the query.",
      input_schema: { type: "object" },
    },
  ],
  model: modelSettings.parse({ provider: "anthropic", model: "m" }),
  timeout_seconds: 60,
}

function turn(input: string): Turn {
  return {
    text: "",
    toolCalls: [{ id: "call-1", name: "submit_query", input }],
    stop: "tool_use",
    usage: {
      inputTokens: 40,
      outputTokens: 4,
      cacheReadTokens: 1,
      cacheWriteTokens: 2,
      calls: 1,
    },
  }
}

class FakeDrafts implements DraftApi {
  finished: FinishDraftInput[] = []
  claims: Array<ClaimDraftAnswer | Error>

  constructor(
    private readonly answers: FinishDraftAnswer[],
    claims: Array<ClaimDraftAnswer | Error> = []
  ) {
    this.claims = claims
  }

  claimDraft(): Promise<ClaimDraftAnswer> {
    const next = this.claims.shift() ?? { draft: null }
    return next instanceof Error ? Promise.reject(next) : Promise.resolve(next)
  }

  finishDraft(input: FinishDraftInput): Promise<FinishDraftAnswer> {
    this.finished.push(input)
    return Promise.resolve(this.answers.shift() ?? { done: true, results: [] })
  }
}

function scripted(turns: Array<Turn | Error>): {
  factory: SessionFactory
  starts: Start[]
  sent: Array<ToolResult[] | undefined>
} {
  const starts: Start[] = []
  const sent: Array<ToolResult[] | undefined> = []
  const factory: SessionFactory = (_settings, start) => {
    starts.push(start)
    const session: Session = {
      next: (results) => {
        sent.push(results)
        const next = turns.shift()
        if (next === undefined || next instanceof Error) {
          return Promise.reject(next ?? new Error("no turn"))
        }
        return Promise.resolve({ turn: next, attempts: 1 })
      },
      compact: () => Promise.resolve(),
    }
    return Promise.resolve(session)
  }
  return { factory, starts, sent }
}

describe("runDraft", () => {
  test("sends each turn and the tool results back until the server ends the draft", async () => {
    const drafts = new FakeDrafts([
      {
        done: false,
        results: [{ call_id: "call-1", content: "fix it", is_error: true }],
      },
      { done: true, results: [] },
    ])
    const model = scripted([turn(`{"query":"x"}`), turn(`{"query":"y"}`)])
    await runDraft(drafts, job, model.factory, new AbortController().signal)

    expect(model.starts).toEqual([
      {
        system: "You write queries.",
        prompt: "errors from checkout",
        userMessage:
          "values of service_name: checkout\n\n<request>\nerrors from checkout\n</request>",
        buildSaves: 0,
        pruning: { keepTurns: 0, stepBytes: 0 },
      },
    ])
    expect(model.sent).toEqual([
      undefined,
      [{ callId: "call-1", content: "fix it", isError: true, prunable: false }],
    ])
    expect(drafts.finished.map((f) => f.turn?.tool_calls[0]?.input)).toEqual([
      { query: "x" },
      { query: "y" },
    ])
    expect(drafts.finished[0]?.usage).toEqual({
      input_tokens: 40,
      output_tokens: 4,
      cache_read_tokens: 1,
      cache_write_tokens: 2,
    })
  })

  test("reports a model failure once and stops", async () => {
    const drafts = new FakeDrafts([])
    const model = scripted([new CallError("anthropic", "rate_limited", 429)])
    await runDraft(drafts, job, model.factory, new AbortController().signal)
    expect(drafts.finished.map((f) => [f.failure, f.turn])).toEqual([
      ["rate_limited", undefined],
    ])
  })

  test("reports a session that could not start", async () => {
    const drafts = new FakeDrafts([])
    const factory: SessionFactory = () => Promise.reject(new NoFixtureError())
    await runDraft(drafts, job, factory, new AbortController().signal)
    expect(drafts.finished.map((f) => f.failure)).toEqual(["no_fixture"])
  })

  test("sends a tool input that is not JSON as text, for the server to refuse", async () => {
    const drafts = new FakeDrafts([])
    const model = scripted([turn("not json")])
    await runDraft(drafts, job, model.factory, new AbortController().signal)
    expect(drafts.finished[0]?.turn?.tool_calls[0]?.input).toBe("not json")
  })
})

describe("draftFailure", () => {
  test("names the failure the server maps to a draft error", () => {
    expect(draftFailure(new CallError("openai", "call_timeout"))).toBe(
      "call_timeout"
    )
    expect(draftFailure(new CallError("openai", "provider_error", 500))).toBe(
      "provider_error"
    )
    expect(draftFailure(new NoFixtureError())).toBe("no_fixture")
    expect(draftFailure(new Error("boom"))).toBe("error")
  })
})

describe("draftLoop", () => {
  test("answers each claimed draft and keeps polling after an idle claim or an error", async () => {
    const controller = new AbortController()
    const drafts = new FakeDrafts(
      [],
      [{ draft: null }, new Error("connection reset"), { draft: job }]
    )
    const model = scripted([turn(`{"query":"x"}`)])
    let claims = 0
    const counting: DraftApi = {
      claimDraft: () => {
        claims++
        if (claims > 3) {
          controller.abort()
        }
        return drafts.claimDraft()
      },
      finishDraft: (input) => drafts.finishDraft(input),
    }
    await draftLoop(counting, model.factory, controller.signal, {
      sleep: () => Promise.resolve(),
    })
    expect(drafts.finished.map((f) => f.id)).toEqual(["draft_1"])
    expect(claims).toBeGreaterThanOrEqual(4)
  })
})
