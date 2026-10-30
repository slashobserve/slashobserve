import { useEffect, useRef, useState } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useRouter } from "@tanstack/react-router"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { refreshSession } from "@/lib/auth"

import { latestDashboardQuery, type DashboardEnvelope } from "../queries"
import {
  dashboardRouteId,
  searchFromState,
  validateDashboardSearch,
} from "../search"
import { parseEvent, type DashboardEvent } from "./events"
import {
  backoffDelay,
  reconnectingGraceMs,
  shouldCheckDashboard,
} from "./reconnect"

export type LiveStatus = "connecting" | "live" | "reconnecting"

function versionToastId(dashboardId: string, version: number): string {
  return `version-${dashboardId}-${version}`
}

const eventNames = [
  "ready",
  "resync",
  "version.created",
  "dashboard.deleted",
  "comment.created",
  "comment.updated",
  "revision.updated",
] as const

const hiddenGraceMs = 5 * 60_000

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
  {
    pinnedVersion,
    toastVersions = true,
  }: { pinnedVersion?: number; toastVersions?: boolean }
): { status: LiveStatus } {
  const queryClient = useQueryClient()
  const router = useRouter()
  const [reported, setReported] = useState<{
    dashboardId: string
    status: LiveStatus
  }>({ dashboardId, status: "connecting" })

  const latest = useRef({ pinnedVersion, toastVersions, router })
  useEffect(() => {
    latest.current = { pinnedVersion, toastVersions, router }
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
    let reconnectingTimer: ReturnType<typeof setTimeout> | undefined
    let current: LiveStatus = "connecting"
    const toastIds = new Set<string>()

    const report = (status: LiveStatus) => {
      clearTimeout(reconnectingTimer)
      reconnectingTimer = undefined
      current = status
      setReported({ dashboardId, status })
    }
    const reportReconnecting = () => {
      if (current === "reconnecting" || reconnectingTimer !== undefined) return
      reconnectingTimer = setTimeout(
        () => report("reconnecting"),
        reconnectingGraceMs
      )
    }
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
      const saving = queryClient.isMutating({
        mutationKey: dashboardKeys.saves(dashboardId),
      })
      if (saving > 0 || !latest.current.toastVersions) return
      const description = describeVersion(event) || undefined
      const pinned = latest.current.pinnedVersion
      const id = versionToastId(dashboardId, event.version)
      if (pinned === undefined) {
        toastIds.add(id)
        toast(`Updated to version ${event.version}`, { id, description })
        return
      }
      if (event.version <= pinned) return
      toastIds.add(id)
      toast(`Version ${event.version} is available`, {
        id,
        description,
        action: {
          label: "View latest",
          onClick: viewLatest,
        },
      })
    }

    function viewLatest() {
      const router = latest.current.router
      const here = router.state.matches.some(
        (match) =>
          match.routeId === dashboardRouteId &&
          match.params.dashboardId === dashboardId
      )
      void router.navigate({
        to: "/dashboards/$dashboardId",
        params: { dashboardId },
        search: (previous: Record<string, unknown>) =>
          here
            ? searchFromState({
                ...validateDashboardSearch(previous),
                version: undefined,
              })
            : {},
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
        case "dashboard.deleted":
          if (
            queryClient.isMutating({
              mutationKey: dashboardKeys.deletes(dashboardId),
            }) > 0
          ) {
            return
          }
          toast(
            event.authorName
              ? `${event.authorName} deleted this dashboard`
              : "This dashboard was deleted",
            { id: `deleted-${dashboardId}` }
          )
          void checkDashboard()
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

    async function checkDashboard() {
      try {
        await queryClient.fetchQuery({
          ...latestDashboardQuery(queryClient, dashboardId),
          staleTime: 0,
          retry: false,
        })
      } catch (error) {
        if (disposed) return
        if (!(error instanceof ApiError) || error.status !== 404) return
        queryClient.removeQueries({
          queryKey: dashboardKeys.latest(dashboardId),
          exact: true,
        })
        void latest.current.router.invalidate()
      }
    }

    async function recover() {
      reportReconnecting()
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
      if (shouldCheckDashboard(attempt)) void checkDashboard()
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
          reportReconnecting()
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
      report("connecting")
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
      clearTimeout(reconnectingTimer)
      source?.close()
      source = undefined
      for (const id of toastIds) toast.dismiss(id)
    }
  }, [dashboardId, queryClient])

  return {
    status:
      reported.dashboardId === dashboardId ? reported.status : "connecting",
  }
}
