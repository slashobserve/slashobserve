import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { RetryButton } from "@/chat/turn-failure"
import { PageHeader } from "@/components/page-header"
import { RelativeTime } from "@/components/relative-time"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { FindingCard } from "./finding-card"
import { findingQuery, type Finding } from "./queries"

const outlineLink = cn(buttonVariants({ variant: "outline", size: "sm" }))

function FindingSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-4">
      <Skeleton className="h-7 w-2/3" />
      <div className="grid grid-cols-1 gap-2.5 md:grid-cols-2">
        <Skeleton className="h-[220px] rounded-card" />
        <Skeleton className="h-[220px] rounded-card" />
      </div>
    </div>
  )
}

function FindingNotFound() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Finding not found</h1>
      <p className="text-sm text-muted-foreground">
        This finding does not exist, or you cannot see it.
      </p>
      <Link to="/" className={cn(buttonVariants({ variant: "outline" }))}>
        Back to Observe
      </Link>
    </div>
  )
}

function LoadedFinding({ finding }: { finding: Finding }) {
  useDocumentTitle(`${finding.title} - SlashObserve`)
  const foundBy =
    finding.agent_request_id === null
      ? `Saved by ${finding.created_by.name}`
      : "Found by the agent"
  return (
    <>
      <PageHeader
        title={finding.title}
        description={
          <>
            {foundBy} <RelativeTime iso={finding.created_at} />
          </>
        }
        actions={
          <>
            {finding.dashboard_id !== null ? (
              <Link
                {...dashboardLink(finding.dashboard_id)}
                className={outlineLink}
              >
                Open dashboard
              </Link>
            ) : null}
            {finding.agent_request_id !== null ? (
              <Link
                to="/c/$requestId"
                params={{ requestId: finding.agent_request_id }}
                className={outlineLink}
              >
                Open chat
              </Link>
            ) : null}
          </>
        }
      />
      <FindingCard finding={finding} showOpenLink={false} />
    </>
  )
}

export function FindingPage({ findingId }: { findingId: string }) {
  const query = useQuery(findingQuery(findingId))
  if (query.data !== undefined) return <LoadedFinding finding={query.data} />
  if (query.error instanceof ApiError && query.error.status === 404) {
    return <FindingNotFound />
  }
  if (query.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Could not load this finding</AlertTitle>
        <AlertDescription>
          <p>{sentence(query.error.message)}</p>
          <RetryButton
            busy={query.isFetching}
            onClick={() => void query.refetch()}
          >
            Retry
          </RetryButton>
        </AlertDescription>
      </Alert>
    )
  }
  return <FindingSkeleton />
}
