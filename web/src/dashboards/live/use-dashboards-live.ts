import { useEffect, useRef } from "react"
import { useQueryClient, type QueryKey } from "@tanstack/react-query"

import { dashboardKeys } from "@/api/keys"
import {
  useActivityStream,
  useDocumentVisible,
} from "@/live/use-activity-stream"

const debounceMs = 1000

const listChangeTypes = [
  "dashboard.created",
  "dashboard.deleted",
  "dashboard.moved",
  "dashboard.version_created",
  "dashboard.restored",
] as const

function useLiveInvalidation(
  types: readonly string[],
  keys: readonly QueryKey[],
  dashboardId?: string
) {
  const queryClient = useQueryClient()
  const visible = useDocumentVisible()
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const keysRef = useRef(keys)
  useEffect(() => {
    keysRef.current = keys
  })
  useEffect(() => () => clearTimeout(timer.current), [])

  const refresh = () => {
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      for (const queryKey of keysRef.current) {
        void queryClient.invalidateQueries({ queryKey })
      }
    }, debounceMs)
  }
  useActivityStream({
    dashboardId,
    types,
    enabled: visible,
    onEvent: refresh,
    onGap: refresh,
  })
}

export function useDashboardListLive() {
  useLiveInvalidation(listChangeTypes, [
    dashboardKeys.list(),
    dashboardKeys.titles(),
  ])
}

export function useDashboardMovedLive(dashboardId: string) {
  useLiveInvalidation(
    ["dashboard.moved"],
    [dashboardKeys.latest(dashboardId), dashboardKeys.list()],
    dashboardId
  )
}
