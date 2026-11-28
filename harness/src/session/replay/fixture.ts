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

export function promptKey(prompt: string): string {
  return prompt
    .toLowerCase()
    .split(/\s+/u)
    .filter((part) => part !== "")
    .join(" ")
}

type Json = null | boolean | number | string | Json[] | { [key: string]: Json }
type JsonObject = { [key: string]: Json }

class ShapeError extends Error {}

function where(path: string): string {
  return path === "" ? "" : `${path}: `
}

function isObject(value: Json): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function object(
  value: Json,
  path: string,
  keys: readonly string[]
): JsonObject {
  if (!isObject(value)) {
    throw new ShapeError(`${where(path)}must be an object`)
  }
  for (const key of Object.keys(value)) {
    if (!keys.includes(key)) {
      throw new ShapeError(`${where(path)}unknown field ${JSON.stringify(key)}`)
    }
  }
  return value
}

function text(value: Json | undefined, path: string): string {
  if (value === undefined) {
    return ""
  }
  if (typeof value !== "string") {
    throw new ShapeError(`${path}: must be a string`)
  }
  return value
}

function flag(value: Json | undefined, path: string): boolean | undefined {
  if (value === undefined) {
    return undefined
  }
  if (typeof value !== "boolean") {
    throw new ShapeError(`${path}: must be true or false`)
  }
  return value
}

function integer(value: Json | undefined, path: string): number {
  if (value === undefined) {
    return 0
  }
  if (typeof value !== "number" || !Number.isSafeInteger(value)) {
    throw new ShapeError(`${path}: must be a whole number`)
  }
  return value
}

function list<T>(
  value: Json | undefined,
  path: string,
  item: (value: Json, path: string) => T
): T[] | undefined {
  if (value === undefined) {
    return undefined
  }
  if (!Array.isArray(value)) {
    throw new ShapeError(`${path}: must be an array`)
  }
  return value.map((entry, i) => item(entry, `${path}[${i}]`))
}

function decodeFault(value: Json, path: string): Fault {
  const o = object(value, path, ["kind", "delay_ms"])
  return {
    kind: text(o.kind, `${path}.kind`),
    delayMs: integer(o.delay_ms, `${path}.delay_ms`),
  }
}

function decodeCall(value: Json, path: string): FixtureCall {
  const o = object(value, path, ["name", "input"])
  if (o.input !== undefined && !isObject(o.input)) {
    throw new ShapeError(`${path}.input: must be an object`)
  }
  return {
    name: text(o.name, `${path}.name`),
    input: JSON.stringify(o.input ?? {}),
  }
}

function decodeExpectation(value: Json, path: string): Expectation {
  const o = object(value, path, ["is_error", "contains"])
  return {
    isError: flag(o.is_error, `${path}.is_error`),
    contains: text(o.contains, `${path}.contains`),
  }
}

function decodeTurn(value: Json, path: string): FixtureTurn {
  const o = object(value, path, ["text", "tool_calls", "expect", "fault"])
  return {
    text: text(o.text, `${path}.text`),
    toolCalls: list(o.tool_calls, `${path}.tool_calls`, decodeCall) ?? [],
    expect: list(o.expect, `${path}.expect`, decodeExpectation),
    fault:
      o.fault === undefined ? undefined : decodeFault(o.fault, `${path}.fault`),
  }
}

function decodeTurns(value: Json | undefined, path: string): FixtureTurn[] {
  return list(value, path, decodeTurn) ?? []
}

function decodeResumes(
  value: Json | undefined,
  path: string
): Map<string, Segment> {
  const resumes = new Map<string, Segment>()
  if (value === undefined) {
    return resumes
  }
  if (!isObject(value)) {
    throw new ShapeError(`${path}: must be an object`)
  }
  for (const [key, segment] of Object.entries(value)) {
    const at = `${path}.${key}`
    const o = object(segment, at, ["turns"])
    resumes.set(key, { turns: decodeTurns(o.turns, `${at}.turns`) })
  }
  return resumes
}

function decodeFixture(value: Json, file: string): Fixture {
  const o = object(value, "", [
    "prompt",
    "turns",
    "resumes",
    "resume_after_saves",
  ])
  return {
    file,
    prompt: text(o.prompt, "prompt"),
    turns: decodeTurns(o.turns, "turns"),
    resumes: decodeResumes(o.resumes, "resumes"),
    resumeAfterSaves: flag(o.resume_after_saves, "resume_after_saves") ?? false,
  }
}

function describeFault(fault: Fault): string | undefined {
  if (!(faultKinds as readonly string[]).includes(fault.kind)) {
    return `kind ${JSON.stringify(fault.kind)} is not one of ${faultKinds.join(", ")}`
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
  return undefined
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
  for (const key of [...fixture.resumes.keys()].sort()) {
    if (!isResumeKey(key)) {
      return `resumes key ${JSON.stringify(key)} is not a positive integer`
    }
    const problem = describeTurns(
      `resumes.${JSON.stringify(key)}.turns`,
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
  let document: Json
  try {
    document = JSON.parse(content) as Json
  } catch (error) {
    return fail(`not valid JSON: ${(error as Error).message}`)
  }
  let fixture: Fixture
  try {
    fixture = decodeFixture(document, file)
  } catch (error) {
    if (error instanceof ShapeError) {
      return fail(error.message)
    }
    throw error
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
