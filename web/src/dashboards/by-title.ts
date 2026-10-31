import { api } from "@/api/client"
import { unwrap } from "@/api/errors"

import { maxQueryLength } from "./list-filters"
import {
  searchFromState,
  validateDashboardSearch,
  type DashboardSearchParams,
} from "./search"

export type ByTitleSearch = DashboardSearchParams & { title?: string }

export type ByTitleTarget =
  | { kind: "dashboard"; id: string }
  | { kind: "list"; title: string }
  | { kind: "none" }

type Candidate = { id: string; title: string }

export function validateByTitleSearch(
  raw: Record<string, unknown>
): ByTitleSearch {
  const { from, to, vars } = validateDashboardSearch(raw)
  const carried = searchFromState({ from, to, vars })
  const title = Array.isArray(raw.title) ? raw.title[0] : raw.title
  if (typeof title !== "string") return carried
  const trimmed = title.trim()
  const length = Array.from(trimmed).length
  if (length === 0 || length > maxQueryLength || trimmed.includes("\0")) {
    return carried
  }
  return { ...carried, title: trimmed }
}

export function pickDashboard(
  title: string,
  candidates: Candidate[]
): string | undefined {
  const exact = candidates.filter((candidate) => candidate.title === title)
  if (exact.length > 0) return exact.length === 1 ? exact[0].id : undefined
  const lower = title.toLowerCase()
  const folded = candidates.filter(
    (candidate) => candidate.title.toLowerCase() === lower
  )
  return folded.length === 1 ? folded[0].id : undefined
}

export function targetFor(
  title: string | undefined,
  candidates: Candidate[]
): ByTitleTarget {
  if (title === undefined) return { kind: "none" }
  const id = pickDashboard(title, candidates)
  return id === undefined ? { kind: "list", title } : { kind: "dashboard", id }
}

export function findByTitle(
  title: string,
  signal?: AbortSignal
): Promise<Candidate[]> {
  return api
    .GET("/dashboards", {
      params: { query: { q: title, limit: 200 } },
      signal,
    })
    .then(unwrap)
    .then((page) => page.dashboards)
}
