import { useEffect, useRef, useState } from "react"

import { ApiError, toApiError } from "@/api/errors"
import { backoffDelay } from "@/dashboards/live/reconnect"
import type { WireFrame } from "@/dashboards/queries"
import { requestZone } from "@/lib/time"

import { startTail, mergeTail, type TailBuffer } from "./frames"
import {
  parseTailEvent,
  tailFailureText,
  tailStartFrom,
  tailUrl,
} from "./tail-stream"

export const tailRows = 1000
const tailWindowMs = 300_000
const streamErrorStatus = 502

function oldestKept(now: number): number {
  return now - tailWindowMs
}

type Held = {
  session: number
  buffer: TailBuffer
  error: ApiError | null
  connected: boolean
}

function fresh(session: number, seed: readonly WireFrame[]): Held {
  const now = Date.now()
  return {
    session,
    buffer: startTail(seed, now, oldestKept(now)),
    error: null,
    connected: false,
  }
}

export function useLiveTail({
  uid,
  query,
  seed,
  running,
  session,
}: {
  uid: string
  query: string
  seed: WireFrame[]
  running: boolean
  session: number
}) {
  const [held, setHeld] = useState(() => fresh(session, seed))
  if (held.session !== session) setHeld(fresh(session, seed))
  const newest = useRef(held.buffer.newest)
  useEffect(() => {
    newest.current = held.buffer.newest
  })

  useEffect(() => {
    if (!running) return
    let source: EventSource | undefined
    let probe: AbortController | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let disposed = false
    let attempt = 0
    let resumeFrom: string | undefined

    const update = (change: Partial<Held>) =>
      setHeld((current) =>
        current.session === session ? { ...current, ...change } : current
      )
    const fail = (error: ApiError) => {
      source?.close()
      source = undefined
      update({ error, connected: false })
    }
    const retry = () => {
      update({ connected: false })
      retryTimer = setTimeout(open, backoffDelay(attempt++))
    }

    async function explain(url: string) {
      const controller = new AbortController()
      probe = controller
      try {
        const response = await fetch(url, {
          headers: { accept: "text/event-stream" },
          signal: controller.signal,
        })
        if (response.ok) {
          controller.abort()
          if (!disposed) retry()
          return
        }
        const body: unknown = await response.json().catch(() => null)
        if (disposed) return
        const reason = toApiError(response, body)
        fail(
          new ApiError(
            reason.status,
            reason.code,
            tailFailureText(reason.status, reason.code, reason.message)
          )
        )
      } catch {
        if (!disposed) retry()
      }
    }

    function open() {
      retryTimer = undefined
      if (disposed) return
      const from = resumeFrom ?? tailStartFrom(newest.current, Date.now())
      const url = tailUrl(uid, query, from, requestZone("browser"))
      const stream = new EventSource(url)
      source = stream
      let ready = false
      stream.addEventListener("ready", () => {
        if (source !== stream) return
        ready = true
        attempt = 0
        update({ connected: true })
      })
      stream.addEventListener("lines", (message) => {
        if (source !== stream) return
        const event = parseTailEvent(
          "lines",
          (message as MessageEvent<string>).data
        )
        if (event?.type !== "lines") return
        resumeFrom = event.newest
        setHeld((current) =>
          current.session === session
            ? {
                ...current,
                buffer: mergeTail(
                  current.buffer,
                  event.frames,
                  tailRows,
                  oldestKept(Date.now())
                ),
              }
            : current
        )
      })
      stream.addEventListener("error", (event) => {
        if (source !== stream) return
        if (event instanceof MessageEvent) {
          const failure = parseTailEvent("error", String(event.data))
          if (failure?.type !== "error") return
          fail(new ApiError(streamErrorStatus, failure.code, failure.error))
          return
        }
        stream.close()
        source = undefined
        update({ connected: false })
        if (ready) retry()
        else void explain(url)
      })
    }

    open()
    return () => {
      disposed = true
      clearTimeout(retryTimer)
      probe?.abort()
      source?.close()
      source = undefined
    }
  }, [uid, query, running, session])

  return {
    buffer: held.buffer,
    error: held.error,
    isFetching: running && held.error === null && !held.connected,
  }
}
