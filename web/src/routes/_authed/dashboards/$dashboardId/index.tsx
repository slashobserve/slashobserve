import { useState } from "react"
import { createFileRoute, useRouter } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { DashboardLoadError } from "@/dashboards/load-error"
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
    return <DashboardRouteError error={error} />
  },
  component: DashboardRoute,
})

function DashboardRouteError({ error }: { error: unknown }) {
  const router = useRouter()
  const [retrying, setRetrying] = useState(false)
  return (
    <DashboardLoadError
      title="Could not load the dashboard"
      error={error}
      retrying={retrying}
      onRetry={() => {
        setRetrying(true)
        void router.invalidate().finally(() => setRetrying(false))
      }}
    />
  )
}

function DashboardRoute() {
  const { dashboardId } = Route.useParams()
  return <DashboardPage dashboardId={dashboardId} />
}
