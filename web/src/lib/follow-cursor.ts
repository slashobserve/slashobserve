export async function followCursor<T>(
  fetchPage: (
    cursor: string | undefined
  ) => Promise<{ items: T[]; next: string | null }>,
  maxPages = 10
): Promise<T[]> {
  const items: T[] = []
  const seen = new Set<string>()
  let cursor: string | undefined
  for (let page = 0; page < maxPages; page++) {
    const result = await fetchPage(cursor)
    items.push(...result.items)
    if (result.next === null || seen.has(result.next)) break
    seen.add(result.next)
    cursor = result.next
  }
  return items
}
