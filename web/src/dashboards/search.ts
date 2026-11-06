import { useCallback, useMemo } from "react"
import {
  useNavigate,
  useSearch,
  type HistoryState,
} from "@tanstack/react-router"

import { parseSearch, stringifySearch } from "@/lib/search"
import { isTimeValue, refreshOptions, refreshPattern } from "@/lib/time"
import type { CursorSync } from "@/panels/shared-cursor"

import { isCursorSync } from "./cursor-sync"

declare module "@tanstack/react-router" {
  interface HistoryState {
    focusVersionButtonAt?: number
  }
}

type EditView = "query" | "json"
type KioskCycle = "rows" | "folder"
export type InspectTab = "data" | "query" | "stats"

export type DashboardSearch = {
  from?: string
  to?: string
  refresh?: string
  cursor?: CursorSync
  version?: number
  panel?: string
  edit?: EditView
  share?: boolean
  ann?: string[]
  kiosk?: boolean
  cycle?: KioskCycle
  interval?: number
  viewPanel?: string
  inspect?: string
  inspectTab?: InspectTab
  chat?: string
  vars: Record<string, string[]>
}

export type DashboardSearchParams = Record<
  string,
  string | string[] | number | undefined
>

export const ALL = "$__all"

const panelIdPattern = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/
const requestIdPattern = /^[A-Za-z0-9_-]{1,64}$/
const variableNamePattern = /^[A-Za-z][A-Za-z0-9_]{0,39}$/
const versionPattern = /^[1-9][0-9]{0,8}$/
const annotationTagPattern = /^[a-z0-9][a-z0-9_.-]{0,39}$/
const maxAnnotationTags = 10
const intervalPattern = /^[0-9]{1,9}$/
export const cycleInterval = { min: 10, max: 3600, initial: 60 }

function isKioskCycle(value: string): value is KioskCycle {
  return value === "rows" || value === "folder"
}

function isInspectTab(value: string): value is InspectTab {
  return value === "data" || value === "query" || value === "stats"
}

export const variablePrefix = "var-"

function firstString(value: unknown): string | undefined {
  const item = Array.isArray(value) ? value[0] : value
  if (typeof item === "string") return item
  if (typeof item === "number") return String(item)
  return undefined
}

function stringList(value: unknown): string[] {
  const items = Array.isArray(value) ? value : [value]
  const out: string[] = []
  let empty = false
  for (const item of items) {
    const text = firstString(item)
    if (text === undefined) continue
    if (text === "") empty = true
    else out.push(text)
  }
  return out.length === 0 && empty ? [""] : out
}

export function parseAnnotationTags(raw: unknown): string[] | undefined {
  const text = firstString(raw)
  if (text === undefined) return undefined
  const tags: string[] = []
  for (const part of text.split(",")) {
    const tag = part.trim()
    if (!annotationTagPattern.test(tag) || tags.includes(tag)) continue
    tags.push(tag)
    if (tags.length === maxAnnotationTags) break
  }
  return tags.length > 0 ? tags : undefined
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

  const cursor = firstString(raw.cursor)
  if (cursor !== undefined && isCursorSync(cursor)) state.cursor = cursor

  const version = firstString(raw.version)
  if (version !== undefined && versionPattern.test(version)) {
    state.version = Number(version)
  }

  const panel = firstString(raw.panel)
  if (panel !== undefined && panelIdPattern.test(panel)) state.panel = panel

  if (state.panel !== undefined) {
    const edit = firstString(raw.edit)
    if (edit === "json") state.edit = "json"
    else if (edit === "query" || firstString(raw.tab) === "query") {
      state.edit = "query"
    }
  }

  if (state.edit === undefined && firstString(raw.share) === "1") {
    state.share = true
  }

  const ann = parseAnnotationTags(raw.ann)
  if (ann !== undefined) state.ann = ann

  const kiosk = firstString(raw.kiosk)
  if (kiosk === "" || kiosk === "1" || kiosk === "true") {
    state.kiosk = true
    const cycle = firstString(raw.cycle)
    if (cycle !== undefined && isKioskCycle(cycle)) {
      state.cycle = cycle
      const interval = firstString(raw.interval)
      if (interval !== undefined && intervalPattern.test(interval)) {
        state.interval = Math.min(
          Math.max(Number(interval), cycleInterval.min),
          cycleInterval.max
        )
      }
    }
  }

  if (state.edit === undefined) {
    const viewPanel = firstString(raw.viewPanel)
    if (viewPanel !== undefined && panelIdPattern.test(viewPanel)) {
      state.viewPanel = viewPanel
    }
    const inspect = firstString(raw.inspect)
    if (inspect !== undefined && panelIdPattern.test(inspect)) {
      state.inspect = inspect
      const tab = firstString(raw.inspectTab)
      if (tab !== undefined && isInspectTab(tab) && tab !== "data") {
        state.inspectTab = tab
      }
    }
  }

  const chat = firstString(raw.chat)
  if (chat !== undefined && requestIdPattern.test(chat)) state.chat = chat

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
  if (state.cursor !== undefined) out.cursor = state.cursor
  if (state.version !== undefined) out.version = state.version
  if (state.panel !== undefined) out.panel = state.panel
  if (state.edit !== undefined) out.edit = state.edit
  if (state.share && state.edit === undefined) out.share = "1"
  if (state.ann !== undefined && state.ann.length > 0) {
    out.ann = state.ann.join(",")
  }
  if (state.kiosk) {
    out.kiosk = "1"
    if (state.cycle !== undefined) {
      out.cycle = state.cycle
      if (
        state.interval !== undefined &&
        state.interval !== cycleInterval.initial
      ) {
        out.interval = state.interval
      }
    }
  }
  if (state.edit === undefined) {
    if (state.viewPanel !== undefined) out.viewPanel = state.viewPanel
    if (state.inspect !== undefined) {
      out.inspect = state.inspect
      if (state.inspectTab !== undefined && state.inspectTab !== "data") {
        out.inspectTab = state.inspectTab
      }
    }
  }
  if (state.chat !== undefined) out.chat = state.chat
  for (const [name, values] of Object.entries(state.vars)) {
    if (values.length > 0) out[`${variablePrefix}${name}`] = values
  }
  return out
}

export function panelEditorShowsNewerVersion(state: DashboardSearch): boolean {
  return (
    state.edit !== undefined &&
    state.panel !== undefined &&
    state.version === undefined
  )
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
    state.edit = undefined
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

export const dashboardRouteId = "/_authed/d/$dashboardId/{-$slug}"
const dashboardRoutePath = "/d/$dashboardId/{-$slug}"

export function useDashboardSearch(): DashboardSearch {
  const search = useSearch({ from: dashboardRouteId })
  return useMemo(() => validateDashboardSearch(search), [search])
}

export function useAnnotationTagFilter(): string[] | undefined {
  const joined = useSearch({
    strict: false,
    select: (search: Record<string, unknown>) =>
      parseAnnotationTags(search.ann)?.join(","),
  })
  return useMemo(() => joined?.split(","), [joined])
}

export function useSetDashboardSearch() {
  const navigate = useNavigate({ from: dashboardRoutePath })
  return useCallback(
    (
      update: (state: DashboardSearch) => DashboardSearch,
      options?: { replace?: boolean; state?: HistoryState }
    ) => {
      void navigate({
        search: (prev: DashboardSearchParams) =>
          searchFromState(update(validateDashboardSearch(prev))),
        replace: options?.replace ?? false,
        state: options?.state,
      })
    },
    [navigate]
  )
}
