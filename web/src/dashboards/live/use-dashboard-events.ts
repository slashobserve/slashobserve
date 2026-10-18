import { useEffect, useRef, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useRouter } from "@tanstack/react-router"
import { toast } from "sonner"

import { dashboardKeys } from "@/api/keys"
import { refreshSession } from "@/lib/auth"

import type { DashboardEnvelope } from "../queries"
import { useSetDashboardSearch } from "../search"
import { parseEvent, type DashboardEvent } from "./events"

export type LiveStatus = "connecting" | "live" | "reconnecting"

const eventNames = [
  "ready",
  "resync",
  "version.created",
  "comment.created",
  "comment.updated",
  "revision.updated",
] as const

const hiddenGraceMs = 5 * 60_000
const minBackoffMs = 1_000
const maxBackoffMs = 30_000
const jitterShare = 0.2

function backoffDelay(attempt: number): number {
  const base = Math.min(minBackoffMs * 2 ** attempt, maxBackoffMs)
  return base * (1 + Math.random() * jitterShare)
}

function cachedLatestVersion(
  queryClient: QueryClient,
  dashboardId: string
): number | undefined {
  return queryClient.getQueryData<DashboardEnvelope>(
    dashboardKeys.latest(dashboardId)
  )?.version.version
}

function describeVersion(event: { summary: string; authorName: string }) {
  if (event.summary && event.authorName) {
    return `${event.summary} by ${event.authorName}`
  }
  return event.summary || (event.authorName ? `By ${event.authorName}` : "")
}

export function useDashboardEvents(
  dashboardId: string,
  { pinnedVersion }: { pinnedVersion?: number }
): { status: LiveStatus } {
  const queryClient = useQueryClient()
  const router = useRouter()
  const setSearch = useSetDashboardSearch()
  const [reported, setReported] = useState<{
    dashboardId: string
    status: LiveStatus
  }>({ dashboardId, status: "connecting" })

  const latest = useRef({ pinnedVersion, router, setSearch })
  useEffect(() => {
    latest.current = { pinnedVersion, router, setSearch }
  })

  useEffect(() => {
    const url = `/api/v1/dashboards/${encodeURIComponent(dashboardId)}/events`
    let source: EventSource | undefined
    let disposed = false
    let suspended = false
    let readyCount = 0
    let attempt = 0
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let hiddenTimer: ReturnType<typeof setTimeout> | undefined

    const report = (status: LiveStatus) => setReported({ dashboardId, status })
    const invalidate = (...keys: (readonly unknown[])[]) => {
      for (const queryKey of keys) {
        void queryClient.invalidateQueries({ queryKey })
      }
    }
    const refetchAll = () =>
      invalidate(
        dashboardKeys.latest(dashboardId),
        dashboardKeys.history(dashboardId),
        dashboardKeys.comments(dashboardId),
        dashboardKeys.revisions(dashboardId)
      )

    function onVersionCreated(
      event: Extract<DashboardEvent, { type: "version.created" }>
    ) {
      const cached = cachedLatestVersion(queryClient, dashboardId)
      if (cached !== undefined && event.version <= cached) return
      invalidate(
        dashboardKeys.latest(dashboardId),
        dashboardKeys.history(dashboardId),
        dashboardKeys.list()
      )
      const description = describeVersion(event) || undefined
      const pinned = latest.current.pinnedVersion
      const id = `version-${dashboardId}-${event.version}`
      if (pinned === undefined) {
        toast(`Updated to version ${event.version}`, { id, description })
        return
      }
      if (event.version <= pinned) return
      toast(`Version ${event.version} is available`, {
        id,
        description,
        action: {
          label: "View latest",
          onClick: () =>
            latest.current.setSearch((state) => ({
              ...state,
              version: undefined,
            })),
        },
      })
    }

    function handle(event: DashboardEvent) {
      switch (event.type) {
        case "ready": {
          attempt = 0
          readyCount++
          report("live")
          const cached = cachedLatestVersion(queryClient, dashboardId)
          const behind = cached !== undefined && cached < event.latestVersion
          if (behind) {
            invalidate(dashboardKeys.latest(dashboardId), dashboardKeys.list())
          }
          if (behind || readyCount > 1) {
            invalidate(dashboardKeys.history(dashboardId))
          }
          if (readyCount > 1) {
            invalidate(
              dashboardKeys.comments(dashboardId),
              dashboardKeys.revisions(dashboardId)
            )
          }
          return
        }
        case "resync":
          refetchAll()
          return
        case "version.created":
          onVersionCreated(event)
          return
        case "comment.created":
        case "comment.updated":
          invalidate(dashboardKeys.comments(dashboardId))
          return
        case "revision.updated":
          invalidate(dashboardKeys.revisions(dashboardId))
          return
      }
    }

    async function recover() {
      report("reconnecting")
      const authenticated = await refreshSession().then(
        ({ session }) => session.authenticated,
        () => true
      )
      if (disposed || suspended) return
      if (!authenticated) {
        void latest.current.router.invalidate()
        return
      }
      const delay = backoffDelay(attempt)
      attempt++
      retryTimer = setTimeout(() => {
        retryTimer = undefined
        open()
      }, delay)
    }

    function open() {
      if (disposed || suspended || source) return
      const stream = new EventSource(url)
      source = stream
      for (const name of eventNames) {
        stream.addEventListener(name, (message) => {
          if (source !== stream) return
          const event = parseEvent(name, (message as MessageEvent<string>).data)
          if (event) handle(event)
        })
      }
      stream.onerror = () => {
        if (source !== stream) return
        if (stream.readyState === EventSource.CONNECTING) {
          report("reconnecting")
          return
        }
        stream.close()
        source = undefined
        void recover()
      }
    }

    function suspend() {
      suspended = true
      clearTimeout(retryTimer)
      retryTimer = undefined
      source?.close()
      source = undefined
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
      if (suspended) {
        suspended = false
        attempt = 0
        report("connecting")
        open()
      }
    }

    document.addEventListener("visibilitychange", onVisibilityChange)
    if (document.hidden) onVisibilityChange()
    open()

    return () => {
      disposed = true
      document.removeEventListener("visibilitychange", onVisibilityChange)
      clearTimeout(retryTimer)
      clearTimeout(hiddenTimer)
      source?.close()
      source = undefined
    }
  }, [dashboardId, queryClient])

  return {
    status:
      reported.dashboardId === dashboardId ? reported.status : "connecting",
  }
}
