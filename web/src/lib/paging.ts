import type { components } from "@/api/schema.gen"

export type PageMeta = components["schemas"]["PageMeta"]

export type PageItem = number | "start-ellipsis" | "end-ellipsis"

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

export const pageSizes = [50, 100, 250, 500] as const
export type PageSize = (typeof pageSizes)[number]
export const defaultPageSize: PageSize = 50

const maxPage = 100_000

export type PageState = {
  page: number
  pageSize: PageSize
  rows: number
  total?: number
  totalRelation: "eq" | "gte" | "unknown"
  hasNext: boolean
  hasPrev: boolean
  byOffset: boolean
  window?: number
  next?: string
  prev?: string
}

function integerOf(value: unknown): number | undefined {
  const item = Array.isArray(value) ? value[0] : value
  if (typeof item === "number") {
    return Number.isInteger(item) ? item : undefined
  }
  return typeof item === "string" && /^\d+$/.test(item)
    ? Number(item)
    : undefined
}

export function validPageSize(value: unknown): PageSize | undefined {
  const size = integerOf(value)
  return pageSizes.find((candidate) => candidate === size)
}

export function validPage(value: unknown): number | undefined {
  const page = integerOf(value)
  return page !== undefined && page >= 1 && page <= maxPage ? page : undefined
}

export function pagesByOffset(type: string): boolean {
  return (
    type === "clickhouse" || type === "elasticsearch" || type === "opensearch"
  )
}

export function pageStateOf(
  meta: PageMeta | undefined,
  rows: number,
  byOffset: boolean,
  fallback: { page: number; pageSize: PageSize; truncated?: boolean }
): PageState {
  if (meta === undefined) {
    return {
      page: fallback.page,
      pageSize: fallback.pageSize,
      rows,
      totalRelation: "unknown",
      hasNext: fallback.truncated === true,
      hasPrev: fallback.page > 1,
      byOffset,
    }
  }
  return {
    page: meta.page,
    pageSize: validPageSize(meta.page_size) ?? fallback.pageSize,
    rows,
    total: meta.total_relation === "unknown" ? undefined : meta.total,
    totalRelation: meta.total_relation,
    hasNext: meta.has_next,
    hasPrev: meta.has_prev,
    byOffset,
    window: meta.window,
    next: meta.next,
    prev: meta.prev,
  }
}

export function pageCountOf(state: PageState): number | null {
  if (!state.byOffset || state.total === undefined) return null
  const reachable = Math.min(state.total, state.window ?? state.total)
  return Math.max(1, Math.ceil(reachable / state.pageSize))
}

export function windowNote(state: PageState, storeName: string): string | null {
  const count = pageCountOf(state)
  if (
    count === null ||
    state.window === undefined ||
    state.total === undefined ||
    state.total <= state.window
  ) {
    return null
  }
  return `Pages stop at ${count.toLocaleString("en")}, the first ${state.window.toLocaleString("en")} documents ${storeName} will page through. Narrow the time range to reach older ones.`
}

export function rangeText(
  state: PageState,
  noun: string,
  qualifier = " in range"
): string {
  if (state.rows === 0) return `No ${noun}${qualifier}`
  const first = (state.page - 1) * state.pageSize + 1
  const last = first + state.rows - 1
  const span = `${first.toLocaleString("en")}-${last.toLocaleString("en")}`
  if (state.total === undefined || state.totalRelation === "unknown") {
    return `${span} ${noun}${qualifier}`
  }
  const of = state.totalRelation === "gte" ? "of at least" : "of"
  return `${span} ${of} ${state.total.toLocaleString("en")} ${noun}${qualifier}`
}
