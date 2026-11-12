import { useState } from "react"
import { createFileRoute, useRouter } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { PublicLinkUnavailable, PublicPageSkeleton } from "@/public/public-page"
import { PublicRcaLoadError, PublicRcaPage } from "@/rca/public-rca-page"

export const Route = createFileRoute("/rca/$token")({
  remountDeps: ({ params }) => ({ token: params.token }),
  pendingComponent: PublicPageSkeleton,
  errorComponent: ({ error, reset }) =>
    error instanceof ApiError && error.status === 404 ? (
      <PublicLinkUnavailable />
    ) : (
      <PublicRouteError error={error} reset={reset} />
    ),
  component: PublicRcaRoute,
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
    <PublicRcaLoadError
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

function PublicRcaRoute() {
  const { token } = Route.useParams()
  return <PublicRcaPage token={token} />
}
