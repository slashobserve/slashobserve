import { clampQuery } from "@/dashboards/list-filters"

export type ObservationStateFilter = "open" | "resolved" | "all"
type ObservationSort = "updated" | "created" | "title"
type ObservationOrder = "asc" | "desc"

export const pageSizes = [20, 50, 100] as const
type ObservationPageSize = (typeof pageSizes)[number]
const defaultPageSize = 20

const maxPage = 5000

export type ObservationsSearch = {
  state?: "resolved" | "all" | undefined
  q?: string | undefined
  mine?: "1" | undefined
  sort?: "created" | "title" | undefined
  order?: ObservationOrder | undefined
  page?: number | undefined
  size?: Exclude<ObservationPageSize, 20> | undefined
  new?: "1" | undefined
}

export type ObservationListView = {
  state: ObservationStateFilter
  q: string
  mine: boolean
  sort: ObservationSort
  order: ObservationOrder
  page: number
  size: ObservationPageSize
}

export const defaultView: ObservationListView = {
  state: "open",
  q: "",
  mine: false,
  sort: "updated",
  order: "desc",
  page: 1,
  size: defaultPageSize,
}

function defaultOrder(sort: ObservationSort): ObservationOrder {
  return sort === "title" ? "asc" : "desc"
}

function firstOf(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

function integerOf(value: unknown): number | undefined {
  const item = firstOf(value)
  if (typeof item === "number") return Number.isInteger(item) ? item : undefined
  return typeof item === "string" && /^\d+$/.test(item)
    ? Number(item)
    : undefined
}

export function validateObservationsSearch(
  raw: Record<string, unknown>
): ObservationsSearch {
  const state = firstOf(raw.state)
  const sort = firstOf(raw.sort)
  const order = firstOf(raw.order)
  const page = integerOf(raw.page)
  const size = integerOf(raw.size)
  const q = firstOf(raw.q)
  const typed = typeof q === "string" ? clampQuery(q.trim()) : ""
  const sortKey = sort === "created" || sort === "title" ? sort : undefined
  const orderKey = order === "asc" || order === "desc" ? order : undefined
  return {
    state: state === "resolved" || state === "all" ? state : undefined,
    q: typed === "" ? undefined : typed,
    mine: firstOf(raw.mine) === "1" ? "1" : undefined,
    sort: sortKey,
    order:
      orderKey === undefined || orderKey === defaultOrder(sortKey ?? "updated")
        ? undefined
        : orderKey,
    page: page !== undefined && page > 1 && page <= maxPage ? page : undefined,
    size: size === 50 || size === 100 ? size : undefined,
    new: firstOf(raw.new) === "1" ? "1" : undefined,
  }
}

export function viewOf(search: ObservationsSearch): ObservationListView {
  const sort = search.sort ?? defaultView.sort
  return {
    state: search.state ?? defaultView.state,
    q: search.q ?? defaultView.q,
    mine: search.mine === "1",
    sort,
    order: search.order ?? defaultOrder(sort),
    page: search.page ?? defaultView.page,
    size: search.size ?? defaultView.size,
  }
}

export function searchOf(
  view: ObservationListView,
  creating: boolean
): ObservationsSearch {
  return {
    ...(view.state === "open" ? {} : { state: view.state }),
    ...(view.q === "" ? {} : { q: view.q }),
    ...(view.mine ? { mine: "1" as const } : {}),
    ...(view.sort === "updated" ? {} : { sort: view.sort }),
    ...(view.order === defaultOrder(view.sort) ? {} : { order: view.order }),
    ...(view.page > 1 ? { page: view.page } : {}),
    ...(view.size === defaultPageSize ? {} : { size: view.size }),
    ...(creating ? { new: "1" as const } : {}),
  }
}

export function filtersApplied(view: ObservationListView): boolean {
  return view.state !== "open" || view.q !== "" || view.mine
}

export function pageCountOf(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size))
}

export function pageRange(
  page: number,
  size: number,
  total: number
): { first: number; last: number } {
  if (total === 0) return { first: 0, last: 0 }
  return {
    first: (page - 1) * size + 1,
    last: Math.min(page * size, total),
  }
}

export type SortChoice = {
  value: string
  label: string
  sort: ObservationSort
  order: ObservationOrder
}

export const sortChoices: readonly SortChoice[] = [
  {
    value: "updated:desc",
    label: "Recently changed",
    sort: "updated",
    order: "desc",
  },
  {
    value: "updated:asc",
    label: "Least recently changed",
    sort: "updated",
    order: "asc",
  },
  { value: "created:desc", label: "Newest", sort: "created", order: "desc" },
  { value: "created:asc", label: "Oldest", sort: "created", order: "asc" },
  { value: "title:asc", label: "Title A to Z", sort: "title", order: "asc" },
  { value: "title:desc", label: "Title Z to A", sort: "title", order: "desc" },
]

export function sortValue(view: ObservationListView): string {
  return `${view.sort}:${view.order}`
}
