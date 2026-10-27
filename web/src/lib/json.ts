export function stableJson(value: unknown): string {
  return JSON.stringify(value, (_key, item: unknown) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      return item
    }
    return Object.fromEntries(
      Object.entries(item).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    )
  })
}

export function sameJson(a: unknown, b: unknown): boolean {
  return stableJson(a) === stableJson(b)
}
