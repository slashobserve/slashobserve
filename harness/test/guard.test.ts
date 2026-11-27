import { describe, expect, test } from "bun:test"
import { existsSync, readdirSync, readFileSync } from "node:fs"
import { join, resolve } from "node:path"
import {
  type Admission,
  type Call,
  type CallResult,
  Guard,
  type GuardState,
  type Hypothesis,
  type Mode,
  type Stop,
  toolPhase,
} from "../src/guard"
import {
  countClaim,
  doubtsTitleCause,
  findingOf,
  readsData,
} from "../src/guard/answer"
import { callKey } from "../src/guard/key"
import { provedSomething } from "../src/guard/loop"
import * as text from "../src/guard/text"
import type { AgentSettings } from "../src/protocol"

const fixtures = resolve(
  import.meta.dir,
  "..",
  "..",
  "spec",
  "examples",
  "agent",
  "guard"
)
const traceFiles = [
  "admission",
  "windows",
  "rejections",
  "writes",
  "answers",
  "grounding",
  "ledger",
  "loop",
  "churn",
]

type TraceCall = {
  name: string
  input?: unknown
  raw?: string
  ungrounded?: string
  unclosed?: string
}
type TraceResult = {
  is_error?: boolean
  content?: unknown
  settled?: CallResult["settled"]
  call?: {
    edits?: NonNullable<CallResult["call"]>["edits"]
    refusal?: string
    unchecked?: string
  }
}
type TraceAdmission = {
  verdict?: string
  phase?: string
  step?: string
  code?: string
  message?: string
  message_has?: string | string[]
  note?: string
  reads_refused?: number
  unchecked?: unknown[]
  reuse_of?: number
}
type TraceStop = Stop | null
type TraceStep = {
  turn?: { saves: number }
  call?: TraceCall
  expect?: TraceAdmission
  result?: TraceResult
  settled?: { note?: string; stop?: TraceStop }
  settle?: number
  end_read?: number
  end_turn?: {
    nudge?: string
    stage_line?: string
    stage_step?: string
    stop?: TraceStop
  }
  check?: {
    limit?: number
    stage?: string
    writes?: number
    phase?: Record<string, string>
    streak?: unknown
    usage?: Record<string, unknown>
  }
  set?: { window_calls?: number; saved?: boolean }
  session?: { building: boolean }
  restart?: object
  seed_ledger?: Hypothesis[]
  forget?: TraceCall
}
type Trace = {
  name: string
  ts_only?: string
  go_only?: string
  start: {
    mode: Mode
    agent?: Partial<AgentSettings>
    task?: string
    investigation?: boolean
    building?: boolean
    window_calls?: number
    streak?: GuardState["streak"]
  }
  steps: TraceStep[]
}

type Cases = {
  write_tools: string[]
  keys: Array<{
    name: string
    tool: string
    other_tool?: string
    a: string
    b: string
    same: boolean
  }>
  count_claims: { match: string[]; no_match: string[] }
  title_causes: Array<{
    name: string
    title: string
    text: string
    want: boolean
  }>
  conclusions: Array<{ input: unknown; want: boolean }>
  proved: Array<{
    name: string
    prev: Hypothesis[]
    next: Hypothesis[]
    want: boolean
  }>
  people_text: Array<{ reasons: string[]; want: string }>
  data_reads: { yes: string[]; no: string[] }
}

function read<T>(name: string): T {
  return JSON.parse(readFileSync(join(fixtures, name), "utf8")) as T
}

const cases = read<Cases>("cases.json")
const writeTools = new Set(cases.write_tools)
const phaseOf = (name: string) => toolPhase(name, !writeTools.has(name))

const serverAgent: AgentSettings = {
  investigation_calls: 0,
  investigation_calls_max: 0,
  default_calls: 30,
  task_calls: {
    measure: 16,
    logs: 20,
    traces: 20,
    incident: 30,
    inventory: 12,
    explain: 12,
  },
  chat_calls: 12,
  save_calls: 6,
  suggest_calls: 12,
  loop: false,
  grounding: "off",
  brief_sweep: false,
  session_renew_bytes: 0,
}

function callOf(c: TraceCall): Call {
  const call: Call = {
    name: c.name,
    input: c.raw ?? JSON.stringify(c.input ?? {}),
  }
  if (c.ungrounded !== undefined) call.ungrounded = c.ungrounded
  if (c.unclosed !== undefined) call.unclosed = c.unclosed
  return call
}

function resultOf(r: TraceResult): CallResult {
  return {
    isError: r.is_error ?? false,
    content: JSON.stringify(r.content ?? { ok: true }),
    settled: r.settled ?? null,
    call: {
      edits: r.call?.edits ?? null,
      refusal: r.call?.refusal ?? null,
      unchecked: r.call?.unchecked ?? null,
      discovered: null,
      ranges: [],
    },
  }
}

function checkAdmission(at: string, adm: Admission, want: TraceAdmission) {
  const body = (adm.content === undefined ? {} : JSON.parse(adm.content)) as {
    code?: string
    message?: string
  }
  const got: Record<string, unknown> = {
    verdict: adm.verdict,
    phase: adm.phase,
    step: adm.step ?? "",
    code: body.code ?? "",
    message: body.message ?? "",
    note: adm.note ?? "",
    reads_refused: adm.readsRefused ?? 0,
    unchecked: adm.unchecked ?? [],
    reuse_of: adm.reuseOf ?? 0,
  }
  for (const [field, value] of Object.entries(want)) {
    if (field === "message_has") {
      for (const part of [value as string | string[]].flat()) {
        expect(body.message ?? "", `${at} message`).toContain(part)
      }
      continue
    }
    expect(got[field], `${at} ${field}`).toEqual(value)
  }
}

function checkStop(
  at: string,
  got: Stop | undefined,
  want: TraceStop | undefined
) {
  if (want === undefined) {
    return
  }
  expect(got ?? null, `${at} stop`).toEqual(want)
}

function startOf(trace: Trace): Guard {
  const start = trace.start
  const guard = Guard.create(
    {
      mode: start.mode,
      agent: { ...serverAgent, ...start.agent },
      ...(start.task === undefined ? {} : { task: start.task }),
      investigation: start.investigation ?? true,
      building: start.building ?? false,
    },
    phaseOf
  )
  guard.state.window.calls = start.window_calls ?? 0
  guard.state.streak = start.streak ?? null
  return guard
}

function runTrace(trace: Trace, reload: boolean) {
  let guard = startOf(trace)
  let calls: Call[] = []
  let admissions: Admission[] = []
  const settle = (
    at: string,
    position: number,
    result: TraceResult,
    want: TraceStep["settled"]
  ) => {
    const settled = guard.settle(
      calls[position] as Call,
      admissions[position] as Admission,
      resultOf(result)
    )
    if (want?.note !== undefined) {
      expect(settled.note ?? "", `${at} note`).toBe(want.note)
    }
    checkStop(at, settled.stop, want?.stop)
  }
  trace.steps.forEach((step, i) => {
    const at = `${trace.name}, step ${i}`
    if (reload) {
      guard = new Guard(
        JSON.parse(JSON.stringify(guard.state)) as GuardState,
        phaseOf
      )
    }
    if (step.turn !== undefined) {
      guard.beginTurn({ saves: step.turn.saves })
      calls = []
      admissions = []
    } else if (step.call !== undefined) {
      const call = callOf(step.call)
      const adm = guard.admit(call)
      calls.push(call)
      admissions.push(adm)
      if (step.expect !== undefined) {
        checkAdmission(at, adm, step.expect)
      }
      if (step.result !== undefined) {
        settle(at, calls.length - 1, step.result, step.settled)
      }
    } else if (step.settle !== undefined) {
      settle(at, step.settle, step.result ?? {}, step.settled)
    } else if (step.end_read !== undefined) {
      guard.endRead(calls[step.end_read] as Call)
    } else if (step.end_turn !== undefined) {
      const end = guard.endTurn()
      const want = step.end_turn
      if (want.nudge !== undefined)
        expect(end.nudge, `${at} nudge`).toBe(want.nudge)
      if (want.stage_line !== undefined)
        expect(end.stageLine, `${at} stage line`).toBe(want.stage_line)
      if (want.stage_step !== undefined)
        expect(end.stageStep, `${at} stage step`).toBe(want.stage_step)
      expect(end.notes).toEqual(
        [end.nudge, end.stageLine].filter((n) => n !== "")
      )
      checkStop(at, end.stop, want.stop)
    } else if (step.check !== undefined) {
      const want = step.check
      if (want.limit !== undefined)
        expect(guard.limit(), `${at} limit`).toBe(want.limit)
      if (want.stage !== undefined)
        expect(guard.stage() as string, `${at} stage`).toBe(want.stage)
      if (want.writes !== undefined)
        expect(guard.state.writes, `${at} writes`).toBe(want.writes)
      for (const [name, phase] of Object.entries(want.phase ?? {})) {
        expect(guard.phase(name) as string, `${at} phase of ${name}`).toBe(
          phase
        )
      }
      if (want.streak !== undefined) {
        expect(guard.state.streak as unknown, `${at} streak`).toEqual(
          want.streak
        )
      }
      const usage = guard.usage() as Record<string, unknown>
      for (const [field, value] of Object.entries(want.usage ?? {})) {
        expect(usage[field] ?? null, `${at} usage ${field}`).toEqual(
          value as object
        )
      }
    } else if (step.set !== undefined) {
      if (step.set.window_calls !== undefined) {
        guard.state.window.calls = step.set.window_calls
      }
      if (step.set.saved !== undefined) {
        guard.state.saved = step.set.saved
      }
    } else if (step.session !== undefined) {
      guard.beginSession(step.session.building)
    } else if (step.restart !== undefined) {
      guard.restart()
    } else if (step.seed_ledger !== undefined) {
      guard.seedLedger(step.seed_ledger)
    } else if (step.forget !== undefined) {
      guard.forget(callOf(step.forget))
    } else {
      throw new Error(`${at}: no known operation`)
    }
  })
}

for (const file of traceFiles) {
  const { traces } = read<{ traces: Trace[] }>(`${file}.json`)
  describe(`guard traces: ${file}`, () => {
    for (const trace of traces) {
      if (trace.go_only !== undefined) {
        continue
      }
      test(trace.name, () => runTrace(trace, false))
      test(`${trace.name}, reloaded before every step`, () =>
        runTrace(trace, true))
    }
  })
}

describe("guard cases", () => {
  test("keys", () => {
    for (const c of cases.keys) {
      const same = callKey(c.tool, c.a) === callKey(c.other_tool ?? c.tool, c.b)
      expect(same, c.name).toBe(c.same)
    }
  })

  test("count claims", () => {
    for (const text of cases.count_claims.match) {
      expect(countClaim.test(text), text).toBe(true)
    }
    for (const text of cases.count_claims.no_match) {
      expect(countClaim.test(text), text).toBe(false)
    }
  })

  test("title causes", () => {
    for (const c of cases.title_causes) {
      expect(doubtsTitleCause(c.title, c.text), c.name).toBe(c.want)
    }
  })

  test("conclusions", () => {
    for (const c of cases.conclusions) {
      expect(findingOf(JSON.stringify(c.input)).conclusion).toBe(c.want)
    }
  })

  test("proved", () => {
    for (const c of cases.proved) {
      expect(provedSomething(c.prev, c.next), c.name).toBe(c.want)
    }
  })

  test("people text", () => {
    for (const c of cases.people_text) {
      expect(text.peopleText(c.reasons)).toBe(c.want)
    }
  })

  test("data reads", () => {
    for (const name of cases.data_reads.yes) {
      expect(readsData(name, toolPhase(name, true)), name).toBe(true)
    }
    for (const name of cases.data_reads.no) {
      expect(readsData(name, toolPhase(name, true)), name).toBe(false)
    }
  })
})

describe("guard state", () => {
  test("holds no tool input or result", () => {
    const guard = Guard.create(
      {
        mode: "create",
        agent: serverAgent,
        investigation: false,
        building: false,
      },
      phaseOf
    )
    guard.beginTurn({ saves: 0 })
    const call = { name: "get_panels", input: '{"secret":"input-text"}' }
    const adm = guard.admit(call)
    guard.settle(call, adm, {
      isError: false,
      content: '{"secret":"result-text"}',
      settled: null,
    })
    const text = JSON.stringify(guard.snapshot())
    expect(text).not.toContain("input-text")
    expect(text).not.toContain("result-text")
  })

  test("takes the call limits from the claim's agent settings", () => {
    const limit = (mode: Mode, agent: Partial<AgentSettings>, task?: string) =>
      Guard.create(
        {
          mode,
          agent: { ...serverAgent, ...agent },
          ...(task === undefined ? {} : { task }),
          investigation: true,
          building: false,
        },
        phaseOf
      ).limit()
    expect(limit("investigate", {}, "measure")).toBe(16)
    expect(limit("investigate", {}, "full")).toBe(30)
    expect(limit("investigate", { investigation_calls: 24 }, "measure")).toBe(
      24
    )
    expect(limit("investigate", { default_calls: 0, task_calls: {} })).toBe(30)
    expect(limit("create", { chat_calls: 14 })).toBe(14)
    expect(limit("revise", { save_calls: 7 })).toBe(7)
    expect(limit("suggest", { suggest_calls: 9 })).toBe(9)
  })
})

const guardSource = resolve(
  import.meta.dir,
  "..",
  "..",
  "internal",
  "agent",
  "guard"
)

describe("guard texts", () => {
  test.skipIf(!existsSync(guardSource))(
    "are the Go guard's own strings",
    () => {
      const source = readdirSync(guardSource)
        .filter((file) => file.endsWith(".go") && !file.endsWith("_test.go"))
        .map((file) => readFileSync(join(guardSource, file), "utf8"))
        .join("\n")
      const fixed = [
        "repeated_save",
        "read_in_flight",
        "repeated_write",
      ] as const
      const texts = [
        ...Object.entries(text)
          .filter(
            ([name, value]) =>
              typeof value === "string" &&
              /^(message|fault|people|step|sentBack)/.test(name)
          )
          .map(([, value]) => value as string),
        ...fixed.flatMap((name) => Object.values(text.ruleOf(name, 0, false))),
      ]
      expect(texts.length).toBeGreaterThan(20)
      for (const value of texts) {
        expect(source, value).toContain(JSON.stringify(value))
      }
    }
  )
})
