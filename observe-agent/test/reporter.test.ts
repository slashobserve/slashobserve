import { describe, expect, test } from "bun:test"
import type { ClaimApi } from "../src/mcp"
import { Reporter, chunkText } from "../src/reporter"

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
