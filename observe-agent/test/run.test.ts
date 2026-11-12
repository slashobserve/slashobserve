import { describe, expect, test } from "bun:test"
import { configureLog } from "../src/log"
import { runLoop } from "../src/run"
import { estimateOutputTokens } from "../src/session/replay/replay"
import {
  FakeClaim,
  claimed,
  factory,
  meta,
  notYourRun,
  progress,
  reply,
} from "./harness"

configureLog("error")

const idle = { heartbeatMs: 60_000, textFlushMs: 60_000 }

async function drive(
  claim: FakeClaim,
  run: ReturnType<typeof claimed>,
  options: Parameters<typeof runLoop>[4] = idle,
  signal: AbortSignal = new AbortController().signal
): Promise<void> {
  await runLoop(claim, run, factory, signal, options)
}

function savedWith(build = progress(1)) {
  return async () => reply({ meta: meta("saved", build) })
}

describe("run loop success", () => {
  test("a saved version finishes the run, reads first, one turn id", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async (call) =>
      call.name === "create_dashboard"
        ? reply({ meta: meta("saved", progress(1)) })
        : reply()
    await drive(claim, claimed("create happy path"))
    expect(claim.tools.calls.map((c) => c.name)).toEqual([
      "list_datasources",
      "list_metrics",
      "create_dashboard",
    ])
    const ids = new Set(claim.tools.calls.map((c) => c.turn?.id))
    expect(ids.size).toBe(1)
    expect([...ids][0]).toMatch(/^[0-9a-f-]{36}$/)
    for (const call of claim.tools.calls) {
      expect(call.turn?.calls).toBe(3)
      expect(call.turn?.saves).toBe(1)
    }
    expect(claim.tools.calls[1]?.args).toEqual({ datasource: "prom" })
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "succeeded",
      run_id: "11111111-1111-4111-8111-111111111111",
      attempt: 1,
    })
    expect(claim.finishes[0]?.commit).toBeUndefined()
    expect(claim.tools.closed).toBe(true)
  })

  test("the usage on finish_run carries the counters", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    await drive(claim, claimed("create happy path"))
    const inputs = ["{}", '{ "datasource": "prom" }', '{ "title": "t" }']
    expect(claim.finishes[0]?.usage).toMatchObject({
      provider: "replay",
      model: "replay",
      input_tokens: 0,
      output_tokens: estimateOutputTokens(inputs),
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      model_calls: 1,
      retries: 0,
      compactions: 0,
      renewals: 0,
      max_save_output_tokens: estimateOutputTokens(inputs),
    })
    expect(claim.finishes[0]?.usage.model_ms).toBeGreaterThanOrEqual(0)
    expect(claim.finishes[0]?.usage).not.toHaveProperty("cost_usd")
  })

  test("a finding finishes an investigation", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async (call) =>
      call.name === "save_finding" ? reply({ meta: meta("finding") }) : reply()
    await drive(
      claim,
      claimed("investigate the spike", { kind: "investigate" })
    )
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]?.outcome).toBe("succeeded")
    expect(
      claim.tools.calls.find((c) => c.name === "save_finding")?.turn
    ).toMatchObject({
      calls: 1,
      saves: 1,
    })
  })

  test("the first brief has no reason and starts the session from it", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    await drive(claim, claimed("create happy path"))
    expect(claim.briefs).toEqual([
      { run_id: "11111111-1111-4111-8111-111111111111", attempt: 1 },
    ])
  })

  test("report_run carries the cumulative usage before every model call", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    await drive(claim, claimed("create happy path"))
    expect(claim.reports.filter((r) => r.usage !== undefined)).toHaveLength(1)
    expect(claim.reports[0]?.usage).toMatchObject({
      model_calls: 0,
      provider: "replay",
    })
  })
})

describe("cut-offs", () => {
  test("two max_tokens in a row end output_limit", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("max tokens twice"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([
      undefined,
      "output_limit",
    ])
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "output_limit",
    })
    expect(claim.finishes[0]?.usage).toMatchObject({
      model_calls: 2,
      compactions: 1,
      renewals: 0,
    })
    expect(claim.finishes[0]?.message).toBeUndefined()
  })

  test("a suggest run stops at the first max_tokens", async () => {
    const claim = new FakeClaim()
    await drive(
      claim,
      claimed("max tokens twice", { source: "suggest", dashboard_id: "d" })
    )
    expect(claim.briefs).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "output_limit",
    })
  })

  test("one call timeout compacts with call_timeout and the run goes on", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async (call) =>
      call.name === "create_dashboard"
        ? reply({ meta: meta("saved", progress(1)) })
        : reply()
    await drive(claim, claimed("timeout once"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([
      undefined,
      "call_timeout",
    ])
    expect(claim.finishes[0]).toMatchObject({ outcome: "succeeded" })
    expect(claim.finishes[0]?.usage).toMatchObject({
      model_calls: 3,
      compactions: 1,
      renewals: 0,
    })
  })

  test("two call timeouts in a row end call_timeout", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("timeout twice"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([
      undefined,
      "call_timeout",
    ])
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "call_timeout",
    })
  })

  test("a build save in between resets the cut-offs", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async (call) =>
      call.name === "create_dashboard"
        ? reply({ meta: meta("saved", progress(2)) })
        : reply({ meta: meta(null, progress(1)) })
    await drive(claim, claimed("timeout twice"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([
      undefined,
      "call_timeout",
      "call_timeout",
    ])
    expect(claim.finishes[0]).toMatchObject({ outcome: "succeeded" })
  })
})

describe("renewal", () => {
  test("compacts with renewed after a build save past the byte threshold", async () => {
    const claim = new FakeClaim()
    let calls = 0
    claim.tools.handler = async () => {
      calls++
      return calls === 1
        ? reply({
            meta: meta(null, progress(1, 1, 2)),
            content: "x".repeat(40),
          })
        : reply({ meta: meta("saved", progress(2, 2, 2)) })
    }
    await drive(claim, claimed("renew the session", { renew: 50 }))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "renewed"])
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]?.usage).toMatchObject({
      compactions: 1,
      renewals: 1,
      model_calls: 2,
    })
  })

  test("does not compact while the session is under the threshold", async () => {
    const claim = new FakeClaim()
    let calls = 0
    claim.tools.handler = async () => {
      calls++
      return calls === 1
        ? reply({ meta: meta(null, progress(1, 1, 2)) })
        : reply({ meta: meta("saved", progress(2, 2, 2)) })
    }
    await drive(claim, claimed("renew the session", { renew: 100_000 }))
    expect(claim.briefs).toHaveLength(1)
    expect(claim.finishes[0]?.usage).toMatchObject({
      compactions: 0,
      renewals: 0,
    })
  })

  test("a turn without a new build save never renews", async () => {
    const claim = new FakeClaim()
    let calls = 0
    claim.tools.handler = async () => {
      calls++
      return calls === 1
        ? reply({
            meta: meta(null, progress(0, 0, 2)),
            content: "x".repeat(400),
          })
        : reply({ meta: meta("saved", progress(1, 1, 1)) })
    }
    await drive(claim, claimed("renew the session", { renew: 50 }))
    expect(claim.briefs).toHaveLength(1)
  })
})

describe("seeds when the model ends without a call", () => {
  test("an investigation seeds no_finding once then fails no_finding", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("model says nothing", { kind: "investigate" }))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "no_finding"])
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "no_finding",
    })
  })

  test("a create run without a build seeds no_answer once then fails no_tool_call", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("model says nothing"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "no_answer"])
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "no_tool_call",
    })
  })

  test("a revise run behaves like a create run", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("model says nothing", { dashboard_id: "d1" }))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "no_answer"])
    expect(claim.finishes[0]?.failure_code).toBe("no_tool_call")
  })

  test("a suggest run fails no_tool_call at once", async () => {
    const claim = new FakeClaim()
    await drive(claim, claimed("suggest a panel", { source: "suggest" }))
    expect(claim.briefs).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "no_tool_call",
    })
  })

  test("pending panels seed pending once then the build is committed", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async () =>
      reply({ meta: meta(null, progress(1, 1, 3)) })
    claim.brief = (input) =>
      input.reason === undefined ? {} : { build: progress(1, 1, 3) }
    await drive(claim, claimed("panels still pending"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "pending"])
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "succeeded",
      commit: true,
    })
  })

  test("a finished build is committed without a seed", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async () =>
      reply({ meta: meta(null, progress(1, 3, 3)) })
    await drive(claim, claimed("panels still pending"))
    expect(claim.briefs).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "succeeded",
      commit: true,
    })
  })

  test("the build from the brief counts as a build", async () => {
    const claim = new FakeClaim()
    claim.brief = () => ({ build: progress(1, 2, 2) })
    await drive(claim, claimed("commit hits a conflict"))
    expect(claim.briefs).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "succeeded",
      commit: true,
    })
  })
})

describe("restarts", () => {
  test("a restarted commit opens a fresh session with reason conflict", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async () => reply({ meta: meta(null, progress(1)) })
    claim.brief = (input) =>
      input.reason === "conflict" ? { build: progress(1) } : {}
    claim.finish = (_input, nth) => ({ restarted: nth === 0 })
    await drive(claim, claimed("commit hits a conflict"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "conflict"])
    expect(claim.finishes).toHaveLength(2)
    expect(claim.finishes.map((f) => f.commit)).toEqual([true, true])
    expect(claim.finishes[1]?.outcome).toBe("succeeded")
  })

  test("a restarted tool result opens a fresh session and resets the cut-offs", async () => {
    const claim = new FakeClaim()
    let calls = 0
    claim.tools.handler = async () => {
      calls++
      return calls === 1
        ? reply({ meta: meta("restarted") })
        : reply({ meta: meta("saved", progress(1)) })
    }
    await drive(claim, claimed("a save restarts the run"))
    expect(claim.briefs.map((b) => b.reason)).toEqual([undefined, "conflict"])
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]?.outcome).toBe("succeeded")
    expect(claim.finishes[0]?.usage.model_calls).toBe(2)
  })
})

describe("model failures", () => {
  const rows = [
    { prompt: "rate limited", code: "rate_limited" },
    { prompt: "model refuses", code: "refused" },
    { prompt: "no fixture for this prompt", code: "no_fixture" },
  ] as const
  for (const row of rows) {
    test(`${row.prompt} fails ${row.code} without a message`, async () => {
      const claim = new FakeClaim()
      await drive(claim, claimed(row.prompt))
      expect(claim.finishes).toHaveLength(1)
      expect(claim.finishes[0]).toMatchObject({
        outcome: "failed",
        failure_code: row.code,
      })
      expect(claim.finishes[0]?.message).toBeUndefined()
    })
  }

  test("a provider without a session fails internal", async () => {
    const claim = new FakeClaim()
    const run = claimed("create happy path")
    run.model.provider = "gemini"
    await drive(claim, run)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "internal",
    })
  })

  test("a replay mismatch fails provider_error", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async () =>
      reply({
        isError: true,
        content: '{"code":"query_invalid","message":"m"}',
      })
    await drive(claim, claimed("create happy path"))
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      failure_code: "provider_error",
    })
  })
})

describe("lease and live text", () => {
  test("a heartbeat reports without usage or text while a model call runs", async () => {
    const claim = new FakeClaim()
    claim.report = (_input, nth) =>
      nth >= 3 ? { stop: true, state: "canceling" } : {}
    await drive(claim, claimed("slow model"), {
      heartbeatMs: 20,
      textFlushMs: 60_000,
    })
    const beats = claim.reports.filter((r) => r.usage === undefined)
    expect(beats.length).toBeGreaterThanOrEqual(2)
    for (const beat of beats) {
      expect(beat.text).toBeUndefined()
    }
  })

  test("canceling aborts the model call and settles the run with finish_run", async () => {
    const claim = new FakeClaim()
    claim.report = (_input, nth) =>
      nth >= 1 ? { stop: true, state: "canceling" } : {}
    const started = performance.now()
    await drive(claim, claimed("slow model"), {
      heartbeatMs: 20,
      textFlushMs: 60_000,
    })
    expect(performance.now() - started).toBeLessThan(2000)
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({
      outcome: "failed",
      run_id: "11111111-1111-4111-8111-111111111111",
      attempt: 1,
    })
    expect(claim.finishes[0]?.failure_code).toBeUndefined()
    expect(claim.tools.closed).toBe(true)
  })

  test("canceling on the first report settles the run before any model call", async () => {
    const claim = new FakeClaim()
    claim.report = () => ({ stop: true, state: "canceling" })
    await drive(claim, claimed("create happy path"))
    expect(claim.tools.calls).toHaveLength(0)
    expect(claim.finishes).toHaveLength(1)
    expect(claim.finishes[0]).toMatchObject({ outcome: "failed" })
  })

  test("a failing finish_run after canceling does not fail the loop", async () => {
    const claim = new FakeClaim()
    claim.report = () => ({ stop: true, state: "canceling" })
    claim.finishRun = async () => {
      throw notYourRun()
    }
    await drive(claim, claimed("create happy path"))
    expect(claim.tools.closed).toBe(true)
  })

  test.each(["failed", "succeeded", "canceled"])(
    "a %s run on the first report ends without finish_run",
    async (state) => {
      const claim = new FakeClaim()
      claim.report = () => ({ stop: true, state })
      await drive(claim, claimed("create happy path"))
      expect(claim.tools.calls).toHaveLength(0)
      expect(claim.finishes).toHaveLength(0)
    }
  )

  test("not_your_run from report_run ends quietly", async () => {
    const claim = new FakeClaim()
    claim.reportError = () => notYourRun()
    await drive(claim, claimed("create happy path"))
    expect(claim.finishes).toHaveLength(0)
    expect(claim.tools.calls).toHaveLength(0)
  })

  test("the turn's text is reported after the flush delay or with the next report", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    await drive(claim, claimed("create happy path"), {
      heartbeatMs: 60_000,
      textFlushMs: 5,
    })
    const texts = claim.reports
      .map((r) => r.text)
      .filter((t) => t !== undefined)
    expect(texts).toEqual(["building"])
  })

  test("text still buffered when the run finishes is reported before finish_run", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    await drive(claim, claimed("create happy path"))
    expect(claim.reports.map((r) => r.text)).toEqual([undefined, "building"])
  })

  test("a settled ended result ends quietly", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = async () => reply({ meta: meta("ended") })
    await drive(claim, claimed("create happy path"))
    expect(claim.finishes).toHaveLength(0)
  })

  test("a not_your_run answer to finish_run ends quietly", async () => {
    const claim = new FakeClaim()
    claim.tools.handler = savedWith()
    claim.finishRun = async () => {
      throw notYourRun()
    }
    await drive(claim, claimed("create happy path"))
  })

  test("an aborted signal ends the loop without finish_run", async () => {
    const claim = new FakeClaim()
    const controller = new AbortController()
    setTimeout(() => controller.abort(new Error("shutdown")), 30)
    await drive(claim, claimed("slow model"), idle, controller.signal)
    expect(claim.finishes).toHaveLength(0)
    expect(claim.tools.closed).toBe(true)
  })
})
