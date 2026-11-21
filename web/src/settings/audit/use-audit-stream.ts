import { useEffect, useRef, useState } from "react"

import { toApiError } from "@/api/errors"
import { backoffDelay } from "@/dashboards/live/reconnect"

import {
  type AuditEvent,
  auditStreamUrl,
  parseAuditFrame,
  stopFrameText,
  streamRefusal,
} from "./stream"

type AuditStreamInput = {
  q: string
  enabled: boolean
  onEvent: (event: AuditEvent) => void
  onGap: () => void
  onStop: (message: string) => void
}

export function useAuditStream({
  q,
  enabled,
  onEvent,
  onGap,
  onStop,
}: AuditStreamInput): { connected: boolean } {
  const handlers = useRef({ onEvent, onGap, onStop })
  useEffect(() => {
    handlers.current = { onEvent, onGap, onStop }
  })
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    if (!enabled) return
    const url = auditStreamUrl(q)
    let source: EventSource | undefined
    let probe: AbortController | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let disposed = false
    let attempt = 0
    let lastId = ""
    let gapPending = false

    const stop = (message: string) => {
      source?.close()
      source = undefined
      setConnected(false)
      handlers.current.onStop(message)
    }
    const reopen = (delay: number) => {
      gapPending = true
      lastId = ""
      retryTimer = setTimeout(open, delay)
    }

    async function explain() {
      const controller = new AbortController()
      probe = controller
      const headers: Record<string, string> = { accept: "text/event-stream" }
      if (lastId !== "") headers["last-event-id"] = lastId
      try {
        const response = await fetch(url, {
          headers,
          signal: controller.signal,
        })
        if (response.ok) {
          controller.abort()
          if (!disposed) reopen(backoffDelay(attempt++))
          return
        }
        const body: unknown = await response.json().catch(() => null)
        if (disposed) return
        const reason = toApiError(response, body)
        const refusal = streamRefusal(
          reason.status,
          reason.code,
          reason.message
        )
        if (refusal.kind === "stop") stop(refusal.message)
        else reopen(refusal.kind === "gap" ? 0 : backoffDelay(attempt++))
      } catch {
        if (!disposed) reopen(backoffDelay(attempt++))
      }
    }

    function open() {
      retryTimer = undefined
      if (disposed) return
      const stream = new EventSource(url)
      source = stream
      stream.addEventListener("ready", () => {
        if (source !== stream) return
        attempt = 0
        setConnected(true)
        if (gapPending) {
          gapPending = false
          handlers.current.onGap()
        }
      })
      stream.addEventListener("audit", (message) => {
        if (source !== stream) return
        const frame = message as MessageEvent<string>
        const event = parseAuditFrame(frame.data)
        if (event === null) return
        if (frame.lastEventId !== "") lastId = frame.lastEventId
        handlers.current.onEvent(event)
      })
      stream.addEventListener("forbidden", (message) => {
        if (source !== stream) return
        stop(stopFrameText("forbidden", (message as MessageEvent<string>).data))
      })
      stream.addEventListener("error", (event) => {
        if (source !== stream) return
        if (event instanceof MessageEvent) {
          stop(stopFrameText("error", String(event.data)))
          return
        }
        setConnected(false)
        if (stream.readyState !== EventSource.CLOSED) return
        stream.close()
        source = undefined
        void explain()
      })
    }

    open()
    return () => {
      disposed = true
      clearTimeout(retryTimer)
      probe?.abort()
      source?.close()
      source = undefined
      setConnected(false)
    }
  }, [q, enabled])

  return { connected: enabled && connected }
}
