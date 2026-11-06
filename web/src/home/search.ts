import type { HomeSearch } from "@/api/keys"

const tagPrefix = "tag:"
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/
const maxTags = 10
const maxQueryLength = 200

export function parseHomeQuery(text: string): HomeSearch {
  const tag: string[] = []
  const words: string[] = []
  for (const word of text.split(/\s+/).filter(Boolean)) {
    const value = word.toLowerCase().startsWith(tagPrefix)
      ? word.slice(tagPrefix.length).toLowerCase()
      : undefined
    if (value === undefined || !tagPattern.test(value)) {
      words.push(word)
    } else if (!tag.includes(value) && tag.length < maxTags) {
      tag.push(value)
    }
  }
  const q = words.join(" ").slice(0, maxQueryLength).trim()
  return q === "" ? { tag } : { q, tag }
}

export function isEmptySearch(search: HomeSearch): boolean {
  return search.q === undefined && search.tag.length === 0
}

function matchRank(title: string, q: string): number {
  const lower = title.toLowerCase()
  if (lower.startsWith(q)) return 0
  if (lower.split(/[^a-z0-9]+/).some((word) => word.startsWith(q))) return 1
  if (lower.includes(q)) return 2
  return 3
}

export function rankResults<T extends { title: string }>(
  results: readonly T[],
  q: string | undefined
): T[] {
  if (q === undefined) return [...results]
  const needle = q.toLowerCase()
  return results
    .map((result, index) => ({
      result,
      index,
      rank: matchRank(result.title, needle),
    }))
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map(({ result }) => result)
}
