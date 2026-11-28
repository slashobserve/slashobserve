import type { Folder } from "@/folders/queries"
import { applyPills, decodePills } from "@/logs/pills"
import type { LogsSearch } from "@/logs/search"
import type { MetricsSearch } from "@/metrics/search"
import type { TraceSearch } from "@/traces/search"

import type { CreateSavedSearch, SavedSearch } from "./queries"

export type SearchDraft = Omit<CreateSavedSearch, "title" | "folder_id">

const titleCodePoints = 80

export function defaultTitle(draft: SearchDraft): string {
  const source =
    draft.query ??
    [draft.filters?.service, draft.filters?.span]
      .filter((part) => part !== undefined)
      .join(" ")
  const flat = source.replace(/[\p{Cc}\s]+/gu, " ").trim()
  return [...flat].slice(0, titleCodePoints).join("").trimEnd()
}

export function matchesSaved(
  draft: SearchDraft,
  saved: Pick<SavedSearch, "query" | "filters" | "from" | "to">
): boolean {
  return (
    (draft.query ?? null) === saved.query &&
    draft.from === saved.from &&
    draft.to === saved.to &&
    filterKey(draft.filters) === filterKey(saved.filters)
  )
}

function filterKey(filters: SearchDraft["filters"] | SavedSearch["filters"]) {
  return JSON.stringify([
    (filters?.columns ?? []).join(","),
    filters?.service ?? "",
    filters?.span ?? "",
    filters?.min ?? "",
    filters?.max ?? "",
    filters?.errors === true,
    filters?.mode === "table",
  ])
}

export function saveTargets(folders: readonly Folder[]): {
  choices: Folder[]
  initial: string | null
} {
  const choices = folders.filter((folder) => folder.permissions.create)
  const initial =
    choices.find((folder) => folder.personal) ??
    choices.find((folder) => folder.general) ??
    choices[0]
  return { choices, initial: initial?.id ?? null }
}

export const noSaveFolderText =
  "You cannot create in any folder, so there is nowhere to save a search."

export function saveBlockedText(
  folders: readonly Folder[] | undefined
): string | undefined {
  return folders !== undefined && saveTargets(folders).choices.length === 0
    ? noSaveFolderText
    : undefined
}

export type LogsDraft =
  { draft: SearchDraft } | { blocked: "no-query" | "pills" }

export function logsDraft(input: {
  uid: string
  type: string
  search: LogsSearch
  range: { from: string; to: string }
}): LogsDraft {
  const { search, range } = input
  if (search.q === undefined) return { blocked: "no-query" }
  const query = applyPills(search.q, input.type, decodePills(search.f))
  if (query === null) return { blocked: "pills" }
  const columns = search.cols === undefined ? [] : search.cols.split(",")
  return {
    draft: {
      kind: "logs",
      datasource_uid: input.uid,
      query,
      ...(columns.length === 0 ? {} : { filters: { columns } }),
      from: range.from,
      to: range.to,
    },
  }
}

export function tracesDraft(input: {
  uid: string
  search: TraceSearch
  range: { from: string; to: string }
}): SearchDraft {
  const { search, range } = input
  return {
    kind: "traces",
    datasource_uid: input.uid,
    ...(search.q === undefined ? {} : { query: search.q }),
    filters: {
      ...(search.service === undefined ? {} : { service: search.service }),
      ...(search.span === undefined ? {} : { span: search.span }),
      ...(search.min === undefined ? {} : { min: search.min }),
      ...(search.max === undefined ? {} : { max: search.max }),
      ...(search.errors === "1" ? { errors: true } : {}),
    },
    from: range.from,
    to: range.to,
  }
}

export type MetricsDraft = { draft: SearchDraft } | { blocked: "no-query" }

export function metricsDraft(input: {
  uid: string
  search: MetricsSearch
  range: { from: string; to: string }
}): MetricsDraft {
  const { search, range } = input
  if (search.q === undefined) return { blocked: "no-query" }
  return {
    draft: {
      kind: "metrics",
      datasource_uid: input.uid,
      query: search.q,
      ...(search.mode === "table" ? { filters: { mode: "table" } } : {}),
      from: range.from,
      to: range.to,
    },
  }
}
