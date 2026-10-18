import { createFileRoute } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
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
  validateDashboardSearch,
  type DashboardSearchParams,
} from "@/dashboards/search"

export const Route = createFileRoute("/_authed/dashboards/$dashboardId/")({
  validateSearch: (raw: Record<string, unknown>): DashboardSearchParams =>
    searchFromState(validateDashboardSearch(raw)),
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
  remountDeps: ({ params }) => params,
  pendingComponent: DashboardPageSkeleton,
  errorComponent: ({ error }) => {
    if (
      error instanceof ApiError &&
      (error.status === 404 || error.status === 400)
    ) {
      return <DashboardNotFound />
    }
    throw error
  },
  component: DashboardRoute,
})

function DashboardRoute() {
  const { dashboardId } = Route.useParams()
  return <DashboardPage dashboardId={dashboardId} />
}
