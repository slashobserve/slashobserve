import type { DashboardListFilters } from "@/api/keys"
import { clientDisplayName } from "@/lib/origin"

export type ListSearch = { q?: string; made_by?: string; datasource?: string }

const madeByPattern =
  /^(me|agent|mcp|mcp:[^\s].{0,63}|user:[0-9a-f-]{36}|service_account:[0-9a-f-]{36})$/
const maxQueryLength = 200
const maxDatasourceLength = 64

function firstString(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  return typeof item === "string" ? item : undefined
}

export function validateListSearch(raw: Record<string, unknown>): ListSearch {
  const search: ListSearch = {}
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
  if (madeBy !== undefined && madeByPattern.test(madeBy)) {
    search.made_by = madeBy
  }
  const datasource = firstString(raw.datasource)
  if (
    datasource !== undefined &&
    datasource.length >= 1 &&
    datasource.length <= maxDatasourceLength
  ) {
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
