import { useEffect, useRef, useState } from "react"

import { toApiError } from "@/api/errors"
import { backoffDelay } from "@/dashboards/live/reconnect"
import { readSse, type SseFrame } from "@/lib/sse"

import {
  type AuditEvent,
  auditStreamUrl,
  parseAuditFrame,
  notAStreamText,
  stopFrameText,
  streamRefusal,
} from "./stream"

type AuditStreamInput = {
  q: string
  enabled: boolean
  onEvent: (event: AuditEvent) => void
  onRestart: () => void
  onStop: (message: string) => void
}

const defaultRetryMs = 3_000

function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms)
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer)
        resolve()
      },
      { once: true }
    )
  })
}

export function useAuditStream({
  q,
  enabled,
  onEvent,
  onRestart,
  onStop,
}: AuditStreamInput): { connected: boolean } {
  const handlers = useRef({ onEvent, onRestart, onStop })
  useEffect(() => {
    handlers.current = { onEvent, onRestart, onStop }
  })
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    if (!enabled) return
    const url = auditStreamUrl(q)
    const controller = new AbortController()
    const { signal } = controller
    let attempt = 0
    let lastEventId = ""
    let restarted = false

    const stop = (message: string) => {
      controller.abort()
      setConnected(false)
      handlers.current.onStop(message)
    }

    function onFrame(frame: SseFrame) {
      switch (frame.event) {
        case "ready":
          attempt = 0
          setConnected(true)
          if (restarted) {
            restarted = false
            handlers.current.onRestart()
          }
          return
        case "audit": {
          const event = parseAuditFrame(frame.data)
          if (event !== null) handlers.current.onEvent(event)
          return
        }
        case "forbidden":
        case "error":
          stop(stopFrameText(frame.event, frame.data))
      }
    }

    async function follow() {
      while (!signal.aborted) {
        const read = await readSse(url, { lastEventId, signal, onFrame })
        lastEventId = read.lastEventId
        if (read.end.kind === "aborted") return
        setConnected(false)
        if (read.end.kind === "closed") {
          await pause(read.retryMs ?? defaultRetryMs, signal)
          continue
        }
        if (read.end.kind === "dropped") {
          await pause(backoffDelay(attempt++), signal)
          continue
        }
        if (read.end.kind === "not-a-stream") {
          stop(notAStreamText(read.end.contentType))
          return
        }
        const reason = toApiError(read.end.response, read.end.body)
        const refusal = streamRefusal(
          reason.status,
          reason.code,
          reason.message
        )
        if (refusal.kind === "stop") {
          stop(refusal.message)
          return
        }
        if (refusal.kind === "gap" && lastEventId !== "") {
          lastEventId = ""
          restarted = true
          continue
        }
        if (refusal.kind === "gap") {
          stop(`Live stopped: ${reason.message}`)
          return
        }
        await pause(backoffDelay(attempt++), signal)
      }
    }

    void follow()
    return () => {
      controller.abort()
      setConnected(false)
    }
  }, [q, enabled])

  return { connected: enabled && connected }
}
