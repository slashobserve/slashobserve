import { useId } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import { observationLink } from "@/lib/links"
import { observationsInfiniteQuery } from "@/observations/queries"
import { loadProblemText } from "@/service-accounts/queries"

export function FolderObservations({ folderId }: { folderId: string }) {
  const headingId = useId()
  const list = useInfiniteQuery(observationsInfiniteQuery({ folderId }))

  if (list.isError && list.data === undefined) {
    return (
      <LoadProblem
        title="Could not load this folder's observations"
        detail={loadProblemText(list.error)}
        retrying={list.isFetching}
        onRetry={() => void list.refetch()}
      />
    )
  }
  if (list.data === undefined) return null
  const observations = list.data.pages.flatMap((page) => page.observations)

  return (
    <div className="mt-6">
      <h2 id={headingId} className="mb-2 text-sm font-semibold">
        Observations
      </h2>
      {observations.length === 0 ? (
        <p className="text-[13px] text-muted-foreground">
          No observations in this folder
        </p>
      ) : (
        <TableCard labelledBy={headingId} minWidth={560} stackOnNarrow>
          <thead>
            <tr>
              <HeaderCell>Title</HeaderCell>
              <HeaderCell>State</HeaderCell>
              <HeaderCell>Updated</HeaderCell>
            </tr>
          </thead>
          <tbody>
            {observations.map((observation) => (
              <tr key={observation.id}>
                <Cell className="wrap-anywhere">
                  <Link
                    {...observationLink(observation.id)}
                    className="rounded-sm font-medium underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {observation.title}
                  </Link>
                </Cell>
                <Cell>
                  {observation.state === "open" ? (
                    <StatusLabel tone="ok">Open</StatusLabel>
                  ) : (
                    <StatusLabel tone="none">Resolved</StatusLabel>
                  )}
                </Cell>
                <Cell className="text-muted-foreground">
                  <RelativeTime iso={observation.updated_at} />
                </Cell>
              </tr>
            ))}
          </tbody>
        </TableCard>
      )}
      {list.hasNextPage ? (
        <div className="mt-3 flex justify-center">
          <Button
            variant="outline"
            disabled={list.isFetchingNextPage}
            aria-busy={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  )
}
