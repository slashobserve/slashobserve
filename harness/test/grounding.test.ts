import { describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { formatFloat } from "../src/brief/gofmt"
import {
  causeSignals,
  scoredCauses,
  type Signals,
} from "../src/grounding/causes"
import { issueFacts } from "../src/grounding/facts"
import { blank, scan } from "../src/grounding/figures"
import {
  checkSave,
  noteResult,
  savedFinding,
  startGrounding,
} from "../src/grounding/grounding"
import { parseTask, type Task } from "../src/route/route"
import { uncitedFigures } from "../src/grounding/needfact"
import { sendBackProblems } from "../src/grounding/problems"
import {
  addRead,
  addSourced,
  type Fact,
  type GroundingRecord,
  maxFacts,
  maxSourced,
  maxSourcedOfRead,
  maxValues,
  maxValuesOfRead,
  newRecord,
  type Range,
  rounded,
  sourceNames,
} from "../src/grounding/record"
import { Ledger } from "../src/grounding/render"
import {
  answerProblems,
  conflicts,
  ledgerOf,
  missingAnswers,
  review,
  sawName,
  unstated,
} from "../src/grounding/review"
import { parseTime } from "../src/grounding/time"
import { traceIDProblems } from "../src/grounding/traceids"

const fixtures = resolve(
  import.meta.dir,
  "..",
  "..",
  "spec",
  "examples",
  "agent",
  "grounding"
)

export function fixture<T>(name: string): T {
  return JSON.parse(readFileSync(resolve(fixtures, name), "utf8")) as T
}

type Problem = { kind: string; text: string; fix: string }

type FigureFixture = {
  uncited: { name: string; text: string; question: string; want: string[] }[]
  steps: { text: string; step: number }[]
  rounded: { in: number; want: number }[]
  trace_ids: { known: string[]; cases: { text: string; want: Problem[] }[] }
  source_names: { name: string; text: string; want: string[] }[]
}

describe("figures", () => {
  const fx = fixture<FigureFixture>("figures.json")

  test("takes the figures Go takes", () => {
    for (const c of fx.uncited) {
      expect({
        name: c.name,
        got: uncitedFigures(c.text, c.question).map((f) => f.text),
      }).toEqual({
        name: c.name,
        got: c.want,
      })
    }
  })

  test("reads the written precision as Go does", () => {
    for (const c of fx.steps) {
      const got = scan(blank(c.text))
      expect(got.length).toBe(1)
      expect(got[0]?.step).toBe(c.step)
    }
  })

  test("rounds to three significant figures as Go does", () => {
    for (const c of fx.rounded) {
      expect(rounded(c.in)).toBe(c.want)
    }
  })

  test("finds cut and unread trace ids as Go does", () => {
    for (const c of fx.trace_ids.cases) {
      expect(traceIDProblems(c.text, fx.trace_ids.known)).toEqual(c.want)
    }
  })

  test("takes the names of MCP source text as Go does", () => {
    for (const c of fx.source_names) {
      expect({ name: c.name, got: sourceNames(c.text) }).toEqual({
        name: c.name,
        got: c.want,
      })
    }
  })
})

type RenderWant = { text: string; problems: Problem[] }
type AnswerCase = {
  name?: string
  answers?: { quantity: string; fact: string }[]
  text?: string
  want: Problem[]
}

type RenderFixture = {
  ledger: Fact[]
  render: { text: string; want: RenderWant }[]
  answers: {
    ledger: Fact[]
    answer_problems: AnswerCase[]
    conflicts: AnswerCase[]
    unstated: AnswerCase[]
    unstated_answers: { quantity: string; fact: string }[]
    send_back: { quantity: string; fact: string }[]
    send_back_want: string
  }
  missing_answers: {
    question: string
    cases: { input: unknown; want: Problem | null }[]
  }
}

describe("render", () => {
  const fx = fixture<RenderFixture>("render.json")

  test("renders each placeholder as Go does", () => {
    const ledger = new Ledger(fx.ledger)
    for (const c of fx.render) {
      expect({ text: c.text, got: ledger.render(c.text) }).toEqual({
        text: c.text,
        got: c.want,
      })
    }
  })

  test("checks the answers list as Go does", () => {
    const ledger = new Ledger(fx.answers.ledger)
    for (const c of fx.answers.answer_problems) {
      expect(answerProblems(ledger, c.answers ?? [])).toEqual(c.want)
    }
    for (const c of fx.answers.conflicts) {
      expect(conflicts(ledger, c.text ?? "")).toEqual(c.want)
    }
    for (const c of fx.answers.unstated) {
      expect(
        unstated(ledger, fx.answers.unstated_answers, c.text ?? "")
      ).toEqual(c.want)
    }
    expect(sendBackProblems(answerProblems(ledger, fx.answers.send_back))).toBe(
      fx.answers.send_back_want
    )
  })

  test("asks for answers as Go does", () => {
    for (const c of fx.missing_answers.cases) {
      expect(
        missingAnswers(c.input, fx.missing_answers.question) ?? null
      ).toEqual(c.want)
    }
  })
})

type RangeJSON = { from: string; to: string }

type Step = {
  read?: { content: string; names?: string[]; ranges?: RangeJSON[] }
  issue?: {
    tool: string
    input: string
    content: string
    ranges?: RangeJSON[]
    want: string
  }
  sourced?: { content: string }
}

type SignalWant = {
  signals: Signals[]
  scored: { score: number; band: string; queries: string[] }[] | null
}

type Snapshot = {
  reads: number[][]
  names: string[]
  discovered: string[]
  sourced: string[]
  ranges: RangeJSON[]
  instants: string[]
  ids: string[]
  facts: Fact[]
  next_fact: number
}

type Scenario = {
  name: string
  steps: Step[]
  reviews?: {
    name?: string
    input: unknown
    question?: string
    want: Problem[]
    send_back: string
  }[]
  signals?: { name?: string; input: unknown; want: SignalWant | null }[]
  renders?: { text: string; want: RenderWant }[]
  record?: Snapshot
}

export function rangesOf(list: readonly RangeJSON[] | undefined): Range[] {
  return (list ?? []).map((r) => ({
    from: parseTime(r.from) as number,
    to: parseTime(r.to) as number,
  }))
}

function stamp(ms: number): string {
  const iso = new Date(ms).toISOString()
  return iso.endsWith(".000Z")
    ? iso.slice(0, -5) + "Z"
    : iso.replace(/0+Z$/, "Z")
}

function snapshot(r: GroundingRecord): Snapshot {
  return {
    reads: r.reads,
    names: r.names,
    discovered: r.discovered,
    sourced: r.sourced,
    ranges: r.ranges.map((x) => ({ from: stamp(x.from), to: stamp(x.to) })),
    instants: r.instants.map(stamp),
    ids: r.ids,
    facts: r.facts,
    next_fact: r.nextFact,
  }
}

export function play(steps: readonly Step[]): GroundingRecord {
  const r = newRecord("enforce") as GroundingRecord
  for (const step of steps) {
    if (step.read) {
      addRead(
        r,
        step.read.content,
        step.read.names ?? [],
        rangesOf(step.read.ranges)
      )
    } else if (step.issue) {
      const got = issueFacts(
        r,
        step.issue.tool,
        step.issue.input,
        step.issue.content,
        rangesOf(step.issue.ranges)
      )
      expect(got).toBe(step.issue.want)
    } else if (step.sourced) {
      addSourced(r, step.sourced.content)
    }
  }
  return r
}

describe("scenarios", () => {
  const scenarios = fixture<Scenario[]>("scenarios.json")

  for (const sc of scenarios) {
    test(sc.name, () => {
      const r = play(sc.steps)
      for (const c of sc.reviews ?? []) {
        const problems = review(r, c.input, c.question ?? "")
        expect({ name: c.name, got: problems }).toEqual({
          name: c.name,
          got: c.want,
        })
        expect(problems.length > 0 ? sendBackProblems(problems) : "").toBe(
          c.send_back
        )
      }
      for (const c of sc.signals ?? []) {
        const signals = causeSignals(r, c.input)
        const scored = scoredCauses(r, c.input)
        const got =
          signals === undefined
            ? null
            : {
                signals,
                scored:
                  scored?.map((s) => ({
                    score: s.score,
                    band: s.band,
                    queries: s.queries,
                  })) ?? null,
              }
        expect({ name: c.name, got }).toEqual({ name: c.name, got: c.want })
      }
      for (const c of sc.renders ?? []) {
        expect(ledgerOf(r)?.render(c.text)).toEqual(c.want)
      }
      if (sc.record) {
        expect(snapshot(r)).toEqual(sc.record)
      }
    })
  }
})

type SessionCase = {
  name: string
  mode: "enforce" | "off"
  task?: string
  question: string
  calls: {
    tool: string
    input: unknown
    content: string
    is_error?: boolean
    discovered?: { datasource: string; of: string; names: string[] }
    ranges?: RangeJSON[]
    want: string
  }[]
  saves: {
    input: unknown
    want: {
      send_back: string
      sent_back: boolean
      unverified: unknown[]
      saved: unknown
    }
  }[]
}

describe("sessions", () => {
  for (const c of fixture<SessionCase[]>("sessions.json")) {
    test(c.name, () => {
      const r = startGrounding(c.mode)
      for (const call of c.calls) {
        const content = noteResult(r, call.tool, call.input, {
          content: call.content,
          isError: call.is_error ?? false,
          meta: {
            edits: null,
            refusal: null,
            unchecked: null,
            discovered: call.discovered ?? null,
            ranges: call.ranges ?? [],
          },
        })
        expect({ tool: call.tool, content }).toEqual({
          tool: call.tool,
          content: call.want,
        })
      }
      const task = parseTask(c.task ?? "") as Task
      for (const save of c.saves) {
        const check = checkSave(r, save.input, c.question, task)
        expect(check.sendBack).toBe(save.want.send_back)
        expect(check.unverified).toEqual(save.want.unverified as never)
        expect(savedFinding(r, save.input, check)).toEqual(save.want.saved)
      }
    })
  }
})

function record(...reads: string[]): GroundingRecord {
  const r = newRecord("enforce") as GroundingRecord
  for (const read of reads) {
    addRead(r, read, [], [])
  }
  return r
}

describe("bounds", () => {
  test("a record keeps at most maxValues values, the newest", () => {
    const r = record()
    for (let read = 0; read < 10; read++) {
      const values = Array.from(
        { length: 600 },
        (_, i) => (100 + i) * 10 ** read
      )
      addRead(r, JSON.stringify(values), [], [])
    }
    expect(r.reads.flat().length).toBe(maxValues)
    const last = r.reads.at(-1) as number[]
    expect(last.at(-1)).toBe(rounded((100 + maxValuesOfRead - 1) * 1e9))
  })

  test("a read keeps its first values whatever the key order", () => {
    const object: Record<string, number> = {}
    for (let i = 0; i < 600; i++) {
      object[`k${String(i).padStart(3, "0")}`] = (100 + i) * 10
    }
    const keys = Object.keys(object).reverse()
    const reversed = Object.fromEntries(keys.map((k) => [k, object[k]]))
    const first = record(JSON.stringify(object))
      .reads.flat()
      .sort((a, b) => a - b)
    expect(first.length).toBe(maxValuesOfRead)
    expect(first.at(-1)).toBe((100 + maxValuesOfRead - 1) * 10)
    expect(
      record(JSON.stringify(reversed))
        .reads.flat()
        .sort((a, b) => a - b)
    ).toEqual(first)
  })

  test("a long text is cut at a word boundary", () => {
    const text = JSON.stringify(["a".repeat(4092) + " 123456 more"])
    expect(record(text).reads).toEqual([])
  })

  test("the ledger drops its oldest facts first", () => {
    const r = record()
    for (let i = 0; i < maxFacts + 5; i++) {
      issueFacts(
        r,
        "measure_aggregate",
        "",
        `{"op":"total","values":[{"value":${i}}]}`,
        []
      )
    }
    expect(r.facts.length).toBe(maxFacts)
    expect(r.facts[0]?.id).toBe("f6")
    expect(r.nextFact).toBe(maxFacts + 5)
  })

  test("discovered names are never evicted", () => {
    const r = record()
    addRead(r, "", ["flagd"], [])
    const labels = Array.from({ length: 600 }, (_, i) => ({
      labels: { pod: `pod-${i}` },
    }))
    addRead(r, JSON.stringify(labels), [], [])
    expect(sawName(r, "flagd")).toBe(true)
    expect(r.names.length).toBe(400)
  })

  test("a source result keeps its first names and the newest are kept", () => {
    const words = Array.from(
      { length: maxSourcedOfRead + 50 },
      (_, i) => `name${String(i).padStart(4, "0")}`
    )
    const got = sourceNames(words.join(" "))
    expect(got.length).toBe(maxSourcedOfRead)
    expect(got.at(-1)).toBe(words[maxSourcedOfRead - 1])
    const r = record()
    addRead(r, `{"series":[{"labels":{"service":"cart"}}]}`, ["checkout"], [])
    const source = (text: string) =>
      JSON.stringify({
        notice: "n",
        source: "cluster",
        tool: "kubectl_get",
        text,
      })
    addSourced(r, source("paymentFailure"))
    for (let batch = 0; batch <= maxSourced / maxSourcedOfRead; batch++) {
      const names = Array.from(
        { length: maxSourcedOfRead },
        (_, i) => `b${batch}n${String(i).padStart(4, "0")}`
      )
      addSourced(r, source(names.join(" ")))
    }
    expect(r.sourced.length).toBe(maxSourced)
    expect(sawName(r, "paymentFailure") || sawName(r, "b0n0000")).toBe(false)
    expect(
      sawName(r, "cart") && sawName(r, "checkout") && sawName(r, "b4n0499")
    ).toBe(true)
  })

  test("off keeps no record and checks nothing", () => {
    const r = newRecord("off")
    addRead(r, `{"v":1}`, ["checkout"], [])
    expect(r).toBeUndefined()
    expect(review(r, { markdown: "12,345 errors" }, "")).toEqual([])
    expect(causeSignals(r, { causes: [{}] })).toBeUndefined()
  })

  test("formats floats as Go does at the edges", () => {
    expect(formatFloat(1250, "g", 2)).toBe("1.2e+03")
    expect(formatFloat(0.125, "f", 2)).toBe("0.12")
    expect(formatFloat(2.5, "f", 0)).toBe("2")
    expect(formatFloat(1e21, "f", -1)).toBe("1000000000000000000000")
    expect(formatFloat(-0, "g", 3)).toBe("-0")
  })
})
