const parseError =
  /^(?:invalid query: )?(?:invalid TraceQL query: )?parse error at line (\d+), col (\d+): (?:syntax error: )?(.*)$/s

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
  const match = parseError.exec(message)
  if (match === null) return message.replace(doubledPrefix, "$1")
  const line = Number(match[1])
  const column = Number(match[2])
  const text = before(query, line, column)
  const where = `line ${line}, column ${column}`
  const after = text === "" ? "" : ` after "${text}"`
  return `Invalid TraceQL at ${where}${after}: ${readable(match[3])}.`
}
