import { describe, expect, test } from "bun:test"
import { StreamableHTTPError } from "@modelcontextprotocol/sdk/client/streamableHttp.js"
import { ErrorCode, McpError } from "@modelcontextprotocol/sdk/types.js"
import { ToolError } from "../src/protocol"
import { retryWithin } from "../src/retry"

const policy = { waitsMs: [1, 1, 1], budgetMs: 5_000 }
const timeout = () => new McpError(ErrorCode.RequestTimeout, "timed out")

describe("retryWithin", () => {
  test("retries a timeout and returns the first success", async () => {
    let calls = 0
    const answer = await retryWithin(
      async () => {
        calls += 1
        if (calls < 3) throw timeout()
        return "ok"
      },
      new AbortController().signal,
      policy
    )
    expect(answer).toBe("ok")
    expect(calls).toBe(3)
  })

  test("stops after the last wait and throws the last error", async () => {
    let calls = 0
    await expect(
      retryWithin(
        async () => {
          calls += 1
          throw timeout()
        },
        new AbortController().signal,
        policy
      )
    ).rejects.toBeInstanceOf(McpError)
    expect(calls).toBe(4)
  })

  test("a call that never answers is cut at the budget", async () => {
    let calls = 0
    const started = Date.now()
    await expect(
      retryWithin(
        (signal) => {
          calls += 1
          return new Promise((_resolve, reject) => {
            signal.addEventListener("abort", () => reject(timeout()))
          })
        },
        new AbortController().signal,
        { waitsMs: [1, 1], budgetMs: 50 }
      )
    ).rejects.toBeInstanceOf(McpError)
    expect(Date.now() - started).toBeLessThan(1_000)
    expect(calls).toBe(1)
  })

  test("a tool error is not retried", async () => {
    let calls = 0
    await expect(
      retryWithin(
        async () => {
          calls += 1
          throw new ToolError("run_ended", "the run ended")
        },
        new AbortController().signal,
        policy
      )
    ).rejects.toBeInstanceOf(ToolError)
    expect(calls).toBe(1)
  })

  test("a client error status is not retried but a 500 is", async () => {
    let conflicts = 0
    await expect(
      retryWithin(
        async () => {
          conflicts += 1
          throw new StreamableHTTPError(409, "run_ended")
        },
        new AbortController().signal,
        policy
      )
    ).rejects.toBeInstanceOf(StreamableHTTPError)
    expect(conflicts).toBe(1)

    let failures = 0
    const answer = await retryWithin(
      async () => {
        failures += 1
        if (failures === 1) throw new StreamableHTTPError(500, "internal")
        return "ok"
      },
      new AbortController().signal,
      policy
    )
    expect(answer).toBe("ok")
    expect(failures).toBe(2)
  })

  test("an abort stops the retrying", async () => {
    const controller = new AbortController()
    let calls = 0
    await expect(
      retryWithin(
        async () => {
          calls += 1
          controller.abort()
          throw timeout()
        },
        controller.signal,
        policy
      )
    ).rejects.toBeInstanceOf(McpError)
    expect(calls).toBe(1)
  })

  test("an abort during the wait ends it at once", async () => {
    const controller = new AbortController()
    let calls = 0
    const started = Date.now()
    const result = retryWithin(
      async () => {
        calls += 1
        throw timeout()
      },
      controller.signal,
      { waitsMs: [10_000, 10_000], budgetMs: 60_000 }
    )
    setTimeout(() => controller.abort(), 20)
    await expect(result).rejects.toBeInstanceOf(McpError)
    expect(Date.now() - started).toBeLessThan(1_000)
    expect(calls).toBe(2)
  })
})
