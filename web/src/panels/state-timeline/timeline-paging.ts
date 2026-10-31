export type Page<T> = { items: T[]; start: number; page: number; count: number }

export function pageCount(total: number, perPage: number | undefined): number {
  if (perPage === undefined || perPage < 1) return 1
  return Math.max(1, Math.ceil(total / perPage))
}

export function pageOf<T>(
  items: readonly T[],
  perPage: number | undefined,
  requested: number
): Page<T> {
  const count = pageCount(items.length, perPage)
  const page = Math.min(Math.max(0, requested), count - 1)
  if (perPage === undefined || perPage < 1) {
    return { items: [...items], start: 0, page, count }
  }
  const start = page * perPage
  return { items: items.slice(start, start + perPage), start, page, count }
}
