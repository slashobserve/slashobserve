import {
  angleLink,
  bareURL,
  blankWith,
  fencedCode,
  linkTarget,
} from "./figures"
import { kinds, type Problem } from "./problems"

const minCutID = 8

type HexToken = { text: string; start: number; end: number }

function idChar(c: string): boolean {
  return /^[0-9a-zA-Z_-]$/.test(c)
}

function looksLikeID(token: string): boolean {
  if (token.length < minCutID || token.length > 32) {
    return false
  }
  return (
    /^[0-9a-fA-F]+$/.test(token) &&
    /[0-9]/.test(token) &&
    /[a-fA-F]/.test(token)
  )
}

function hexTokens(text: string): HexToken[] {
  const out: HexToken[] = []
  for (let i = 0; i < text.length;) {
    if (!idChar(text[i] ?? "")) {
      i++
      continue
    }
    let j = i
    while (j < text.length && idChar(text[j] ?? "")) {
      j++
    }
    const token = text.slice(i, j)
    if (looksLikeID(token)) {
      out.push({ text: token, start: i, end: j })
    }
    i = j
  }
  return out
}

export function blankIDs(text: string): string {
  const tokens = hexTokens(text)
  if (tokens.length === 0) {
    return text
  }
  let out = ""
  let at = 0
  for (const t of tokens) {
    out += text.slice(at, t.start) + " ".repeat(t.end - t.start)
    at = t.end
  }
  return out + text.slice(at)
}

export function traceIDProblems(
  text: string,
  known: readonly string[]
): Problem[] {
  const problems: Problem[] = []
  for (const t of hexTokens(
    blankWith(text, fencedCode, linkTarget, angleLink, bareURL)
  )) {
    const id = t.text.toLowerCase()
    if (known.includes(id)) {
      continue
    }
    const full = known.find((k) => k.startsWith(id))
    if (full !== undefined) {
      problems.push({
        kind: kinds.traceID,
        text: t.text,
        fix: `an id cut short: write the whole id ${full}, or cite its fact`,
      })
      continue
    }
    if (id.length === 16 || id.length === 32) {
      problems.push({
        kind: kinds.traceID,
        text: t.text,
        fix: "not an id this run read: read the trace, cite an id a result gave, or remove it",
      })
    }
  }
  return problems
}
