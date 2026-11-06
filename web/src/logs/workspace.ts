import { createContext, useContext } from "react"
import type { UseQueryResult } from "@tanstack/react-query"

import type { InputMode } from "@/ask/input-mode"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import type { DatasourceStatus } from "@/datasources/queries"

import type { ContextRequest } from "./logs-results"
import type { LogsSearch } from "./search"
import type { useLiveTail } from "./use-live-tail"

export const resultRows = 1000

export type LogsWorkspace = {
  datasource: DatasourceStatus
  datasources: DatasourceStatus[]
  search: LogsSearch
  update: (next: Partial<LogsSearch>, options?: { replace?: boolean }) => void
  draft: string
  setDraft: (text: string) => void
  run: (text?: string, asked?: string) => void
  range: TimeRangeValue
  results: UseQueryResult<PanelDataResponse>
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
  openContext: (request: ContextRequest) => void
}

export const LogsWorkspaceContext = createContext<LogsWorkspace | null>(null)

export function useLogsWorkspace(): LogsWorkspace {
  const workspace = useContext(LogsWorkspaceContext)
  if (workspace === null) {
    throw new Error("useLogsWorkspace needs a LogsWorkspaceContext provider")
  }
  return workspace
}
