import { useEffect, useRef, useState } from "react"

import { ApiError, toApiError } from "@/api/errors"
import { backoffDelay } from "@/dashboards/live/reconnect"
import type { WireFrame } from "@/dashboards/queries"
import { closeWhileCached } from "@/lib/page-cache"
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
const hiddenGraceMs = 15_000

function oldestKept(now: number): number {
  return now - tailWindowMs
}

export type TailConnection =
  "off" | "connecting" | "live" | "retrying" | "suspended" | "failed"

type Connection = Exclude<TailConnection, "off">

type Held = {
  session: number
  buffer: TailBuffer
  error: ApiError | null
  connection: Connection
}

function fresh(session: number, seed: readonly WireFrame[]): Held {
  const now = Date.now()
  return {
    session,
    buffer: startTail(seed, now, oldestKept(now)),
    error: null,
    connection: "connecting",
  }
}

export function useLiveTail({
  uid,
  query,
  seed,
  running,
  session,
  onFailed,
}: {
  uid: string
  query: string
  seed: WireFrame[]
  running: boolean
  session: number
  onFailed: () => void
}) {
  const [held, setHeld] = useState(() => fresh(session, seed))
  if (held.session !== session) setHeld(fresh(session, seed))
  const newest = useRef(held.buffer.newest)
  const reportFailed = useRef(onFailed)
  useEffect(() => {
    newest.current = held.buffer.newest
    reportFailed.current = onFailed
  })

  useEffect(() => {
    if (!running) return
    let source: EventSource | undefined
    let probe: AbortController | undefined
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let disposed = false
    let connection: Connection = "connecting"
    let hiddenTimer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let resumeFrom: string | undefined

    const update = (change: Partial<Held>) =>
      setHeld((current) =>
        current.session === session ? { ...current, ...change } : current
      )
    const enter = (next: Connection, change: Partial<Held> = {}) => {
      connection = next
      update({ ...change, connection: next })
    }
    const closed = () =>
      disposed || connection === "suspended" || connection === "failed"
    const fail = (error: ApiError) => {
      source?.close()
      source = undefined
      enter("failed", { error })
      reportFailed.current()
    }
    const retry = () => {
      if (closed()) return
      enter("retrying")
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
          retry()
          return
        }
        const body: unknown = await response.json().catch(() => null)
        if (closed()) return
        const reason = toApiError(response, body)
        fail(
          new ApiError(
            reason.status,
            reason.code,
            tailFailureText(reason.status, reason.code, reason.message)
          )
        )
      } catch {
        retry()
      }
    }

    function open() {
      retryTimer = undefined
      if (closed()) return
      const from = resumeFrom ?? tailStartFrom(newest.current, Date.now())
      const url = tailUrl(uid, query, from, requestZone("browser"))
      const stream = new EventSource(url)
      source = stream
      let ready = false
      stream.addEventListener("ready", () => {
        if (source !== stream) return
        ready = true
        attempt = 0
        enter("live")
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
        if (ready) retry()
        else void explain(url)
      })
    }

    function close() {
      clearTimeout(retryTimer)
      retryTimer = undefined
      probe?.abort()
      source?.close()
      source = undefined
    }

    function suspend() {
      if (connection === "failed") return
      enter("suspended")
      close()
    }

    function resume() {
      if (connection !== "suspended") return
      enter("connecting")
      open()
    }

    function onVisibilityChange() {
      if (document.hidden) {
        hiddenTimer ??= setTimeout(() => {
          hiddenTimer = undefined
          suspend()
        }, hiddenGraceMs)
        return
      }
      clearTimeout(hiddenTimer)
      hiddenTimer = undefined
      resume()
    }

    document.addEventListener("visibilitychange", onVisibilityChange)
    const stopWatchingCache = closeWhileCached(suspend, resume)
    open()
    if (document.hidden) onVisibilityChange()
    return () => {
      disposed = true
      document.removeEventListener("visibilitychange", onVisibilityChange)
      clearTimeout(hiddenTimer)
      stopWatchingCache()
      close()
    }
  }, [uid, query, running, session])

  const connection: TailConnection =
    running || held.connection === "failed" ? held.connection : "off"
  return {
    buffer: held.buffer,
    error: held.error,
    connection,
    isFetching: running && connection !== "live" && connection !== "failed",
  }
}
