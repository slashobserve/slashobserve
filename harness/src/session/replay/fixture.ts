import { goQuote } from "./goquote"
import { type JsonNode, JsonSyntaxError, parseJsonDocument } from "./raw"

export type Fault = { kind: string; delayMs: number }
export type FixtureCall = { name: string; input: string }
export type Expectation = { isError: boolean | undefined; contains: string }

export type FixtureTurn = {
  text: string
  toolCalls: FixtureCall[]
  expect: Expectation[] | undefined
  fault: Fault | undefined
}

export type Segment = { turns: FixtureTurn[] }

export type Fixture = {
  file: string
  prompt: string
  turns: FixtureTurn[]
  resumes: Map<string, Segment>
  resumeAfterSaves: boolean
}

export const faultKinds = [
  "rate_limited",
  "server_error",
  "call_timeout",
  "max_tokens",
  "refusal",
  "delay",
] as const

const saveTools = ["create_dashboard", "update_dashboard", "update_panels"]

const goSpace = new RegExp(
  "[\\t-\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]+"
)

function goLower(text: string): string {
  let lowered = ""
  for (const rune of text) {
    const single = rune.toLowerCase()
    lowered += [...single].length === 1 ? single : rune === "İ" ? "i" : rune
  }
  return lowered
}

export function promptKey(prompt: string): string {
  return goLower(prompt)
    .split(goSpace)
    .filter((part) => part !== "")
    .join(" ")
}

type Path = readonly string[]

const rootStruct = "fixture"

function pointerToken(token: string): string {
  return token.replaceAll("~", "~0").replaceAll("/", "~1")
}

class Decoder {
  private saved: string | undefined

  get error(): string | undefined {
    return this.saved
  }

  private save(message: string): void {
    this.saved ??= message
  }

  private mismatch(value: string, type: string, at: Path): void {
    if (at.length === 0) {
      this.save(`json: cannot unmarshal ${value} into Go value of type ${type}`)
      return
    }
    const field = at.map(pointerToken).join(".")
    const last = field.slice(field.lastIndexOf(".") + 1)
    const into = /^\d+$/.test(last) ? "" : "Go struct field "
    this.save(
      `json: cannot unmarshal ${value} into ${into}${rootStruct}.${field} of type ${type}`
    )
  }

  text(node: JsonNode, at: Path): string {
    if (node.kind === "string") {
      return node.value
    }
    if (node.kind !== "null") {
      this.mismatch(node.kind, "string", at)
    }
    return ""
  }

  flag(node: JsonNode, at: Path): boolean | undefined {
    if (node.kind === "bool") {
      return node.raw === "true"
    }
    if (node.kind !== "null") {
      this.mismatch(node.kind, "bool", at)
    }
    return undefined
  }

  integer(node: JsonNode, at: Path): number {
    if (node.kind === "number") {
      if (
        /^-?\d+$/.test(node.raw) &&
        BigInt(node.raw) >= -(2n ** 63n) &&
        BigInt(node.raw) < 2n ** 63n
      ) {
        return Number(node.raw)
      }
      this.mismatch(`number ${node.raw}`, "int", at)
    } else if (node.kind !== "null") {
      this.mismatch(node.kind, "int", at)
    }
    return 0
  }

  list<T>(
    node: JsonNode,
    at: Path,
    type: string,
    element: (item: JsonNode, at: Path) => T
  ): T[] | undefined {
    if (node.kind === "array") {
      return node.items.map((item, i) => element(item, [...at, String(i)]))
    }
    if (node.kind !== "null") {
      this.mismatch(node.kind, type, at)
    }
    return undefined
  }

  fields(
    node: JsonNode,
    struct: string,
    at: Path,
    handlers: Record<string, (value: JsonNode, inner: Path) => void>
  ): void {
    if (node.kind === "null") {
      return
    }
    if (node.kind !== "object") {
      this.mismatch(node.kind, `replay.${struct}`, at)
      return
    }
    const names = Object.keys(handlers)
    for (const { key, value } of node.members) {
      const name = names.includes(key)
        ? key
        : names.find((candidate) => candidate === key.toLowerCase())
      const handler = name === undefined ? undefined : handlers[name]
      if (handler === undefined) {
        this.save(`json: unknown field ${goQuote(key)}`)
        continue
      }
      handler(value, [...at, key])
    }
  }

  fault(node: JsonNode, at: Path): Fault | undefined {
    if (node.kind === "null") {
      return undefined
    }
    const fault: Fault = { kind: "", delayMs: 0 }
    this.fields(node, "fault", at, {
      kind: (value, inner) => {
        fault.kind = this.text(value, inner)
      },
      delay_ms: (value, inner) => {
        fault.delayMs = this.integer(value, inner)
      },
    })
    return fault
  }

  call(node: JsonNode, at: Path): FixtureCall {
    const call: FixtureCall = { name: "", input: "" }
    this.fields(node, "fixtureCall", at, {
      name: (value, inner) => {
        call.name = this.text(value, inner)
      },
      input: (value) => {
        call.input = value.raw
      },
    })
    return call
  }

  expectation(node: JsonNode, at: Path): Expectation {
    const expectation: Expectation = { isError: undefined, contains: "" }
    this.fields(node, "expectation", at, {
      is_error: (value, inner) => {
        expectation.isError = this.flag(value, inner)
      },
      contains: (value, inner) => {
        expectation.contains = this.text(value, inner)
      },
    })
    return expectation
  }

  turn(node: JsonNode, at: Path): FixtureTurn {
    const turn: FixtureTurn = {
      text: "",
      toolCalls: [],
      expect: undefined,
      fault: undefined,
    }
    this.fields(node, "turn", at, {
      text: (value, inner) => {
        turn.text = this.text(value, inner)
      },
      tool_calls: (value, inner) => {
        turn.toolCalls =
          this.list(value, inner, "[]replay.fixtureCall", (item, where) =>
            this.call(item, where)
          ) ?? []
      },
      expect: (value, inner) => {
        turn.expect = this.list(
          value,
          inner,
          "[]replay.expectation",
          (item, where) => this.expectation(item, where)
        )
      },
      fault: (value, inner) => {
        turn.fault = this.fault(value, inner)
      },
    })
    return turn
  }

  turns(value: JsonNode, at: Path): FixtureTurn[] {
    return (
      this.list(value, at, "[]replay.turn", (item, where) =>
        this.turn(item, where)
      ) ?? []
    )
  }

  segment(node: JsonNode, at: Path): Segment {
    const segment: Segment = { turns: [] }
    this.fields(node, "segment", at, {
      turns: (value, inner) => {
        segment.turns = this.turns(value, inner)
      },
    })
    return segment
  }

  resumes(node: JsonNode, at: Path): Map<string, Segment> {
    const resumes = new Map<string, Segment>()
    if (node.kind === "object") {
      for (const { key, value } of node.members) {
        resumes.set(key, this.segment(value, [...at, key]))
      }
    } else if (node.kind !== "null") {
      this.mismatch(node.kind, "map[string]replay.segment", at)
    }
    return resumes
  }

  fixture(node: JsonNode, file: string): Fixture {
    const fixture: Fixture = {
      file,
      prompt: "",
      turns: [],
      resumes: new Map(),
      resumeAfterSaves: false,
    }
    this.fields(node, "fixture", [], {
      prompt: (value, inner) => {
        fixture.prompt = this.text(value, inner)
      },
      turns: (value, inner) => {
        fixture.turns = this.turns(value, inner)
      },
      resumes: (value, inner) => {
        fixture.resumes = this.resumes(value, inner)
      },
      resume_after_saves: (value, inner) => {
        fixture.resumeAfterSaves = this.flag(value, inner) ?? false
      },
    })
    return fixture
  }
}

function describeFault(fault: Fault): string | undefined {
  if (!(faultKinds as readonly string[]).includes(fault.kind)) {
    return `kind ${goQuote(fault.kind)} is not one of ${faultKinds.join(", ")}`
  }
  if (fault.delayMs < 0) {
    return "delay_ms must not be negative"
  }
  return undefined
}

function describeCall(call: FixtureCall): string | undefined {
  if (call.name === "") {
    return "name is required"
  }
  if (call.input.trim() === "") {
    call.input = "{}"
    return undefined
  }
  return call.input.startsWith("{") ? undefined : "input must be a JSON object"
}

function describeTurn(
  turn: FixtureTurn,
  index: number,
  previousCalls: number
): string | undefined {
  if (turn.fault !== undefined) {
    const problem = describeFault(turn.fault)
    if (problem !== undefined) {
      return `fault: ${problem}`
    }
  } else if (turn.text === "" && turn.toolCalls.length === 0) {
    return "needs text or tool_calls"
  }
  for (const [i, call] of turn.toolCalls.entries()) {
    const problem = describeCall(call)
    if (problem !== undefined) {
      return `tool_calls[${i}]: ${problem}`
    }
  }
  if (turn.expect === undefined) {
    return undefined
  }
  if (index === 0) {
    return "expect is not allowed on the first turn"
  }
  if (turn.expect.length !== previousCalls) {
    return `expect has ${turn.expect.length} entries but the previous turn has ${previousCalls} tool calls`
  }
  for (const [i, expectation] of turn.expect.entries()) {
    if (expectation.isError === undefined) {
      return `expect[${i}]: is_error is required`
    }
  }
  return undefined
}

function describeTurns(
  label: string,
  turns: FixtureTurn[]
): string | undefined {
  if (turns.length === 0) {
    return `${label} needs at least one turn`
  }
  for (const [i, turn] of turns.entries()) {
    const previousCalls = i > 0 ? (turns[i - 1]?.toolCalls.length ?? 0) : 0
    const problem = describeTurn(turn, i, previousCalls)
    if (problem !== undefined) {
      return `${label}: turn ${i}: ${problem}`
    }
  }
  return undefined
}

function saveCalls(turn: FixtureTurn): number {
  return turn.toolCalls.filter((call) => saveTools.includes(call.name)).length
}

function describeResumeAfterSaves(fixture: Fixture): string | undefined {
  if (fixture.resumes.size > 0) {
    return "resume_after_saves cannot be combined with resumes"
  }
  for (const [i, turn] of fixture.turns.entries()) {
    const saves = saveCalls(turn)
    if (saves > 1) {
      return `turns: turn ${i} has ${saves} save calls; resume_after_saves needs at most one per turn`
    }
    const previous = fixture.turns[i - 1]
    if (
      i > 0 &&
      turn.expect !== undefined &&
      previous &&
      saveCalls(previous) > 0
    ) {
      return `turns: turn ${i}: expect is not allowed on the turn after a save when resume_after_saves is set, because a resume starts there`
    }
  }
  return undefined
}

const maxInt64 = 2n ** 63n - 1n

function isResumeKey(key: string): boolean {
  return /^[1-9]\d*$/.test(key) && BigInt(key) <= maxInt64
}

function byteOrder(a: string, b: string): number {
  return Buffer.compare(Buffer.from(a), Buffer.from(b))
}

function describeFixture(fixture: Fixture): string | undefined {
  if (promptKey(fixture.prompt) === "") {
    return "prompt is required"
  }
  const turns = describeTurns("turns", fixture.turns)
  if (turns !== undefined) {
    return turns
  }
  if (fixture.resumeAfterSaves) {
    const problem = describeResumeAfterSaves(fixture)
    if (problem !== undefined) {
      return problem
    }
  }
  for (const key of [...fixture.resumes.keys()].sort(byteOrder)) {
    if (!isResumeKey(key)) {
      return `resumes key ${goQuote(key)} is not a positive integer`
    }
    const problem = describeTurns(
      `resumes.${goQuote(key)}.turns`,
      fixture.resumes.get(key)?.turns ?? []
    )
    if (problem !== undefined) {
      return problem
    }
  }
  return undefined
}

export class FixtureError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "FixtureError"
  }
}

export function parseFixture(file: string, content: string): Fixture {
  const fail = (reason: string): never => {
    throw new FixtureError(`replay fixture ${file}: ${reason}`)
  }
  let document
  try {
    document = parseJsonDocument(content)
  } catch (error) {
    if (error instanceof JsonSyntaxError) {
      return fail(error.message)
    }
    throw error
  }
  const decoder = new Decoder()
  const fixture = decoder.fixture(document.node, file)
  if (decoder.error !== undefined) {
    return fail(decoder.error)
  }
  if (document.trailing) {
    return fail("unexpected content after the fixture object")
  }
  const problem = describeFixture(fixture)
  if (problem !== undefined) {
    return fail(problem)
  }
  return fixture
}

export function turnsFor(
  fixture: Fixture,
  buildSaves: number
): FixtureTurn[] | undefined {
  if (buildSaves === 0) {
    return fixture.turns
  }
  if (fixture.resumeAfterSaves) {
    return turnsAfterSave(fixture, buildSaves)
  }
  return fixture.resumes.get(String(buildSaves))?.turns
}

function turnsAfterSave(
  fixture: Fixture,
  k: number
): FixtureTurn[] | undefined {
  let seen = 0
  for (const [i, turn] of fixture.turns.entries()) {
    if (saveCalls(turn) === 0) {
      continue
    }
    seen++
    if (seen === k) {
      return fixture.turns.slice(i + 1)
    }
  }
  return undefined
}
