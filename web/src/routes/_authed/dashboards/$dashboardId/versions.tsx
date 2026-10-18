import { useCallback, useEffect, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, createFileRoute } from "@tanstack/react-router"
import { ArrowLeftIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { DashboardNotFound } from "@/dashboards/not-found"
import { useDashboardEvents } from "@/dashboards/live/use-dashboard-events"
import { latestDashboardQuery } from "@/dashboards/queries"
import { dashboardSearchFromBack, validateBack } from "@/dashboards/search"
import { useKnownLatestVersion } from "@/history/use-version-history"
import { VersionDiff } from "@/history/version-diff"
import { VersionList } from "@/history/version-list"

type HistorySearch = { from?: number; to?: number; back?: string }

const versionNumber = /^(0|[1-9][0-9]{0,8})$/

function versionParam(value: unknown): number | undefined {
  const item = Array.isArray(value) ? value[0] : value
  if (typeof item !== "string" && typeof item !== "number") return undefined
  const text = String(item)
  return versionNumber.test(text) ? Number(text) : undefined
}

type FocusRequest = { from: number; to: number }

export const Route = createFileRoute(
  "/_authed/dashboards/$dashboardId/versions"
)({
  validateSearch: (raw: Record<string, unknown>): HistorySearch => {
    const search: HistorySearch = {}
    const from = versionParam(raw.from)
    if (from !== undefined) search.from = from
    const to = versionParam(raw.to)
    if (to !== undefined) search.to = to
    const back = validateBack(raw.back)
    if (back !== undefined) search.back = back
    return search
  },
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(
      latestDashboardQuery(context.queryClient, params.dashboardId)
    )
  },
  errorComponent: ({ error }) => {
    if (
      error instanceof ApiError &&
      (error.status === 404 || error.status === 400)
    ) {
      return <DashboardNotFound />
    }
    throw error
  },
  component: HistoryRoute,
})

function HistoryRoute() {
  const { dashboardId } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const queryClient = useQueryClient()
  const latest = useQuery(latestDashboardQuery(queryClient, dashboardId))
  const latestVersion = useKnownLatestVersion(dashboardId)
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null)
  useDashboardEvents(dashboardId, { toastVersions: false })
  const title = latest.data?.dashboard.title

  useEffect(() => {
    if (title === undefined) return
    const previous = document.title
    document.title = `History - ${title} - SlashObserve`
    return () => {
      document.title = previous
    }
  }, [title])

  const onCompare = useCallback(
    (version: number) => {
      const range = { from: version - 1, to: version }
      setFocusRequest(range)
      void navigate({ search: (previous) => ({ ...previous, ...range }) })
    },
    [navigate]
  )

  const onRange = useCallback(
    (range: { from: number; to: number }) => {
      setFocusRequest(null)
      void navigate({
        search: (previous) => ({ ...previous, ...range }),
        replace: true,
      })
    },
    [navigate]
  )

  const onFocused = useCallback(() => setFocusRequest(null), [])

  const onRestored = useCallback(() => {
    void navigate({
      to: "/dashboards/$dashboardId",
      params: { dashboardId },
      search: dashboardSearchFromBack(search.back, { dropVersion: true }),
    })
  }, [navigate, dashboardId, search.back])

  const showDiff =
    search.from !== undefined || search.to !== undefined || latestVersion >= 2

  return (
    <main className="mx-auto w-full max-w-5xl p-6">
      <Link
        to="/dashboards/$dashboardId"
        params={{ dashboardId }}
        search={dashboardSearchFromBack(search.back)}
        className="mb-4 inline-flex items-center gap-1.5 rounded-sm text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <ArrowLeftIcon aria-hidden="true" className="size-4" />
        Back to dashboard
      </Link>
      <header className="mb-6 flex flex-col gap-1">
        <h1 className="text-2xl font-semibold">Version history</h1>
        <p className="text-sm wrap-anywhere text-muted-foreground">{title}</p>
      </header>
      <VersionList
        dashboardId={dashboardId}
        latestVersion={latestVersion}
        back={search.back}
        onCompare={onCompare}
        onRestored={onRestored}
      />
      {showDiff && latestVersion > 0 ? (
        <VersionDiff
          dashboardId={dashboardId}
          latestVersion={latestVersion}
          from={search.from}
          to={search.to}
          focusRequest={focusRequest}
          onFocused={onFocused}
          onChange={onRange}
        />
      ) : null}
    </main>
  )
}
