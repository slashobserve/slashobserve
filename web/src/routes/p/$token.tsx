import { useCallback, useMemo, useState } from "react"
import { useSuspenseQuery } from "@tanstack/react-query"
import { createFileRoute, useRouter } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import {
  searchFromState,
  validateDashboardSearch,
  type DashboardSearch,
  type DashboardSearchParams,
} from "@/dashboards/search"
import {
  PublicLinkUnavailable,
  PublicLoadError,
  PublicPage,
  PublicPageSkeleton,
  type SetPublicSearch,
} from "@/public/public-page"
import { publicDashboardQuery } from "@/public/queries"

function publicSearch(state: DashboardSearch): DashboardSearch {
  return {
    from: state.from,
    to: state.to,
    refresh: state.refresh,
    vars: state.vars,
  }
}

function publicSearchParams(raw: Record<string, unknown>) {
  return searchFromState(publicSearch(validateDashboardSearch(raw)))
}

export const Route = createFileRoute("/p/$token")({
  validateSearch: (raw: Record<string, unknown>): DashboardSearchParams =>
    publicSearchParams(raw),
  loader: ({ context, params }) =>
    context.queryClient.ensureQueryData(publicDashboardQuery(params.token)),
  remountDeps: ({ params }) => ({ token: params.token }),
  pendingComponent: PublicPageSkeleton,
  errorComponent: ({ error, reset }) =>
    error instanceof ApiError && error.status === 404 ? (
      <PublicLinkUnavailable />
    ) : (
      <PublicRouteError error={error} reset={reset} />
    ),
  component: PublicRoute,
})

function PublicRouteError({
  error,
  reset,
}: {
  error: unknown
  reset: () => void
}) {
  const router = useRouter()
  const [retrying, setRetrying] = useState(false)
  return (
    <PublicLoadError
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

function PublicRoute() {
  const { token } = Route.useParams()
  const raw = Route.useSearch()
  const search = useMemo(() => validateDashboardSearch(raw), [raw])
  const navigate = Route.useNavigate()
  const dashboard = useSuspenseQuery(publicDashboardQuery(token)).data
  const setSearch = useCallback<SetPublicSearch>(
    (update, options) =>
      void navigate({
        search: (prev: DashboardSearchParams) =>
          searchFromState(publicSearch(update(validateDashboardSearch(prev)))),
        replace: options?.replace ?? false,
      }),
    [navigate]
  )
  return (
    <PublicPage
      token={token}
      dashboard={dashboard}
      search={search}
      onSearchChange={setSearch}
    />
  )
}
