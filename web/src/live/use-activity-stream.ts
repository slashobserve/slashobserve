import { useEffect, useRef, useSyncExternalStore } from "react"

import { afterClosed, backoffDelay } from "@/dashboards/live/reconnect"
import { closeWhileCached } from "@/lib/page-cache"

import type { ActivityEvent } from "./activity-match"

type ActivityStreamInput = {
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

function parseActivity(data: string): ActivityEvent | null {
  try {
    const event = JSON.parse(data) as ActivityEvent
    return typeof event?.target?.id === "string" ? event : null
  } catch {
    return null
  }
}

export function useActivityStream({
  enabled,
  onEvent,
  onGap,
}: ActivityStreamInput): void {
  const handlers = useRef({ onEvent, onGap })
  useEffect(() => {
    handlers.current = { onEvent, onGap }
  })
  const mayHaveMissed = useRef(false)

  useEffect(() => {
    if (!enabled) {
      mayHaveMissed.current = true
      return
    }
    let source: EventSource | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let gapPending = mayHaveMissed.current
    mayHaveMissed.current = true

    function open() {
      const stream = new EventSource(streamPath)
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
      stream.addEventListener("closed", (message) => {
        if (source !== stream) return
        if (afterClosed((message as MessageEvent<string>).data) !== "stop") {
          return
        }
        stream.close()
        source = undefined
        gapPending = true
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

    function close() {
      clearTimeout(retryTimer)
      source?.close()
      source = undefined
    }

    const stopWatchingCache = closeWhileCached(
      () => {
        if (source !== undefined) gapPending = true
        close()
      },
      () => {
        if (source === undefined) open()
      }
    )
    open()
    return () => {
      stopWatchingCache()
      close()
    }
  }, [enabled])
}
