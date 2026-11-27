export function uniqueById<P, T extends { id: string }>(
  pages: readonly P[] | undefined,
  pick: (page: P) => readonly T[]
): T[] {
  const seen = new Set<string>()
  const unique: T[] = []
  for (const page of pages ?? []) {
    for (const item of pick(page)) {
      if (seen.has(item.id)) continue
      seen.add(item.id)
      unique.push(item)
    }
  }
  return unique
}
