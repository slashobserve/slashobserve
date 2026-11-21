const longAnswerCharacters = 600

const fencePattern = /^\s*(```|~~~)/
const headingPattern = /^\s{0,3}#{1,6}(\s|$)/

export type AnswerFold = { opening: string; rest: string }

function paragraphs(markdown: string): string[] {
  const out: string[] = []
  let current: string[] = []
  let fenced = false
  for (const line of markdown.split("\n")) {
    if (fencePattern.test(line)) fenced = !fenced
    if (!fenced && line.trim() === "") {
      if (current.length > 0) out.push(current.join("\n"))
      current = []
      continue
    }
    current.push(line)
  }
  if (current.length > 0) out.push(current.join("\n"))
  return out
}

export function foldAnswer(markdown: string): AnswerFold | null {
  if (markdown.length <= longAnswerCharacters) return null
  const parts = paragraphs(markdown)
  let end = 0
  while (end < parts.length && headingPattern.test(parts[end] ?? "")) end++
  end = Math.min(end + 1, parts.length)
  if (end >= parts.length) return null
  return {
    opening: parts.slice(0, end).join("\n\n"),
    rest: parts.slice(end).join("\n\n"),
  }
}
