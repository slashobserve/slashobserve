import { describe, expect, test } from "bun:test"
import {
  ToolError,
  briefAnswer,
  briefInput,
  claimAnswer,
  claimInput,
  finishInput,
  parseToolError,
  reportAnswer,
  reportInput,
  runMeta,
  turnMeta,
} from "../src/protocol"

const model = {
  provider: "replay",
  model: "replay",
  api_key: "",
  base_url: "",
  effort: "",
  max_tokens: 4096,
  request_timeout_seconds: 60,
  idle_timeout_seconds: 30,
  replay_dir: "/fixtures",
  log_provider_errors: false,
  session_renew_bytes: 262144,
}

const claimed = {
  run: {
    id: "9f1c2f5e-3a0e-4a8e-93a6-1c3d6a7f0a11",
    attempt: 1,
    kind: "observe",
    intent: "ask",
    prompt: "show checkout health",
    dashboard_id: null,
  },
  session: {
    url: "http://127.0.0.1:8080/mcp",
    token: "sot_x",
    expires_at: "2026-11-12T11:00:00Z",
  },
  lease_seconds: 60,
  system_prompt: { name: "observe", arguments: {} },
  retry_after_seconds: 0,
  model,
}

describe("protocol", () => {
  test("a claim answer with a field the server added later parses", () => {
    const parsed = claimAnswer.safeParse({
      ...claimed,
      added_next_year: { nested: true },
      run: { ...claimed.run, requested_by: { kind: "user", id: "1" } },
    })
    expect(parsed.success).toBe(true)
    expect(parsed.data?.run?.id).toBe(claimed.run.id)
    expect(parsed.data?.model?.replay_dir).toBe("/fixtures")
  })

  test("an empty claim has no run and no model", () => {
    const parsed = claimAnswer.parse({ run: null, retry_after_seconds: 3 })
    expect(parsed.run).toBeNull()
    expect(parsed.model ?? null).toBeNull()
    expect(parsed.retry_after_seconds).toBe(3)
  })

  test("model settings fill the fields the server leaves out", () => {
    const parsed = claimAnswer.parse({
      ...claimed,
      model: { provider: "replay" },
    })
    expect(parsed.model).toEqual({
      provider: "replay",
      model: "",
      api_key: "",
      base_url: "",
      effort: "",
      max_tokens: 16000,
      request_timeout_seconds: 600,
      idle_timeout_seconds: 180,
      replay_dir: "",
      log_provider_errors: false,
      session_renew_bytes: 0,
    })
  })

  const defaults: Array<[string, object, string, number]> = [
    [
      "anthropic gets medium effort",
      { provider: "anthropic" },
      "medium",
      16000,
    ],
    [
      "anthropic keeps a configured effort and limit",
      { provider: "anthropic", effort: "high", max_tokens: 4096 },
      "high",
      4096,
    ],
    ["openai gets no effort", { provider: "openai" }, "", 16000],
    [
      "openai keeps a configured effort",
      { provider: "openai", effort: "low", max_tokens: 2048 },
      "low",
      2048,
    ],
  ]
  test.each(defaults)(
    "%s when the claim leaves them out",
    (_name, input, effort, maxTokens) => {
      const parsed = claimAnswer.parse({ ...claimed, model: input })
      expect(parsed.model?.effort).toBe(effort)
      expect(parsed.model?.max_tokens).toBe(maxTokens)
    }
  )

  test("an empty effort and a zero limit sent by the server get the defaults", () => {
    const parsed = claimAnswer.parse({
      ...claimed,
      model: { provider: "anthropic", effort: "", max_tokens: 0 },
    })
    expect(parsed.model).toMatchObject({ effort: "medium", max_tokens: 16000 })
  })

  test("absent timeouts get Go's defaults, set ones are kept and zero means no limit", () => {
    const unset = claimAnswer.parse({
      ...claimed,
      model: { provider: "openai" },
    })
    expect(unset.model).toMatchObject({
      request_timeout_seconds: 600,
      idle_timeout_seconds: 180,
    })
    const set = claimAnswer.parse({
      ...claimed,
      model: {
        provider: "openai",
        request_timeout_seconds: 30,
        idle_timeout_seconds: 20,
      },
    })
    expect(set.model).toMatchObject({
      request_timeout_seconds: 30,
      idle_timeout_seconds: 20,
    })
    const none = claimAnswer.parse({
      ...claimed,
      model: {
        provider: "openai",
        request_timeout_seconds: 0,
        idle_timeout_seconds: 0,
      },
    })
    expect(none.model).toMatchObject({
      request_timeout_seconds: 0,
      idle_timeout_seconds: 0,
    })
  })

  test("a report answer from an older server has no build", () => {
    const parsed = reportAnswer.parse({
      state: "running",
      stop: false,
      lease_until: null,
    })
    expect(parsed.build ?? null).toBeNull()
  })

  test("a report answer keeps the lease end and the server's time", () => {
    const parsed = reportAnswer.parse({
      state: "running",
      stop: false,
      lease_until: "2026-11-27T10:01:00Z",
      server_time: "2026-11-27T10:00:00Z",
      build: null,
    })
    expect(parsed).toMatchObject({
      lease_until: "2026-11-27T10:01:00Z",
      server_time: "2026-11-27T10:00:00Z",
    })
  })

  test("inputs are strict", () => {
    expect(claimInput.safeParse({ wait_seconds: 30 }).success).toBe(true)
    expect(claimInput.safeParse({ wait_seconds: 31 }).success).toBe(false)
    expect(claimInput.safeParse({ wait_seconds: 1, extra: 1 }).success).toBe(
      false
    )
    expect(
      briefInput.safeParse({ run_id: "r", attempt: 1, reason: "pending" })
        .success
    ).toBe(true)
    expect(
      briefInput.safeParse({ run_id: "r", attempt: 1, reason: "fresh" }).success
    ).toBe(false)
    expect(
      reportInput.safeParse({ run_id: "r", attempt: 1, note: "x" }).success
    ).toBe(false)
    expect(
      finishInput.safeParse({
        run_id: "r",
        attempt: 1,
        outcome: "failed",
        failure_code: "agent_failed",
        usage: {},
      }).success
    ).toBe(false)
  })

  test("the two meta keys read what the server writes", () => {
    expect(turnMeta.safeParse({ id: "t1", calls: 2, saves: 1 }).success).toBe(
      true
    )
    expect(turnMeta.safeParse({ id: "t1", calls: 0, saves: 0 }).success).toBe(
      false
    )
    expect(
      runMeta.parse({
        settled: "saved",
        build: { saves: 1, panels: 2, planned_panels: 3 },
      }).build?.planned_panels
    ).toBe(3)
    expect(runMeta.parse({ settled: "proposed", build: null }).settled).toBe(
      "proposed"
    )
    expect(runMeta.parse({ settled: null, build: null }).settled).toBeNull()
    expect(runMeta.safeParse({ settled: "done", build: null }).success).toBe(
      false
    )
  })

  test("a claim reads the agent settings the server sends its own agent", () => {
    const parsed = claimAnswer.parse({
      ...claimed,
      model,
      agent: {
        investigation_calls: 0,
        investigation_calls_max: 40,
        default_calls: 30,
        task_calls: { measure: 16, incident: 30 },
        chat_calls: 12,
        save_calls: 6,
        suggest_calls: 12,
        loop: true,
        grounding: "enforce",
        brief_sweep: true,
        session_renew_bytes: 0,
      },
    })
    expect(parsed.agent?.task_calls.measure).toBe(16)
    expect(parsed.agent?.grounding).toBe("enforce")
    expect(claimAnswer.parse({ ...claimed, model }).agent).toBeUndefined()
    expect(
      claimAnswer.parse({ ...claimed, model, agent: { loop: "yes" } }).agent
    ).toBeNull()
  })

  test("a brief reads the structured brief and keeps working without it", () => {
    const brief = {
      request: {
        mode: "revise",
        source: "prompt",
        prompt: "add a panel",
        panel_id: null,
        from: null,
        to: null,
        time_zone: null,
        as_of: null,
        continue_note: null,
        thread_context: null,
        continues: false,
        attempt: 1,
        investigation: false,
        pointer: {
          anchor: null,
          block_id: null,
          observation_id: null,
          thread_ask: false,
        },
      },
      current: {
        dashboard_id: "d1",
        number: 2,
        spec: { title: "t" },
        unverified: { panels: null, variables: null },
        conversion: null,
      },
      build: null,
      datasources: [
        {
          uid: "prom",
          type: "prometheus",
          name: "Prometheus",
          source: "file",
          healthy: true,
          checked_at: "2026-03-01T09:00:00Z",
          health_state: "healthy",
          everyone: true,
        },
      ],
      withheld: false,
      guide: { text: "guide", schema: { type: "object" } },
      draft: null,
    }
    const parsed = briefAnswer.parse({
      system: "s",
      message: "m",
      reason: "",
      build: null,
      brief,
    })
    expect(parsed.brief?.current?.number).toBe(2)
    expect(parsed.brief?.datasources[0]?.everyone).toBe(true)
    expect(
      briefAnswer.parse({ system: "s", message: "m", brief: { request: 1 } })
        .brief
    ).toBeNull()
  })

  test("the run key reads the call side fields when the server sends them", () => {
    const meta = runMeta.parse({
      settled: null,
      build: null,
      call: {
        edits: { panels: ["a"], added: ["a"], deleted: [], settings: false },
        refusal: "one_panel",
        unchecked: null,
        discovered: { datasource: "prom", of: "metrics", names: ["up"] },
        ranges: [{ from: "2026-03-01T09:00:00Z", to: "2026-03-01T10:00:00Z" }],
      },
    })
    expect(meta.call?.edits?.panels).toEqual(["a"])
    expect(meta.call?.refusal).toBe("one_panel")
    expect(meta.call?.ranges).toHaveLength(1)
    expect(runMeta.parse({ settled: null, build: null }).call).toBeUndefined()
  })

  test("a tool error body becomes a ToolError with its code", () => {
    const error = parseToolError(
      JSON.stringify({ code: "not_your_run", message: "that run is not yours" })
    )
    expect(error).toBeInstanceOf(ToolError)
    expect(error.code).toBe("not_your_run")
    expect(error.message).toBe("that run is not yours")
  })

  test("a tool error body that is not JSON keeps its text under a generic code", () => {
    for (const text of ["boom", '{"message":"no code"}', "[1]", ""]) {
      const error = parseToolError(text)
      expect(error.code).toBe("tool_error")
      expect(error.message).toBe(text.trim())
    }
  })
})
