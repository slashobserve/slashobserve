export const listPageSize = 50
export const maxListPage = 2000

export function pageCountOf(total: number): number {
  return Math.max(1, Math.ceil(total / listPageSize))
}

export function pageRange(
  page: number,
  total: number
): { first: number; last: number } {
  if (total === 0) return { first: 0, last: 0 }
  return {
    first: (page - 1) * listPageSize + 1,
    last: Math.min(page * listPageSize, total),
  }
}

export function pageOutOfRange(page: number, total: number): number | null {
  const last = pageCountOf(total)
  return page > last ? last : null
}

export function listOffset(page: number): number {
  return (page - 1) * listPageSize
}

export function withOffset<T extends object>(
  list: T,
  offset: number
): T & { offset: number } {
  return { ...list, offset }
}
