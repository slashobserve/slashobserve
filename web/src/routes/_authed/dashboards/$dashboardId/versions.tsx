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
import { ThemeSwitch } from "@/shell/theme-switch"

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
    <>
      <header className="flex items-start justify-between gap-x-6 border-b px-6 py-2.5">
        <div className="flex min-h-[30px] min-w-0 flex-col items-start gap-1 min-[640px]:flex-row min-[640px]:items-center min-[640px]:gap-2.5">
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId }}
            search={dashboardSearchFromBack(search.back)}
            className="flex shrink-0 items-center gap-1.5 rounded-sm text-[13px] text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeftIcon aria-hidden="true" className="size-3.5" />
            Back to dashboard
          </Link>
          <span
            aria-hidden="true"
            className="hidden text-muted-foreground min-[640px]:inline"
          >
            /
          </span>
          <h1 className="min-w-0 text-[17px] font-semibold wrap-anywhere">
            {title ? `${title}: history` : "History"}
          </h1>
        </div>
        <div className="ml-auto flex flex-none items-center">
          <ThemeSwitch />
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl p-6">
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
    </>
  )
}
