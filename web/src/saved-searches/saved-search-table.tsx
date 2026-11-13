import { useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Skeleton } from "@/components/ui/skeleton"
import type { DatasourceStatus } from "@/datasources/queries"
import { FolderHeaderRow } from "@/folders/folder-header-row"
import { countText, groupByFolder } from "@/folders/group-by-folder"
import { foldersQuery } from "@/folders/queries"
import { markOf } from "@/lib/author"
import { loadProblemText } from "@/service-accounts/queries"

import { savedSearchLocation } from "./links"
import {
  savedSearchesPage,
  savedSearchesQuery,
  type SavedSearch,
} from "./queries"
import { SavedSearchRowMenu } from "./saved-search-row-menu"

const tableWidth = 640
const columnCount = 5
const skeletonRows = 3

const emptyHint = {
  logs: "Save search on a log source's page keeps its query, columns and time range for everyone who can view the folder.",
  traces:
    "Save search on a trace source's page keeps its filters and time range for everyone who can view the folder.",
} as const

function SavedSearchColumns() {
  return (
    <thead>
      <tr>
        <HeaderCell>Title</HeaderCell>
        <HeaderCell>Source</HeaderCell>
        <HeaderCell>Saved by</HeaderCell>
        <HeaderCell>Updated</HeaderCell>
        <HeaderCell>
          <span className="sr-only">Actions</span>
        </HeaderCell>
      </tr>
    </thead>
  )
}

function SavedSearchSkeleton({ headingId }: { headingId: string }) {
  return (
    <TableCard labelledBy={headingId} minWidth={tableWidth} stackOnNarrow>
      <SavedSearchColumns />
      <tbody aria-busy="true">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <tr key={index}>
            <Cell>
              <Skeleton className="h-4 w-48 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-24 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-28 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-12 motion-safe:animate-none" />
            </Cell>
            <Cell data-cell="action">
              <Skeleton className="size-6 motion-safe:animate-none" />
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}

function SourceCell({
  saved,
  sources,
}: {
  saved: SavedSearch
  sources: readonly DatasourceStatus[]
}) {
  const source = sources.find(
    (candidate) => candidate.uid === saved.datasource_uid
  )
  return (
    <Cell className="wrap-anywhere text-muted-foreground">
      {source === undefined ? (
        <span className="font-mono text-xs">{saved.datasource_uid}</span>
      ) : (
        source.name
      )}
    </Cell>
  )
}

function SavedSearchRow({
  saved,
  sources,
}: {
  saved: SavedSearch
  sources: readonly DatasourceStatus[]
}) {
  return (
    <tr>
      <Cell className="wrap-anywhere">
        <Link
          {...savedSearchLocation(saved)}
          className="rounded-sm font-medium underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {saved.title}
        </Link>
      </Cell>
      <SourceCell saved={saved} sources={sources} />
      <Cell>
        <span className="inline-flex items-center gap-2">
          <AuthorMark mark={markOf(saved.actor)} size={20} />
          {saved.actor.label}
        </span>
      </Cell>
      <Cell className="whitespace-nowrap text-muted-foreground">
        <RelativeTime iso={saved.updated_at} />
      </Cell>
      <Cell data-cell="action" className="w-10 text-right">
        <SavedSearchRowMenu saved={saved} />
      </Cell>
    </tr>
  )
}

export function SavedSearchTable({
  kind,
  sources,
}: {
  kind: SavedSearch["kind"]
  sources: readonly DatasourceStatus[]
}) {
  const headingId = useId()
  const query = useQuery(savedSearchesQuery({ kind }))
  const folderResult = useQuery(foldersQuery())
  const searches = query.data?.saved_searches
  const folders = folderResult.data
  const truncated = query.data?.next_cursor != null
  const sections = useMemo(
    () =>
      searches === undefined || folders === undefined
        ? null
        : groupByFolder(folders, searches, (saved) => saved.folder.id),
    [searches, folders]
  )
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const toggle = (folderId: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(folderId)) next.add(folderId)
      return next
    })

  return (
    <section aria-labelledby={headingId} className="mt-6">
      <h2 id={headingId} className="mb-2 text-sm font-semibold">
        Saved searches
      </h2>
      {query.isError ? (
        <LoadProblem
          title="Could not load saved searches"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      ) : searches?.length === 0 ? (
        <div className="rounded-card border bg-background">
          <EmptyState title="No saved searches yet">
            {emptyHint[kind]}
          </EmptyState>
        </div>
      ) : folderResult.isError ? (
        <LoadProblem
          title="Could not load folders"
          detail={loadProblemText(folderResult.error)}
          retrying={folderResult.isFetching}
          onRetry={() => void folderResult.refetch()}
        />
      ) : sections === null ? (
        <SavedSearchSkeleton headingId={headingId} />
      ) : (
        <>
          <TableCard labelledBy={headingId} minWidth={tableWidth} stackOnNarrow>
            <SavedSearchColumns />
            <tbody>
              {sections.flatMap(({ folder, rows }) => {
                const isCollapsed = collapsed.has(folder.id)
                const header = (
                  <FolderHeaderRow
                    key={`folder:${folder.id}`}
                    folder={folder}
                    collapsed={isCollapsed}
                    countText={
                      truncated
                        ? null
                        : countText(
                            rows.length,
                            "saved search",
                            "saved searches"
                          )
                    }
                    columnCount={columnCount}
                    onToggle={() => toggle(folder.id)}
                  />
                )
                return isCollapsed
                  ? [header]
                  : [
                      header,
                      ...rows.map((saved) => (
                        <SavedSearchRow
                          key={saved.id}
                          saved={saved}
                          sources={sources}
                        />
                      )),
                    ]
              })}
            </tbody>
          </TableCard>
          {truncated ? (
            <p className="mt-2 text-xs text-muted-foreground">
              Showing the {savedSearchesPage} most recently changed saved
              searches.
            </p>
          ) : null}
        </>
      )}
    </section>
  )
}
