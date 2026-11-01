export const listPageSize = 50
export const maxListPage = 2000

export type PageItem = number | "start-ellipsis" | "end-ellipsis"

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

export function pageItems(page: number, pageCount: number): PageItem[] {
  if (pageCount <= 7) {
    return Array.from({ length: pageCount }, (_, i) => i + 1)
  }
  if (page <= 4) return [1, 2, 3, 4, 5, "end-ellipsis", pageCount]
  if (page >= pageCount - 3) {
    return [
      1,
      "start-ellipsis",
      pageCount - 4,
      pageCount - 3,
      pageCount - 2,
      pageCount - 1,
      pageCount,
    ]
  }
  return [
    1,
    "start-ellipsis",
    page - 1,
    page,
    page + 1,
    "end-ellipsis",
    pageCount,
  ]
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
