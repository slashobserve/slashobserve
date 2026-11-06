import { useEffect, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { createFileRoute, redirect, useRouter } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { DashboardLoadError } from "@/dashboards/load-error"
import { DashboardNoAccess } from "@/dashboards/no-access"
import { DashboardNotFound } from "@/dashboards/not-found"
import {
  DashboardPage,
  DashboardPageSkeleton,
} from "@/dashboards/dashboard-page"
import {
  dashboardVersionQuery,
  latestDashboardQuery,
} from "@/dashboards/queries"
import {
  searchFromState,
  useDashboardSearch,
  validateDashboardSearch,
  type DashboardSearchParams,
} from "@/dashboards/search"
import { useRecordRecent } from "@/home/recent"
import { useRecordView } from "@/health/record-view"
import { dashboardLink } from "@/lib/links"

export const Route = createFileRoute("/_authed/d/$dashboardId/{-$slug}")({
  validateSearch: (raw: Record<string, unknown>): DashboardSearchParams =>
    searchFromState(validateDashboardSearch(raw)),
  beforeLoad: ({ location, params }) => {
    if (!("tab" in location.search)) return
    throw redirect({
      to: "/d/$dashboardId/{-$slug}",
      params,
      search: searchFromState(validateDashboardSearch(location.search)),
      replace: true,
    })
  },
  loaderDeps: ({ search }) => ({
    version: validateDashboardSearch(search).version,
  }),
  loader: async ({ context, params, deps }) => {
    const latest = latestDashboardQuery(context.queryClient, params.dashboardId)
    if (deps.version === undefined) {
      await context.queryClient.ensureQueryData(latest)
      return
    }
    await Promise.all([
      context.queryClient.ensureQueryData(
        dashboardVersionQuery(params.dashboardId, deps.version)
      ),
      context.queryClient.ensureQueryData(latest),
    ])
  },
  remountDeps: ({ params }) => ({ dashboardId: params.dashboardId }),
  pendingComponent: DashboardPageSkeleton,
  errorComponent: ({ error, reset }) => {
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
    return <DashboardRouteError error={error} reset={reset} />
  },
  component: DashboardRoute,
})

function DashboardRouteError({
  error,
  reset,
}: {
  error: unknown
  reset: () => void
}) {
  const router = useRouter()
  const { version } = useDashboardSearch()
  const [retrying, setRetrying] = useState(false)
  return (
    <DashboardLoadError
      title={
        version === undefined
          ? "Could not load the dashboard"
          : `Could not load version ${version}`
      }
      error={error}
      retrying={retrying}
      onRetry={() => {
        setRetrying(true)
        reset()
        void router.invalidate().finally(() => setRetrying(false))
      }}
    />
  )
}

function DashboardRoute() {
  const { dashboardId, slug } = Route.useParams()
  const navigate = Route.useNavigate()
  const queryClient = useQueryClient()
  useRecordView(dashboardId)
  const title = useQuery(latestDashboardQuery(queryClient, dashboardId)).data
    ?.dashboard.title
  useRecordRecent(dashboardId, title)
  useEffect(() => {
    if (title === undefined) return
    const canonical = dashboardLink(dashboardId, title)
    if (canonical.params.slug === slug) return
    void navigate({ ...canonical, search: true, hash: true, replace: true })
  }, [navigate, dashboardId, slug, title])
  return <DashboardPage dashboardId={dashboardId} />
}
