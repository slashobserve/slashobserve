import { useEffect, useRef, useSyncExternalStore } from "react"

import { backoffDelay } from "@/dashboards/live/reconnect"

import type { ActivityEvent } from "./activity-match"

type ActivityStreamInput = {
  folderId?: string | null
  dashboardId?: string
  types?: readonly string[]
  enabled: boolean
  onEvent: (event: ActivityEvent) => void
  onGap: () => void
}

const streamPath = "/api/v1/activity/stream"

function subscribeToVisibility(notify: () => void) {
  document.addEventListener("visibilitychange", notify)
  return () => document.removeEventListener("visibilitychange", notify)
}

export function useDocumentVisible(): boolean {
  return useSyncExternalStore(
    subscribeToVisibility,
    () => !document.hidden,
    () => true
  )
}

function streamUrl(
  folderId: string | null | undefined,
  dashboardId: string | undefined,
  types: readonly string[]
): string {
  const params = new URLSearchParams()
  if (folderId) params.set("folder_id", folderId)
  if (dashboardId) params.set("dashboard_id", dashboardId)
  for (const type of types) params.append("type", type)
  const query = params.toString()
  return query === "" ? streamPath : `${streamPath}?${query}`
}

function parseActivity(data: string): ActivityEvent | null {
  try {
    const event = JSON.parse(data) as ActivityEvent
    return typeof event?.target?.id === "string" ? event : null
  } catch {
    return null
  }
}

export function useActivityStream({
  folderId,
  dashboardId,
  types,
  enabled,
  onEvent,
  onGap,
}: ActivityStreamInput): void {
  const handlers = useRef({ onEvent, onGap })
  useEffect(() => {
    handlers.current = { onEvent, onGap }
  })
  const opened = useRef(false)
  const typesKey = (types ?? []).join(",")

  useEffect(() => {
    if (!enabled) return
    const url = streamUrl(
      folderId,
      dashboardId,
      typesKey === "" ? [] : typesKey.split(",")
    )
    let source: EventSource | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let gapPending = opened.current
    opened.current = true

    function open() {
      const stream = new EventSource(url)
      source = stream
      stream.addEventListener("ready", () => {
        if (source !== stream) return
        attempt = 0
        if (gapPending) {
          gapPending = false
          handlers.current.onGap()
        }
      })
      stream.addEventListener("activity", (message) => {
        if (source !== stream) return
        const event = parseActivity((message as MessageEvent<string>).data)
        if (event !== null) handlers.current.onEvent(event)
      })
      stream.addEventListener("reset", () => {
        if (source !== stream) return
        handlers.current.onGap()
      })
      stream.onerror = () => {
        if (source !== stream || stream.readyState !== EventSource.CLOSED) {
          return
        }
        stream.close()
        source = undefined
        gapPending = true
        retryTimer = setTimeout(open, backoffDelay(attempt))
        attempt++
      }
    }

    open()
    return () => {
      clearTimeout(retryTimer)
      source?.close()
      source = undefined
    }
  }, [folderId, dashboardId, typesKey, enabled])
}
