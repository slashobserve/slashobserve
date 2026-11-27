import { readsData } from "../phase"
import type { CallMeta } from "../protocol"
import type { Task } from "../route/route"
import { scoredCauses, scoredRootCause, type ScoredCause } from "./causes"
import { issueFacts } from "./facts"
import { sendBackProblems, type Unverified, unverifiedOf } from "./problems"
import {
  addRead,
  addSourced,
  type GroundingRecord,
  type Mode,
  newRecord,
  type Range,
} from "./record"
import { render } from "./render"
import { ledgerOf, missingAnswers, review } from "./review"
import { parseTime } from "./time"

export type { GroundingRecord, Mode } from "./record"

const callMCPTool = "call_mcp_tool"

export function parseMode(raw: string | undefined): Mode | undefined {
  switch (raw) {
    case undefined:
    case "":
    case "enforce":
      return "enforce"
    case "off":
      return "off"
  }
  return undefined
}

export function startGrounding(mode: Mode): GroundingRecord | undefined {
  return newRecord(mode)
}

function rangesOf(meta: CallMeta | undefined): Range[] {
  const out: Range[] = []
  for (const r of meta?.ranges ?? []) {
    const from = parseTime(r.from)
    const to = parseTime(r.to)
    if (from !== undefined && to !== undefined) {
      out.push({ from, to })
    }
  }
  return out
}

export type ToolResult = {
  content: string
  isError: boolean
  meta?: CallMeta | undefined
}

export function noteResult(
  r: GroundingRecord | undefined,
  tool: string,
  input: unknown,
  result: ToolResult
): string {
  if (!r || result.isError) {
    return result.content
  }
  const ranges = rangesOf(result.meta)
  const content = issueFacts(
    r,
    tool,
    JSON.stringify(input ?? null),
    result.content,
    ranges
  )
  const discovered = result.meta?.discovered
  if (!readsData(tool) && !discovered) {
    return content
  }
  if (tool === callMCPTool) {
    addSourced(r, content)
    return content
  }
  addRead(r, content, discovered?.names ?? [], ranges)
  return content
}

function answersAFigure(task: Task): boolean {
  return task === "measure" || task === "logs" || task === "traces"
}

export type SaveCheck = {
  sendBack: string
  unverified: Unverified[]
  causes: ScoredCause[] | undefined
  rootCause?: ScoredCause | undefined
}

export function checkSave(
  r: GroundingRecord | undefined,
  input: unknown,
  question: string,
  task: Task
): SaveCheck {
  if (!r) {
    return { sendBack: "", unverified: [], causes: undefined }
  }
  const problems = review(r, input, question)
  const sendBack = [...problems]
  const missing = missingAnswers(input, question)
  if (missing && answersAFigure(task)) {
    sendBack.push(missing)
  }
  return {
    sendBack: sendBack.length > 0 ? sendBackProblems(sendBack) : "",
    unverified: unverifiedOf(problems),
    causes: scoredCauses(r, input),
    rootCause: scoredRootCause(r, input),
  }
}

type Fields = Record<string, unknown>

function isFields(v: unknown): v is Fields {
  return typeof v === "object" && v !== null && !Array.isArray(v)
}

function scoreInto(c: Fields, scored: ScoredCause): void {
  c["signals"] = scored.signals
  c["score"] = scored.score
  c["band"] = scored.band
  if (scored.queries.length > 0) {
    c["queries"] = scored.queries
  } else {
    delete c["queries"]
  }
}

export function savedFinding(
  r: GroundingRecord | undefined,
  input: unknown,
  check: SaveCheck,
  more: readonly Unverified[] = []
): unknown {
  if (!isFields(input)) {
    return input
  }
  const out: Fields = structuredClone(input)
  out["unverified"] = [...check.unverified, ...more]
  if (!r) {
    return out
  }
  const ledger = ledgerOf(r)
  const rendered = (v: unknown) =>
    typeof v === "string" ? render(ledger, v).text : v
  for (const key of ["title", "markdown", "impact"]) {
    if (key in out) {
      out[key] = rendered(out[key])
    }
  }
  const rootCause = out["root_cause"]
  if (isFields(rootCause)) {
    if ("summary" in rootCause) {
      rootCause["summary"] = rendered(rootCause["summary"])
    }
    if (check.rootCause) {
      scoreInto(rootCause, check.rootCause)
    }
  }
  for (const key of ["ruled_out", "next_steps"]) {
    const list = out[key]
    if (Array.isArray(list)) {
      out[key] = list.map(rendered)
    }
  }
  const causes = out["causes"]
  if (Array.isArray(causes)) {
    causes.forEach((c: unknown, i) => {
      if (!isFields(c)) {
        return
      }
      if ("mechanism" in c) {
        c["mechanism"] = rendered(c["mechanism"])
      }
      const scored = check.causes?.[i]
      if (scored) {
        scoreInto(c, scored)
      }
    })
  }
  return out
}
