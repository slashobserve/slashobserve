import type { Folder } from "@/folders/queries"
import { applyPills, decodePills } from "@/logs/pills"
import type { LogsSearch } from "@/logs/search"
import type { TraceSearch } from "@/traces/search"

import type { CreateSavedSearch } from "./queries"

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
      filters: columns.length === 0 ? undefined : { columns },
      from: range.from,
      to: range.to,
    },
  }
}

export function tracesDraft(input: {
  uid: string
  type: string
  search: TraceSearch
  range: { from: string; to: string }
}): SearchDraft {
  const { search, range } = input
  return {
    kind: "traces",
    datasource_uid: input.uid,
    query: input.type === "tempo" ? search.q : undefined,
    filters: {
      service: search.service,
      span: search.span,
      min: search.min,
      max: search.max,
      errors: search.errors === "1" ? true : undefined,
    },
    from: range.from,
    to: range.to,
  }
}
