export type QueryForm = "kql" | "lucene" | "dsl"

const whitespace = /^[ \t\r\n]*/
const quotedValue = /"(?:[^"\\]|\\.)*"/
const luceneSignals = [
  /(?:^|[\s(])[+-][\w"(@*]/,
  /[\w")]~\d*(?:[\s)]|$)/,
  /[\w")]\^\d/,
  /\b[\w.@-]+:\S+\s+(?:AND|OR)\s/,
]
const luceneOnly = /[[\]{}^]/
const dslSkeleton = '{\n  "query": {\n    "match_all": {}\n  }\n}'

export const formLabels: Record<QueryForm, string> = {
  kql: "KQL",
  lucene: "Lucene",
  dsl: "Query DSL",
}

function prefixOf(rest: string): { form: QueryForm; length: number } | null {
  const lower = rest.slice(0, 7).toLowerCase()
  if (lower === "lucene:") return { form: "lucene", length: 7 }
  if (lower.startsWith("kql:")) return { form: "kql", length: 4 }
  return null
}

export function splitForm(
  text: string,
  defaultForm: QueryForm = "kql"
): { form: QueryForm; body: string; offset: number } {
  const lead = whitespace.exec(text)?.[0].length ?? 0
  const rest = text.slice(lead)
  if (rest.startsWith("{")) {
    return { form: "dsl", body: rest.trim(), offset: lead }
  }
  const prefix = prefixOf(rest)
  const form = prefix?.form ?? defaultForm
  const afterPrefix = rest.slice(prefix?.length ?? 0)
  const skipped = whitespace.exec(afterPrefix)?.[0].length ?? 0
  return {
    form,
    body: afterPrefix.trim(),
    offset: lead + (prefix?.length ?? 0) + skipped,
  }
}

export function formOf(
  text: string,
  defaultForm: QueryForm = "kql"
): QueryForm {
  return splitForm(text, defaultForm).form
}

export function withForm(text: string, form: QueryForm): string {
  const { form: current, body } = splitForm(text)
  if (form === "dsl") return body === "" ? dslSkeleton : text
  if (current === "dsl") return text
  if (form === "lucene") return body === "" ? "lucene: " : `lucene: ${body}`
  return body
}

export function looksLikeLucene(text: string): boolean {
  return luceneSignals.some((signal) => signal.test(text))
}

function lowerOperators(segment: string): string {
  return segment.replace(/(?<=^|[\s)])(AND|OR|NOT)(?=[\s(]|$)/g, (word) =>
    word.toLowerCase()
  )
}

function kqlSegment(segment: string): string {
  return lowerOperators(
    segment
      .replace(/(^|[\s(])\+(?=[^\s)])/g, "$1")
      .replace(/(^|[\s(])-(?=[^\s)])/g, "$1not ")
      .replace(/~\d*(?=[\s)]|$)/g, "")
  )
}

export function kqlFromLucene(text: string): string | null {
  const { form, body } = splitForm(text)
  if (form === "dsl") return null
  const pieces = body.split(new RegExp(`(${quotedValue.source})`))
  const unquoted = pieces.filter((_, index) => index % 2 === 0)
  if (unquoted.some((piece) => luceneOnly.test(piece))) return null
  const converted = pieces
    .map((piece, index) => (index % 2 === 0 ? kqlSegment(piece) : piece))
    .join("")
  return converted === body ? null : converted
}
