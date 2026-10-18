import { useCallback, useMemo } from "react"
import { useNavigate, useSearch } from "@tanstack/react-router"

import { parseSearch, stringifySearch } from "@/lib/search"
import { isTimeValue, refreshOptions, refreshPattern } from "@/lib/time"

export type DashboardSearch = {
  from?: string
  to?: string
  refresh?: string
  version?: number
  panel?: string
  tab?: "query" | "comments"
  vars: Record<string, string[]>
}

export type DashboardSearchParams = Record<
  string,
  string | string[] | number | undefined
>

export const ALL = "$__all"

const panelIdPattern = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/
const variableNamePattern = /^[A-Za-z][A-Za-z0-9_]{0,39}$/
const versionPattern = /^[1-9][0-9]{0,8}$/
const variablePrefix = "var-"

function firstString(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  if (typeof item === "string") return item
  if (typeof item === "number") return String(item)
  return undefined
}

function stringList(value: unknown): string[] {
  const items = Array.isArray(value) ? value : [value]
  const out: string[] = []
  for (const item of items) {
    const text = firstString(item)
    if (text !== undefined && text !== "") out.push(text)
  }
  return out
}

export function validateDashboardSearch(
  raw: Record<string, unknown>
): DashboardSearch {
  const state: DashboardSearch = { vars: {} }

  const from = firstString(raw.from)
  if (from !== undefined && isTimeValue(from)) state.from = from
  const to = firstString(raw.to)
  if (to !== undefined && isTimeValue(to)) state.to = to

  const refresh = firstString(raw.refresh)
  if (
    refresh !== undefined &&
    (refreshOptions.includes(refresh) || refreshPattern.test(refresh))
  ) {
    state.refresh = refresh
  }

  const version = firstString(raw.version)
  if (version !== undefined && versionPattern.test(version)) {
    state.version = Number(version)
  }

  const panel = firstString(raw.panel)
  if (panel !== undefined && panelIdPattern.test(panel)) state.panel = panel

  const tab = firstString(raw.tab)
  if (tab === "comments" || (tab === "query" && state.panel !== undefined)) {
    state.tab = tab
  }

  for (const key of Object.keys(raw)) {
    if (!key.startsWith(variablePrefix)) continue
    const name = key.slice(variablePrefix.length)
    if (!variableNamePattern.test(name)) continue
    const values = stringList(raw[key])
    if (values.length > 0) state.vars[name] = values
  }
  return state
}

export function searchFromState(
  state: DashboardSearch
): Record<string, string | string[] | number> {
  const out: Record<string, string | string[] | number> = {}
  if (state.from !== undefined) out.from = state.from
  if (state.to !== undefined) out.to = state.to
  if (state.refresh !== undefined) out.refresh = state.refresh
  if (state.version !== undefined) out.version = state.version
  if (state.panel !== undefined) out.panel = state.panel
  if (state.tab !== undefined) out.tab = state.tab
  for (const [name, values] of Object.entries(state.vars)) {
    if (values.length > 0) out[`${variablePrefix}${name}`] = values
  }
  return out
}

export function backFromSearch(state: DashboardSearch): string | undefined {
  const text = stringifySearch(searchFromState(state)).slice(1)
  return text === "" ? undefined : text
}

export function validateBack(value: unknown): string | undefined {
  const text = firstString(value)
  if (text === undefined) return undefined
  return backFromSearch(validateDashboardSearch(parseSearch(text)))
}

export function dashboardSearchFromBack(
  back: string | undefined,
  options?: { dropVersion?: boolean }
): DashboardSearchParams {
  if (back === undefined) return {}
  const state = validateDashboardSearch(parseSearch(back))
  if (options?.dropVersion) {
    state.version = undefined
    state.panel = undefined
    state.tab = undefined
  }
  return searchFromState(state)
}

export function withVariables<Q extends object>(
  query: Q,
  vars: Record<string, string[]>
): Q {
  const out = { ...query } as Record<string, unknown>
  for (const [name, values] of Object.entries(vars)) {
    out[`${variablePrefix}${name}`] = values
  }
  return out as Q
}

export const dashboardRouteId = "/_authed/dashboards/$dashboardId/"
const dashboardRoutePath = "/dashboards/$dashboardId/"

export function useDashboardSearch(): DashboardSearch {
  const search = useSearch({ from: dashboardRouteId })
  return useMemo(() => validateDashboardSearch(search), [search])
}

export function useSetDashboardSearch() {
  const navigate = useNavigate({ from: dashboardRoutePath })
  return useCallback(
    (
      update: (state: DashboardSearch) => DashboardSearch,
      options?: { replace?: boolean }
    ) => {
      void navigate({
        search: (prev: DashboardSearchParams) =>
          searchFromState(update(validateDashboardSearch(prev))),
        replace: options?.replace ?? false,
      })
    },
    [navigate]
  )
}
