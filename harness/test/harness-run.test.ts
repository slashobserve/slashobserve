import { describe, expect, test } from "bun:test"
import { configureLog } from "../src/log"
import type { AgentSettings, RunBrief } from "../src/protocol"
import { runLoop } from "../src/run"
import { stepRouted } from "../src/runstate"
import type {
  Session,
  SessionFactory,
  Start,
  ToolResult,
  ToolSpec,
} from "../src/session/types"
import { FakeClaim, claimed, factory, meta, reply } from "./harness"

configureLog("error")

const idle = { heartbeatMs: 60_000, textFlushMs: 60_000 }
const investigatePrompt = "What is the p95 latency of checkout?"

function structured(
  mode: string,
  prompt: string,
  investigation: boolean
): RunBrief {
  return {
    request: {
      mode,
      source: "ask",
      prompt,
      panel_id: null,
      from: null,
      to: null,
      time_zone: null,
      zone: null,
      as_of: null,
      continue_note: null,
      thread_context: null,
      continues: false,
      attempt: 1,
      investigation,
      observations: [],
      pointer: {
        anchor: null,
        block_id: null,
        observation_id: null,
        thread_ask: false,
      },
    },
    current: null,
    build: null,
    datasources: [
      {
        uid: "prom",
        type: "prometheus",
        name: "Prometheus",
        spans: "",
        healthy: true,
        everyone: true,
      },
    ],
    withheld: false,
    guide: { text: "THE GUIDE", schema: {} },
    draft: null,
  }
}

const specs: ToolSpec[] = [
  "list_metrics",
  "query_range",
  "read_series",
  "list_services",
  "save_finding",
].map((name) => ({ name, description: "", inputSchema: {}, readOnly: true }))

type Seen = { starts: Start[]; tools: string[][]; results: ToolResult[][] }

function watching(): { factory: SessionFactory; seen: Seen } {
  const seen: Seen = { starts: [], tools: [], results: [] }
  const watched: SessionFactory = async (settings, start, signal) => {
    seen.starts.push(start)
    const session = await factory(settings, start, signal)
    const wrapped: Session = {
      next: (results, io) => {
        seen.tools.push(io.tools.map((t) => t.name))
        seen.results.push(results ?? [])
        return session.next(results, io)
      },
      compact: (c) => session.compact(c),
    }
    return wrapped
  }
  return { factory: watched, seen }
}

function investigation(agent: Partial<AgentSettings> = {}) {
  const claim = new FakeClaim()
  claim.tools.specs = specs
  claim.brief = () => ({
    system: "",
    message: "",
    brief: structured("investigate", investigatePrompt, true),
  })
  claim.tools.handler = async (call) =>
    call.name === "save_finding"
      ? reply({ meta: meta("finding") })
      : reply({ content: '{"result":[]}' })
  const run = claimed(investigatePrompt, {
    kind: "investigate",
    agent: {
      brief_sweep: true,
      loop: true,
      grounding: "enforce",
      default_calls: 30,
      task_calls: { measure: 16 },
      ...agent,
    },
  })
  return { claim, run }
}

describe("a harness investigation", () => {
  test("is routed and swept before its tools are listed, and its task narrows them", async () => {
    const { claim, run } = investigation()
    const order: string[] = []
    const list = claim.tools.listTools.bind(claim.tools)
    claim.tools.listTools = async () => {
      order.push("list")
      return list()
    }
    const report = claim.report
    claim.report = (input, nth) => {
      order.push(input.task === undefined ? "report" : `report ${input.task}`)
      return report(input, nth)
    }
    const { factory: watched, seen } = watching()
    await runLoop(claim, run, watched, new AbortController().signal, idle)

    expect(order.slice(0, 2)).toEqual(["report measure", "list"])
    const steps = claim.reports.flatMap((r) => r.steps ?? [])
    expect(steps[0]).toEqual({
      phase: "discover",
      status: "ok",
      message: stepRouted("measure"),
    })
    expect(steps[0]?.message).toBe(
      "Routed the question to the measure task type"
    )
    expect(steps[1]?.message).toStartWith(
      "Swept the datasources before the first model call: "
    )
    const sweepCalls = claim.tools.calls.filter((c) => c.hints?.sweep)
    expect(sweepCalls.length).toBeGreaterThan(0)
    expect(sweepCalls.every((c) => c.turn === undefined)).toBe(true)
    expect(seen.tools[0]).not.toContain("read_series")
    expect(seen.tools[0]).toContain("query_range")
    expect(seen.starts[0]?.system).toContain("# Authoring guide\nTHE GUIDE")
    expect(seen.starts[0]?.userMessage).toContain(
      "<request>\n" + investigatePrompt + "\n</request>"
    )
  })

  test("replays an identical read with the Go step text and note", async () => {
    const { claim, run } = investigation({ brief_sweep: false })
    const { factory: watched, seen } = watching()
    await runLoop(claim, run, watched, new AbortController().signal, idle)
    const reads = claim.tools.calls.filter((c) => c.name === "list_metrics")
    expect(reads).toHaveLength(2)
    expect(reads[0]?.hints).toBeUndefined()
    expect(reads[1]?.hints).toEqual({
      step: "Reused the result of an identical call",
    })
    expect(seen.results[2]?.[0]?.content).toContain(
      "<note>This is the result of the same earlier call. A third identical call is refused.</note>"
    )
  })

  test("sends an ungrounded finding back once, then saves it with what it could not back", async () => {
    const { claim, run } = investigation({ brief_sweep: false })
    await runLoop(claim, run, factory, new AbortController().signal, idle)
    const saves = claim.tools.calls.filter((c) => c.name === "save_finding")
    expect(saves).toHaveLength(1)
    expect(saves[0]?.args["markdown"]).toBe(
      "Checkout p95 latency is 2.1 s over the hour."
    )
    expect(saves[0]?.args["unverified"]).toEqual([
      { kind: "figure", text: "2.1 s" },
    ])
    expect(saves[0]?.args["reads_refused"]).toBe(0)
    const steps = claim.reports.flatMap((r) => r.steps ?? [])
    expect(
      steps.some(
        (s) =>
          s.tool === "save_finding" &&
          s.status === "error" &&
          s.message.startsWith("The answer was sent back for a fix: ")
      )
    ).toBe(true)
    expect(claim.finishes[0]?.outcome).toBe("succeeded")
  })

  test("keeps its state in agent_state and resumes from it without routing or sweeping again", async () => {
    const first = investigation()
    await runLoop(
      first.claim,
      first.run,
      factory,
      new AbortController().signal,
      idle
    )
    const kept = first.claim.reports
      .map((r) => r.agent_state)
      .filter((s) => s !== undefined)
      .at(0)
    expect(kept).toBeDefined()
    expect((kept?.session as { task: string }).task).toBe("measure")

    const { claim, run } = investigation()
    claim.brief = () => ({
      system: "",
      message: "",
      brief: structured("investigate", investigatePrompt, true),
      agent_state: kept ?? null,
    })
    await runLoop(claim, run, factory, new AbortController().signal, idle)
    expect(claim.tools.calls.some((c) => c.hints?.sweep)).toBe(false)
    const steps = claim.reports.flatMap((r) => r.steps ?? [])
    expect(steps.some((s) => s.message.startsWith("Routed the question"))).toBe(
      false
    )
    expect(claim.reports.some((r) => r.task !== undefined)).toBe(false)
  })
})

describe("a harness create run", () => {
  test("refuses reads past its call limit without calling the server and stops after two stalled turns", async () => {
    const claim = new FakeClaim()
    claim.tools.specs = specs
    claim.brief = () => ({
      system: "",
      message: "",
      brief: structured("create", "harness keeps reading", false),
    })
    await runLoop(
      claim,
      claimed("harness keeps reading", { agent: { chat_calls: 1 } }),
      factory,
      new AbortController().signal,
      idle
    )
    expect(claim.tools.calls.filter((c) => c.name === "list_metrics")).toEqual(
      []
    )
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "no_progress",
    })
    expect(claim.finishes[0]?.usage.save_now_refusals).toBe(2)
    const steps = claim.reports.flatMap((r) => r.steps ?? [])
    expect(steps.map((s) => s.message)).toEqual([
      "Refused list_metrics: 1 model calls passed without a save",
      "Refused list_metrics: 1 model calls passed without a save",
      "Stopped: the model kept reading after 1 calls without a save",
    ])
    expect(steps.at(-1)).toMatchObject({ phase: "save", status: "error" })
  })
})
