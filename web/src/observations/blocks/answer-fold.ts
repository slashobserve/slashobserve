const longAnswerCharacters = 600

const fencePattern = /^\s{0,3}(`{3,}|~{3,})/
const headingPattern = /^\s{0,3}#{1,6}(\s|$)/
const listItemPattern = /^\s{0,3}([-*+]|\d{1,9}[.)])(\s|$)/
const continuationPattern = /^( {2,}|\t)\S/
const definitionPattern = /^\s{0,3}\[[^\]]+\]:\s*\S/

export type AnswerFold = { opening: string; rest: string }

function blocks(markdown: string): string[] {
  const out: string[] = []
  let current: string[] = []
  let fence: string | null = null
  for (const line of markdown.split("\n")) {
    const marker = fencePattern.exec(line)?.[1]?.[0]
    if (marker !== undefined && (fence === null || fence === marker)) {
      fence = fence === null ? marker : null
    }
    if (fence === null && line.trim() === "") {
      if (current.length > 0) out.push(current.join("\n"))
      current = []
      continue
    }
    current.push(line)
  }
  if (current.length > 0) out.push(current.join("\n"))
  return out
}

function continuesList(previous: string, next: string): boolean {
  return (
    listItemPattern.test(previous) &&
    (listItemPattern.test(next) || continuationPattern.test(next))
  )
}

function joinLists(parts: string[]): string[] {
  const out: string[] = []
  for (const part of parts) {
    const previous = out[out.length - 1]
    if (previous !== undefined && continuesList(previous, part)) {
      out[out.length - 1] = `${previous}\n\n${part}`
    } else {
      out.push(part)
    }
  }
  return out
}

function isDefinitions(part: string): boolean {
  return part.split("\n").every((line) => definitionPattern.test(line))
}

export function foldAnswer(markdown: string): AnswerFold | null {
  if (markdown.length <= longAnswerCharacters) return null
  const all = joinLists(blocks(markdown))
  const definitions = all.filter(isDefinitions)
  const parts = all.filter((part) => !isDefinitions(part))
  let end = 0
  while (end < parts.length && headingPattern.test(parts[end] ?? "")) end++
  end = Math.min(end + 1, parts.length)
  if (end >= parts.length) return null
  return {
    opening: [...parts.slice(0, end), ...definitions].join("\n\n"),
    rest: [...parts.slice(end), ...definitions].join("\n\n"),
  }
}
