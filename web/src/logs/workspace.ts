import { createContext, useContext, useEffect, useState } from "react"
import { useQuery, type UseQueryResult } from "@tanstack/react-query"

import type { ExploreParams } from "@/api/keys"
import type { InputMode } from "@/ask/input-mode"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import type { DatasourceStatus } from "@/datasources/queries"
import { defaultPageSize, pagesByOffset, type PageSize } from "@/lib/paging"
import { usePageCursors } from "@/lib/use-page-cursors"
import type { LogSelection } from "@/panels/logs/logs-panel"

import { exploreQuery } from "./explore"
import type { LogsSearch } from "./search"
import type { useLiveTail } from "./use-live-tail"

export function logsPageParams({
  query,
  range,
  pageSize,
  page,
  cursor,
}: {
  query: string
  range: TimeRangeValue
  pageSize: PageSize
  page: number
  cursor: string | undefined
}): ExploreParams {
  return {
    query,
    kind: "instant",
    from: range.from,
    to: range.to,
    pageSize,
    page,
    ...(cursor !== undefined && { cursor }),
  }
}

export type LogsPaging = {
  page: number
  pageSize: PageSize
  lostPage: number | null
}

export function useLogsResults({
  datasource,
  query,
  search,
  range,
  update,
}: {
  datasource: Pick<DatasourceStatus, "uid" | "type">
  query: string | undefined
  search: LogsSearch
  range: TimeRangeValue
  update: (next: Partial<LogsSearch>, options?: { replace?: boolean }) => void
}): { results: UseQueryResult<PanelDataResponse>; paging: LogsPaging } {
  const byOffset = pagesByOffset(datasource.type)
  const pageSize = search.size ?? defaultPageSize
  const scope = JSON.stringify([
    datasource.uid,
    query,
    range.from,
    range.to,
    pageSize,
  ])
  const cursors = usePageCursors(scope, byOffset)
  const requested = search.page ?? 1
  const page = cursors.reachable(requested) ? requested : 1
  const results = useQuery({
    ...exploreQuery(
      datasource.uid,
      logsPageParams({
        query: query ?? "",
        range,
        pageSize,
        page,
        cursor: byOffset ? undefined : cursors.cursorFor(page),
      })
    ),
    enabled: query !== undefined,
  })
  const [lost, setLost] = useState<{ scope: string; page: number } | null>(null)
  if (page !== requested && lost?.scope !== scope) {
    setLost({ scope, page: requested })
  } else if (page === requested && requested !== 1 && lost !== null) {
    setLost(null)
  }
  const fresh =
    results.data !== undefined && !results.isPlaceholderData
      ? results.data
      : undefined
  const freshPage = fresh?.meta.page?.page
  const nextCursor = fresh?.meta.page?.next
  const { remember } = cursors

  useEffect(() => {
    if (page !== requested) update({ page: undefined }, { replace: true })
  }, [page, requested, update])

  useEffect(() => {
    if (freshPage !== undefined) remember(freshPage, nextCursor)
  }, [freshPage, nextCursor, remember])

  return {
    results,
    paging: {
      page,
      pageSize,
      lostPage: lost?.scope === scope ? lost.page : null,
    },
  }
}

export type DetailTab = "table" | "json" | "context"

type LogDetailState = {
  selection: LogSelection | null
  tab: DetailTab
  drawerId: string
  select: (selection: LogSelection | null, tab?: DetailTab) => void
  setTab: (tab: DetailTab) => void
}

export type LogsWorkspace = {
  datasource: DatasourceStatus
  datasources: DatasourceStatus[]
  search: LogsSearch
  effectiveQuery: string | undefined
  update: (next: Partial<LogsSearch>, options?: { replace?: boolean }) => void
  draft: string
  setDraft: (text: string) => void
  run: (text?: string, asked?: string) => void
  range: TimeRangeValue
  results: UseQueryResult<PanelDataResponse>
  paging: LogsPaging
  tail: ReturnType<typeof useLiveTail> & {
    on: boolean
    shown: boolean
    start: () => void
    stop: () => void
  }
  mode: InputMode
  setMode: (mode: InputMode, remember?: boolean) => void
  announce: (text: string) => void
  focusQuery: () => void
  registerQueryFocus: (focus: () => void) => void
  labels: { open: boolean; setOpen: (open: boolean) => void }
  detail: LogDetailState
}

export const LogsWorkspaceContext = createContext<LogsWorkspace | null>(null)

export function useLogsWorkspace(): LogsWorkspace {
  const workspace = useContext(LogsWorkspaceContext)
  if (workspace === null) {
    throw new Error("useLogsWorkspace needs a LogsWorkspaceContext provider")
  }
  return workspace
}
