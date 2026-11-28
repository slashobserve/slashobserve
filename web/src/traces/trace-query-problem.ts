const parseError =
  /^(?:invalid query: )?(?:invalid TraceQL query: )?parse error at line (\d+), col (\d+): (?:syntax error: )?(.*)$/s

const cleanedByServer = /^(?:invalid query: )?(Invalid TraceQL at .*)$/s
const doubledPrefix = /^invalid query: (invalid TraceQL query: )/
const excerptLength = 30

function readable(detail: string): string {
  return detail.replaceAll("$end", "end of the query")
}

function before(query: string, line: number, column: number): string {
  const lines = query.split("\n")
  const text = [
    ...lines.slice(0, line - 1),
    lines[line - 1]?.slice(0, column - 1),
  ]
    .join("\n")
    .trim()
  return text.length > excerptLength ? `...${text.slice(-excerptLength)}` : text
}

export function traceQLProblem(message: string, query: string): string {
  const cleaned = cleanedByServer.exec(message)?.[1]
  if (cleaned !== undefined) return cleaned
  const [, lineText, columnText, problem] = parseError.exec(message) ?? []
  if (problem === undefined) return message.replace(doubledPrefix, "$1")
  const line = Number(lineText)
  const column = Number(columnText)
  const text = before(query, line, column)
  const where = `line ${line}, column ${column}`
  const after = text === "" ? "" : ` after "${text}"`
  return `Invalid TraceQL at ${where}${after}: ${readable(problem)}.`
}

const searchLimit = /Tempo searches at most (\d+) (day|hour|minute)s? at a time/

export function shorterRange(
  message: string
): { from: string; label: string } | null {
  const [, countText, unit] = searchLimit.exec(message) ?? []
  if (countText === undefined || unit === undefined) return null
  const count = Number(countText)
  return {
    from: `now-${count}${unit === "minute" ? "m" : unit.charAt(0)}`,
    label: `Last ${count} ${unit}${count === 1 ? "" : "s"}`,
  }
}
