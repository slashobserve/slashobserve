import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { ArrowLeftIcon } from "lucide-react"

import { loadProblemText } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { Skeleton } from "@/components/ui/skeleton"
import { buttonVariants } from "@/components/ui/button"
import { useDocumentTitle } from "@/lib/document-title"
import { cn } from "@/lib/utils"
import { snapshotQuery } from "@/observations/blocks/block-data"
import { observationIsMissing } from "@/observations/errors"

import { rcaQuery, rcasQuery } from "./queries"
import { RcaDocument, type UseSnapshot } from "./rca-document"

const useStoredSnapshot: UseSnapshot = (snapshotId) =>
  useQuery({
    ...snapshotQuery(snapshotId),
    select: (snapshot) => snapshot.data,
  })

function BackToObservation({ observationId }: { observationId: string }) {
  return (
    <Link
      to="/o/$observationId"
      params={{ observationId }}
      search={{ rca: "1" }}
      className={cn(buttonVariants({ variant: "outline" }), "self-start")}
    >
      <ArrowLeftIcon aria-hidden="true" className="size-3.5" />
      Back to the observation
    </Link>
  )
}

function RevisionNotFound({ observationId }: { observationId: string }) {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Revision not found</h1>
      <p className="text-sm text-muted-foreground">
        This revision was withdrawn, or you cannot see it
      </p>
      <BackToObservation observationId={observationId} />
    </div>
  )
}

export function RcaRevisionPage({
  observationId,
  revision,
}: {
  observationId: string
  revision: number
}) {
  const valid = Number.isInteger(revision) && revision > 0
  const query = useQuery({
    ...rcaQuery(observationId, revision),
    enabled: valid,
  })
  const rca = query.data
  const newest = useQuery({ ...rcasQuery(observationId), enabled: valid })
    .data?.[0]?.revision
  useDocumentTitle(
    rca === undefined
      ? "RCA revision - SlashObserve"
      : `${rca.title}, revision ${rca.revision} - SlashObserve`
  )
  if (!valid) return <RevisionNotFound observationId={observationId} />
  if (rca !== undefined) {
    return (
      <div className="flex w-full max-w-[720px] flex-col gap-6">
        <BackToObservation observationId={observationId} />
        <RcaDocument
          rca={{
            ...rca,
            publishedBy: rca.created_by.label,
            supersededBy:
              newest !== undefined && newest > rca.revision ? newest : null,
          }}
          useSnapshot={useStoredSnapshot}
          newerLink={
            newest === undefined ? null : (
              <Link
                to="/o/$observationId/rca/$revision"
                params={{ observationId, revision: String(newest) }}
                className="underline underline-offset-2"
              >
                Open revision {newest}
              </Link>
            )
          }
        />
      </div>
    )
  }
  if (observationIsMissing(query.error))
    return <RevisionNotFound observationId={observationId} />
  if (query.isError) {
    return (
      <LoadProblem
        title="Could not load this revision"
        detail={loadProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return (
    <div aria-busy="true" className="flex w-full max-w-[720px] flex-col gap-6">
      <Skeleton className="h-7 w-1/2" />
      <Skeleton className="h-40 rounded-card" />
    </div>
  )
}
