import { describe, expect, test } from "bun:test"
import { executeTurn, maxParallelTools } from "../src/turn"
import type { Usage } from "../src/protocol"
import { meta, progress, reply, FakeTools } from "./harness"

const signal = new AbortController().signal

const usage: Usage = {
  provider: "replay",
  model: "replay",
  input_tokens: 0,
  output_tokens: 7,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  model_calls: 1,
  failed_calls: 0,
  retries: 0,
  compactions: 0,
  renewals: 0,
  model_ms: 0,
  max_save_output_tokens: 0,
}

function calls(...names: string[]) {
  return names.map((name, i) => ({ id: `replay_0_${i}`, name, input: "{}" }))
}

describe("executeTurn", () => {
  test("reads run before saves and saves run in order", async () => {
    const tools = new FakeTools()
    tools.handler = async () => reply()
    await executeTurn(
      tools,
      calls("create_dashboard", "list_metrics", "update_panels", "lint_query"),
      "turn-1",
      usage,
      signal
    )
    expect(tools.calls.map((c) => c.name)).toEqual([
      "list_metrics",
      "lint_query",
      "create_dashboard",
      "update_panels",
    ])
  })

  test("reads run at most four at once", async () => {
    const tools = new FakeTools()
    let inFlight = 0
    let peak = 0
    tools.handler = async () => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await new Promise((resolve) => setTimeout(resolve, 15))
      inFlight--
      return reply()
    }
    await executeTurn(
      tools,
      calls(...Array.from({ length: 9 }, () => "list_metrics")),
      "turn-1",
      usage,
      signal
    )
    expect(maxParallelTools).toBe(4)
    expect(peak).toBe(4)
    expect(tools.calls).toHaveLength(9)
  })

  test("saves never overlap", async () => {
    const tools = new FakeTools()
    let inFlight = 0
    let peak = 0
    tools.handler = async () => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await new Promise((resolve) => setTimeout(resolve, 5))
      inFlight--
      return reply()
    }
    await executeTurn(
      tools,
      calls("update_panels", "update_panels", "update_panels"),
      "turn-1",
      usage,
      signal
    )
    expect(peak).toBe(1)
  })

  test("every call carries the turn id, the call count and the save count", async () => {
    const tools = new FakeTools()
    await executeTurn(
      tools,
      calls("list_metrics", "save_finding", "update_panels"),
      "turn-9",
      usage,
      signal
    )
    for (const call of tools.calls) {
      expect(call.turn).toEqual({ id: "turn-9", calls: 3, saves: 2, usage })
    }
  })

  test("results come back in call order whatever finishes first", async () => {
    const tools = new FakeTools()
    tools.handler = async (call) => {
      await new Promise((resolve) =>
        setTimeout(resolve, call.name === "slow_read" ? 30 : 1)
      )
      return reply({ content: call.name })
    }
    const out = await executeTurn(
      tools,
      calls("slow_read", "fast_read", "create_dashboard"),
      "t",
      usage,
      signal
    )
    expect(out.results.map((r) => r.content)).toEqual([
      "slow_read",
      "fast_read",
      "create_dashboard",
    ])
    expect(out.results.map((r) => r.callId)).toEqual([
      "replay_0_0",
      "replay_0_1",
      "replay_0_2",
    ])
  })

  test("prunable is set for successful reads only", async () => {
    const tools = new FakeTools()
    tools.handler = async (call) => reply({ isError: call.name === "bad_read" })
    const out = await executeTurn(
      tools,
      calls(
        "good_read",
        "bad_read",
        "update_panels",
        "import_grafana_dashboard"
      ),
      "t",
      usage,
      signal
    )
    expect(out.results.map((r) => r.prunable)).toEqual([
      true,
      false,
      false,
      false,
    ])
    expect(out.results.map((r) => r.isError)).toEqual([
      false,
      true,
      false,
      false,
    ])
  })

  test("run metas come back in call order", async () => {
    const tools = new FakeTools()
    tools.handler = async (call) =>
      call.name === "a"
        ? reply({ meta: meta("saved", progress(2)) })
        : reply({ meta: meta(null, progress(1)) })
    const out = await executeTurn(tools, calls("a", "b"), "t", usage, signal)
    expect(out.metas.map((m) => m.build?.saves)).toEqual([2, 1])
  })

  test("a tool input that is not a JSON object is refused locally", async () => {
    const tools = new FakeTools()
    const out = await executeTurn(
      tools,
      [
        { id: "x0", name: "list_metrics", input: "[1]" },
        { id: "x1", name: "list_metrics", input: "{not json" },
        { id: "x2", name: "lint_query", input: "{}" },
      ],
      "t",
      usage,
      signal
    )
    expect(tools.calls.map((c) => c.name)).toEqual(["lint_query"])
    expect(tools.calls[0]?.turn?.calls).toBe(1)
    expect(out.results[0]).toMatchObject({ isError: true, prunable: false })
    expect(JSON.parse(out.results[1]?.content ?? "")).toMatchObject({
      code: "invalid_arguments",
    })
  })

  test("a failing call becomes an error result and the others still run", async () => {
    const tools = new FakeTools()
    tools.handler = async (call) => {
      if (call.name === "broken") {
        throw new Error("socket closed")
      }
      return reply()
    }
    const out = await executeTurn(
      tools,
      calls("broken", "fine"),
      "t",
      usage,
      signal
    )
    expect(out.results[0]).toMatchObject({ isError: true, prunable: false })
    expect(JSON.parse(out.results[0]?.content ?? "")).toEqual({
      code: "tool_unavailable",
      message: "the tool call did not complete",
    })
    expect(out.results[0]?.content).not.toContain("socket")
    expect(out.results[1]?.isError).toBe(false)
  })

  test("an aborted signal rethrows", async () => {
    const tools = new FakeTools()
    const controller = new AbortController()
    tools.handler = async () => {
      controller.abort(new Error("stopped"))
      throw new Error("aborted")
    }
    await expect(
      executeTurn(tools, calls("list_metrics"), "t", usage, controller.signal)
    ).rejects.toThrow("aborted")
  })
})
