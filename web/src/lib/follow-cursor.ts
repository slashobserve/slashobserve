const maxPages = 10

export async function followCursor<T>(
  fetchPage: (
    cursor: string | undefined
  ) => Promise<{ items: T[]; next: string | null }>
): Promise<T[]> {
  const items: T[] = []
  let cursor: string | undefined
  for (let page = 0; page < maxPages; page++) {
    const result = await fetchPage(cursor)
    items.push(...result.items)
    if (result.next === null) break
    cursor = result.next
  }
  return items
}
