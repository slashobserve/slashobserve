import type { DashboardListFilters } from "@/api/keys"
import { clientDisplayName } from "@/lib/origin"

export type ListSearch = { q?: string; made_by?: string; datasource?: string }

const uuid =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
const madeByPattern = new RegExp(
  `^(?:me|agent|mcp|mcp:(.+)|user:${uuid}|service_account:${uuid})$`,
  "su"
)
const maxClientLength = 64
const unsafeClientPattern = /[\p{Cc}\p{Cf}\p{Cs}]|^\s|\s$/u
const datasourcePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$/

export const maxQueryLength = 200

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

function firstString(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  return typeof item === "string" ? item : undefined
}

export function validateListSearch(raw: Record<string, unknown>): ListSearch {
  // The router merges the raw URL under a route's validated search, so a dropped key must be present as undefined to override it.
  const search: ListSearch = {
    q: undefined,
    made_by: undefined,
    datasource: undefined,
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
  return search
}

export function toFilters(search: ListSearch): DashboardListFilters {
  return {
    q: search.q,
    madeBy: search.made_by,
    datasource: search.datasource,
  }
}

export function hasFilters(search: ListSearch): boolean {
  return (
    search.q !== undefined ||
    search.made_by !== undefined ||
    search.datasource !== undefined
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
  if (value.startsWith("mcp:")) return clientDisplayName(value.slice(4))
  if (value.startsWith("user:")) {
    const id = value.slice(5)
    return users.find((user) => user.id === id)?.name ?? "a person"
  }
  return "a service account"
}

export function countText(n: number): string {
  return `${n} ${n === 1 ? "dashboard" : "dashboards"}`
}
