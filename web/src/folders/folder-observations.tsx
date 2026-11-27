import { useId } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { loadProblemText } from "@/api/errors"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { observationLink } from "@/lib/links"
import {
  observationsInfiniteQuery,
  type Observation,
} from "@/observations/queries"
import { plainTitle } from "@/comments/plain-title"

const columns: DataGridColumn<Observation>[] = [
  {
    id: "title",
    header: "Title",
    minWidth: 240,
    flex: 2,
    autoHeight: true,
    cell: (observation) => (
      <Link
        {...observationLink(observation.id)}
        className="rounded-sm font-medium wrap-anywhere underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        {plainTitle(observation.title)}
      </Link>
    ),
  },
  {
    id: "state",
    header: "State",
    width: 110,
    cell: (observation) =>
      observation.state === "open" ? (
        <StatusLabel tone="ok">Open</StatusLabel>
      ) : (
        <StatusLabel tone="none">Resolved</StatusLabel>
      ),
  },
  {
    id: "updated",
    header: "Updated",
    width: 130,
    cell: (observation) => (
      <span className="text-muted-foreground">
        <RelativeTime iso={observation.updated_at} />
      </span>
    ),
  },
]

const observationId = (observation: Observation) => observation.id

export function FolderObservations({
  folderId,
  labelledBy,
}: {
  folderId: string
  labelledBy?: string
}) {
  const ownHeadingId = useId()
  const headingId = labelledBy ?? ownHeadingId
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
    <div className={labelledBy === undefined ? "mt-6" : undefined}>
      {labelledBy === undefined ? (
        <h2 id={headingId} className="mb-2 text-sm font-semibold">
          Observations
        </h2>
      ) : null}
      {observations.length === 0 ? (
        <p className="text-[13px] text-muted-foreground">
          No observations in this folder
        </p>
      ) : (
        <DataGrid
          labelledBy={headingId}
          rows={observations}
          columns={columns}
          rowId={observationId}
        />
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
