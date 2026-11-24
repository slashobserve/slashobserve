import { describe, expect, test } from "bun:test"
import { ErrorCode, McpError } from "@modelcontextprotocol/sdk/types.js"
import type { ClaimApi } from "../src/mcp"
import { ToolError, type Usage } from "../src/protocol"
import { Reporter, RunEnded, chunkText } from "../src/reporter"

describe("chunkText", () => {
  test("splits at code points, never inside a surrogate pair", () => {
    const text = "\u{1f600}".repeat(5)
    expect(chunkText(text, 2)).toEqual([
      "\u{1f600}\u{1f600}",
      "\u{1f600}\u{1f600}",
      "\u{1f600}",
    ])
  })

  test("gives no chunk for empty text and one for short text", () => {
    expect(chunkText("", 4000)).toEqual([])
    expect(chunkText("abc", 4000)).toEqual(["abc"])
  })

  test("an oversized turn text is sent in chunks of at most 4000 code points", () => {
    const chunks = chunkText("a".repeat(9000), 4000)
    expect(chunks.map((c) => c.length)).toEqual([4000, 4000, 1000])
  })
})

describe("Reporter live text", () => {
  async function flushed(
    pushes: (reporter: Reporter) => void
  ): Promise<string[]> {
    const sent: string[] = []
    const api = {
      reportRun: async (input: { text?: string }) => {
        if (input.text !== undefined) sent.push(input.text)
        return { state: "running", stop: false }
      },
    } as unknown as ClaimApi
    const reporter = new Reporter(
      api,
      { run_id: "r", attempt: 1 },
      { heartbeatMs: 1_000_000, textFlushMs: 1_000_000 },
      { onBuild: () => {}, onEnd: () => {} },
      new AbortController().signal
    )
    pushes(reporter)
    await reporter.flush()
    reporter.stop()
    return sent
  }

  test("separates the messages of two model turns", async () => {
    const sent = await flushed((reporter) => {
      reporter.startMessage()
      reporter.push("It reads errors and latency.")
      reporter.startMessage()
      reporter.push("Next I will check the labels.")
    })
    expect(sent.join("")).toBe(
      "It reads errors and latency.\n\nNext I will check the labels."
    )
  })

  test("adds nothing before the first message or after a silent turn", async () => {
    const sent = await flushed((reporter) => {
      reporter.startMessage()
      reporter.push("First.")
      reporter.startMessage()
      reporter.startMessage()
      reporter.push("")
      reporter.push("Second.")
    })
    expect(sent.join("")).toBe("First.\n\nSecond.")
  })
})

describe("Reporter usage reports", () => {
  const usage: Usage = {
    provider: "p",
    model: "m",
    input_tokens: 1,
    output_tokens: 1,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    model_calls: 1,
    failed_calls: 0,
    retries: 0,
    compactions: 0,
    renewals: 0,
    model_ms: 1,
    max_save_output_tokens: 0,
  }

  function reporterOver(
    reportRun: (input: unknown, signal?: AbortSignal) => Promise<unknown>,
    ended: RunEnded[] = [],
    usageBudgetMs?: number
  ): Reporter {
    return new Reporter(
      { reportRun } as unknown as ClaimApi,
      { run_id: "r", attempt: 1 },
      {
        heartbeatMs: 1_000_000,
        textFlushMs: 1_000_000,
        usageRetryMs: [1, 1],
        ...(usageBudgetMs === undefined ? {} : { usageBudgetMs }),
      },
      { onBuild: () => {}, onEnd: (e) => ended.push(e) },
      new AbortController().signal
    )
  }

  const timeout = () =>
    new McpError(ErrorCode.RequestTimeout, "Request timed out")

  test("retries a usage report that timed out twice before failing", async () => {
    let calls = 0
    const reporter = reporterOver(async () => {
      calls += 1
      if (calls < 3) throw timeout()
      return { state: "running", stop: false }
    })
    await reporter.report(usage)
    expect(calls).toBe(3)
  })

  test("fails after the third timed out usage report", async () => {
    let calls = 0
    const reporter = reporterOver(async () => {
      calls += 1
      throw timeout()
    })
    await expect(reporter.report(usage)).rejects.toBeInstanceOf(McpError)
    expect(calls).toBe(3)
  })

  test("a usage report never blocks longer than its budget", async () => {
    let calls = 0
    const reporter = reporterOver(
      (_input, signal) => {
        calls += 1
        return new Promise((_resolve, reject) => {
          signal?.addEventListener("abort", () => reject(timeout()))
        })
      },
      [],
      50
    )
    const started = Date.now()
    await expect(reporter.report(usage)).rejects.toBeInstanceOf(McpError)
    expect(Date.now() - started).toBeLessThan(1_000)
    expect(calls).toBe(1)
  })

  test("a text only report keeps its single retry", async () => {
    let calls = 0
    const reporter = reporterOver(async () => {
      calls += 1
      throw timeout()
    })
    reporter.push("thinking")
    await reporter.flush()
    reporter.stop()
    expect(calls).toBe(2)
  })

  test("not_your_run ends the run at once without a retry", async () => {
    let calls = 0
    const ended: RunEnded[] = []
    const reporter = reporterOver(async () => {
      calls += 1
      throw new ToolError("not_your_run", "no")
    }, ended)
    await expect(reporter.report(usage)).rejects.toBeInstanceOf(RunEnded)
    expect(calls).toBe(1)
    expect(ended[0]?.state).toBe("lost")
  })
})
