import { useState } from "react"
import {
  Link,
  createFileRoute,
  redirect,
  useRouter,
} from "@tanstack/react-router"

import { ApiError, loadProblemText } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { buttonVariants } from "@/components/ui/button"
import { findingQuery } from "@/findings/queries"
import { useDocumentTitle } from "@/lib/document-title"
import { observationsLink } from "@/lib/links"
import { cn } from "@/lib/utils"

export const Route = createFileRoute("/_authed/_shell/findings/$findingId")({
  beforeLoad: async ({ params, context }) => {
    const { finding } = await context.queryClient.fetchQuery(
      findingQuery(params.findingId)
    )
    if (finding.observation_id !== null) {
      throw redirect({
        to: "/o/$observationId",
        params: { observationId: finding.observation_id },
        replace: true,
      })
    }
    throw redirect({
      to: "/c/$requestId",
      params: { requestId: finding.agent_request_id ?? params.findingId },
      replace: true,
    })
  },
  errorComponent: FindingRedirectError,
})

function FindingRedirectError({
  error,
  reset,
}: {
  error: unknown
  reset: () => void
}) {
  const router = useRouter()
  const [retrying, setRetrying] = useState(false)
  const missing = error instanceof ApiError && error.status === 404
  useDocumentTitle(missing ? "Finding not found" : "Could not load the finding")
  if (missing) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
        <h1 className="text-2xl font-semibold">Finding not found</h1>
        <p className="text-sm text-muted-foreground">
          This finding does not exist or you cannot see it
        </p>
        <Link
          {...observationsLink()}
          className={cn(buttonVariants({ variant: "outline" }))}
        >
          Back to Observations
        </Link>
      </div>
    )
  }
  return (
    <LoadProblem
      title="Could not load the finding"
      detail={loadProblemText(error)}
      retrying={retrying}
      onRetry={() => {
        setRetrying(true)
        reset()
        void router.invalidate().finally(() => setRetrying(false))
      }}
    />
  )
}
