import { useCallback, useId, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { loadProblemText } from "@/api/errors"
import { AuthorMark } from "@/components/author-mark"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import {
  dataCell,
  skeletonRows,
  type SkeletonRow,
} from "@/components/data-grid/skeleton-cell"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import type { DatasourceStatus } from "@/datasources/queries"
import { FolderHeaderRow } from "@/folders/folder-header-row"
import { countText, groupByFolder } from "@/folders/group-by-folder"
import { foldersQuery, type Folder } from "@/folders/queries"
import { markOf } from "@/lib/author"

import { savedSearchLocation } from "./links"
import {
  savedSearchesPage,
  savedSearchesQuery,
  type SavedSearch,
} from "./queries"
import { SavedSearchRowMenu } from "./saved-search-row-menu"

const emptyHint = {
  logs: "Save search on a log source's page keeps its query, columns and time range for everyone who can view the folder.",
  traces:
    "Save search on a trace source's page keeps its filters and time range for everyone who can view the folder.",
  metrics:
    "Save search on a metrics source's page keeps its query, view and time range for everyone who can view the folder.",
} as const

type Row =
  | {
      kind: "folder"
      id: string
      folder: Folder
      collapsed: boolean
      countText: string | null
    }
  | { kind: "search"; id: string; saved: SavedSearch }
  | SkeletonRow

function savedSearchColumns(
  sources: readonly DatasourceStatus[]
): DataGridColumn<Row>[] {
  return [
    {
      id: "title",
      header: "Title",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: dataCell<Row, "search">(
        "search",
        "h-4 w-48 motion-safe:animate-none",
        ({ saved }) => (
          <Link
            {...savedSearchLocation(saved)}
            className="rounded-sm font-medium wrap-anywhere underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            {saved.title}
          </Link>
        )
      ),
    },
    {
      id: "source",
      header: "Source",
      minWidth: 120,
      flex: 1,
      autoHeight: true,
      cell: dataCell<Row, "search">(
        "search",
        "h-4 w-24 motion-safe:animate-none",
        ({ saved }) => {
          const source = sources.find(
            (candidate) => candidate.uid === saved.datasource_uid
          )
          return (
            <span className="wrap-anywhere text-muted-foreground">
              {source === undefined ? (
                <span className="font-mono text-xs">
                  {saved.datasource_uid}
                </span>
              ) : (
                source.name
              )}
            </span>
          )
        }
      ),
    },
    {
      id: "savedBy",
      header: "Saved by",
      minWidth: 140,
      flex: 1,
      cell: dataCell<Row, "search">(
        "search",
        "h-4 w-28 motion-safe:animate-none",
        ({ saved }) => (
          <span className="inline-flex items-center gap-2">
            <AuthorMark mark={markOf(saved.actor)} size={20} />
            {saved.actor.label}
          </span>
        )
      ),
    },
    {
      id: "updated",
      header: "Updated",
      width: 110,
      cell: dataCell<Row, "search">(
        "search",
        "h-4 w-12 motion-safe:animate-none",
        ({ saved }) => (
          <span className="whitespace-nowrap text-muted-foreground">
            <RelativeTime iso={saved.updated_at} />
          </span>
        )
      ),
    },
    {
      id: "actions",
      header: "Actions",
      headerCell: () => <span className="sr-only">Actions</span>,
      width: 56,
      action: true,
      cell: dataCell<Row, "search">(
        "search",
        "size-6 motion-safe:animate-none",
        ({ saved }) => <SavedSearchRowMenu saved={saved} />
      ),
    },
  ]
}

const rowId = (row: Row) => row.id

const skeleton = skeletonRows(3)

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
  const columns = useMemo(() => savedSearchColumns(sources), [sources])
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const toggle = useCallback(
    (folderId: string) =>
      setCollapsed((current) => {
        const next = new Set(current)
        if (!next.delete(folderId)) next.add(folderId)
        return next
      }),
    []
  )
  const rows = useMemo(
    () =>
      (sections ?? []).flatMap(({ folder, rows }): Row[] => {
        const isCollapsed = collapsed.has(folder.id)
        return [
          {
            kind: "folder",
            id: `folder:${folder.id}`,
            folder,
            collapsed: isCollapsed,
            countText: truncated
              ? null
              : countText(rows.length, "saved search", "saved searches"),
          },
          ...(isCollapsed
            ? []
            : rows.map((saved): Row => ({
                kind: "search",
                id: saved.id,
                saved,
              }))),
        ]
      }),
    [sections, collapsed, truncated]
  )
  const folderRow = useMemo(
    () => ({
      is: (row: Row) => row.kind === "folder",
      render: (row: Row) =>
        row.kind === "folder" ? (
          <FolderHeaderRow
            folder={row.folder}
            collapsed={row.collapsed}
            countText={row.countText}
            onToggle={() => toggle(row.folder.id)}
          />
        ) : null,
    }),
    [toggle]
  )

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
        <DataGrid
          labelledBy={headingId}
          busy
          rows={skeleton}
          columns={columns}
          rowId={rowId}
        />
      ) : (
        <>
          <DataGrid
            labelledBy={headingId}
            rows={rows}
            columns={columns}
            rowId={rowId}
            fullWidthRow={folderRow}
          />
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
