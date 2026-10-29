import { useCallback, useEffect, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, createFileRoute } from "@tanstack/react-router"
import { ArrowLeftIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { DashboardNoAccess } from "@/dashboards/no-access"
import { DashboardNotFound } from "@/dashboards/not-found"
import { useDashboardEvents } from "@/dashboards/live/use-dashboard-events"
import { latestDashboardQuery } from "@/dashboards/queries"
import { dashboardSearchFromBack } from "@/dashboards/search"
import { validateHistorySearch } from "@/history/search"
import { useKnownLatestVersion } from "@/history/use-version-history"
import { VersionDiff } from "@/history/version-diff"
import { VersionList } from "@/history/version-list"
import { ThemeSwitch } from "@/shell/theme-switch"

type FocusRequest = { from: number; to: number }

export const Route = createFileRoute(
  "/_authed/dashboards/$dashboardId/versions"
)({
  validateSearch: validateHistorySearch,
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
    if (
      error instanceof ApiError &&
      error.status === 403 &&
      error.code === "forbidden_role"
    ) {
      return <DashboardNoAccess />
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
        <div className="flex min-h-[30px] min-w-0 flex-row items-center gap-2.5 narrow:flex-col narrow:items-start narrow:gap-1">
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
            className="text-muted-foreground narrow:hidden"
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
