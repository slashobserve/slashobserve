import { firstString, firstText } from "@/lib/search"
import type { DashboardListFilters } from "@/api/keys"
import { auditDatasourceUid } from "@/audit/audit-source"
import {
  importedFilter,
  isProblemFilter,
  type ProblemFilter,
} from "@/health/health-labels"

import { maxListPage } from "./list-pages"

export const listSorts = ["updated", "name", "views"] as const

type ListSort = (typeof listSorts)[number]

export type ListSearch = {
  q?: string | undefined
  made_by?: string | undefined
  datasource?: string | undefined
  tag?: string[] | undefined
  folder?: string | undefined
  kind?: ProblemFilter | undefined
  sort?: ListSort | undefined
  page?: number | undefined
}

const uuid =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
const folderPattern = new RegExp(`^${uuid}$`)
const madeByPattern = new RegExp(
  `^(?:me|agent|mcp|mcp:(.+)|user:${uuid}|service_account:${uuid})$`,
  "su"
)
const maxClientLength = 64
const unsafeClientPattern = /[\p{Cc}\p{Cf}\p{Cs}]|^\s|\s$/u
const datasourcePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/
const tagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/
const pagePattern = /^[1-9][0-9]{0,3}$/

export const maxQueryLength = 200
export const maxTags = 10

function validMadeBy(value: string): boolean {
  const match = madeByPattern.exec(value)
  if (match === null) return false
  const client = match[1]
  if (client === undefined) return true
  return (
    Array.from(client).length <= maxClientLength &&
    !unsafeClientPattern.test(client)
  )
}

export function clampQuery(text: string): string {
  const points = Array.from(text)
  return points.length <= maxQueryLength
    ? text
    : points.slice(0, maxQueryLength).join("")
}

function validTags(value: unknown): string[] | undefined {
  const items = Array.isArray(value) ? value : [value]
  const tags = new Set<string>()
  for (const item of items) {
    if (typeof item === "string" && tagPattern.test(item)) tags.add(item)
  }
  if (tags.size === 0) return undefined
  return [...tags].sort().slice(0, maxTags)
}

export function validateListSearch(raw: Record<string, unknown>): ListSearch {
  // The router merges the raw URL under a route's validated search, so a dropped key must be present as undefined to override it.
  const search: ListSearch = {
    q: undefined,
    made_by: undefined,
    datasource: undefined,
    tag: undefined,
    folder: undefined,
    kind: undefined,
    sort: undefined,
    page: undefined,
  }
  const q = firstString(raw.q)?.trim()
  if (
    q !== undefined &&
    q !== "" &&
    Array.from(q).length <= maxQueryLength &&
    !q.includes("\0")
  ) {
    search.q = q
  }
  const madeBy = firstString(raw.made_by)
  if (madeBy !== undefined && validMadeBy(madeBy)) {
    search.made_by = /^(?:user|service_account):/.test(madeBy)
      ? madeBy.toLowerCase()
      : madeBy
  }
  const datasource = firstString(raw.datasource)
  if (datasource !== undefined && datasourcePattern.test(datasource)) {
    search.datasource = datasource
  }
  search.tag = validTags(raw.tag)
  const folder = firstString(raw.folder)
  if (folder !== undefined && folderPattern.test(folder)) {
    search.folder = folder.toLowerCase()
  }
  const kind = firstString(raw.kind)
  if (isProblemFilter(kind)) search.kind = kind
  const sort = firstString(raw.sort)
  search.sort = listSorts.find((known) => known === sort)
  const page = firstText(raw.page)
  if (page !== undefined && pagePattern.test(page)) {
    const n = Number(page)
    if (n >= 2 && n <= maxListPage) search.page = n
  }
  return search
}

export function pageSearch(search: ListSearch, page: number): ListSearch {
  return { ...search, page: page === 1 ? undefined : page }
}

export function withoutPage(search: ListSearch): ListSearch {
  return { ...search, page: undefined }
}

export function toFilters(search: ListSearch): DashboardListFilters {
  const problem = search.kind === importedFilter ? undefined : search.kind
  return {
    ...(search.q === undefined ? {} : { q: search.q }),
    ...(search.made_by === undefined ? {} : { madeBy: search.made_by }),
    ...(search.datasource === undefined
      ? {}
      : { datasource: search.datasource }),
    ...(search.tag === undefined ? {} : { tags: search.tag }),
    ...(search.folder === undefined ? {} : { folder: search.folder }),
    ...(problem === undefined ? {} : { problem }),
    ...(search.sort === undefined ? {} : { order: search.sort }),
  }
}

export function leavesOutAudit(
  search: ListSearch,
  folderLocked: boolean
): boolean {
  return !folderLocked && search.folder === undefined
}

export function foldersToBrowse<T extends { id: string; audit: boolean }>(
  folders: readonly T[],
  chosen: string | undefined
): T[] {
  return folders.filter((folder) => !folder.audit || folder.id === chosen)
}

export function datasourcesToBrowse<T extends { uid: string }>(
  datasources: readonly T[],
  chosen: string | undefined
): T[] {
  return datasources.filter(
    (datasource) =>
      datasource.uid !== auditDatasourceUid || datasource.uid === chosen
  )
}

export function hasFilters(search: ListSearch): boolean {
  return (
    search.q !== undefined ||
    search.made_by !== undefined ||
    search.datasource !== undefined ||
    search.tag !== undefined ||
    search.folder !== undefined ||
    search.kind !== undefined
  )
}

export function madeByText(
  value: string | undefined,
  users: readonly { id: string; name: string }[]
): string {
  if (value === undefined) return "anyone"
  if (value === "me") return "me"
  if (value === "agent") return "Agent"
  if (value === "mcp") return "MCP clients"
  if (value.startsWith("mcp:")) return value.slice(4)
  if (value.startsWith("user:")) {
    const id = value.slice(5)
    return users.find((user) => user.id === id)?.name ?? "a person"
  }
  return "a service account"
}

export function tagsText(tags: string[] | undefined): string {
  const first = tags?.[0]
  if (tags === undefined || first === undefined) return "any"
  if (tags.length === 1) return first
  return `${first} +${tags.length - 1}`
}

export function folderText(
  value: string | undefined,
  folders: readonly { id: string; title: string }[]
): string {
  if (value === undefined) return "any"
  return folders.find((folder) => folder.id === value)?.title ?? "a folder"
}
