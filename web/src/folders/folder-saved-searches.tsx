import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { loadProblemText } from "@/api/errors"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { savedSearchLocation } from "@/saved-searches/links"
import {
  savedSearchesPage,
  savedSearchesQuery,
  type SavedSearch,
} from "@/saved-searches/queries"
import { SavedSearchRowMenu } from "@/saved-searches/saved-search-row-menu"

const kindLabels = {
  logs: "Logs",
  traces: "Traces",
  metrics: "Metrics",
} as const

const columns: DataGridColumn<SavedSearch>[] = [
  {
    id: "title",
    header: "Title",
    minWidth: 200,
    flex: 2,
    autoHeight: true,
    cell: (saved) => (
      <Link
        {...savedSearchLocation(saved)}
        className="rounded-sm font-medium wrap-anywhere underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        {saved.title}
      </Link>
    ),
  },
  {
    id: "kind",
    header: "Kind",
    width: 110,
    cell: (saved) => (
      <span className="text-muted-foreground">{kindLabels[saved.kind]}</span>
    ),
  },
  {
    id: "datasource",
    header: "Datasource",
    minWidth: 140,
    flex: 1,
    cell: (saved) => (
      <span className="truncate font-mono text-xs text-muted-foreground">
        {saved.datasource_uid}
      </span>
    ),
  },
  {
    id: "updated",
    header: "Updated",
    width: 110,
    cell: (saved) => (
      <span className="text-muted-foreground">
        <RelativeTime iso={saved.updated_at} />
      </span>
    ),
  },
  {
    id: "actions",
    header: "Actions",
    headerCell: () => <span className="sr-only">Actions</span>,
    width: 56,
    action: true,
    cell: (saved) => <SavedSearchRowMenu saved={saved} />,
  },
]

const savedSearchId = (saved: SavedSearch) => saved.id

export function FolderSavedSearches({ folderId }: { folderId: string }) {
  const headingId = useId()
  const query = useQuery(savedSearchesQuery({ folderId }))

  if (query.isError) {
    return (
      <LoadProblem
        title="Could not load this folder's saved searches"
        detail={loadProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const searches = query.data?.saved_searches ?? []
  if (searches.length === 0) return null

  return (
    <div className="mt-6">
      <h2 id={headingId} className="mb-2 text-sm font-semibold">
        Saved searches
      </h2>
      <DataGrid
        labelledBy={headingId}
        rows={searches}
        columns={columns}
        rowId={savedSearchId}
      />
      {query.data?.next_cursor != null ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Showing the {savedSearchesPage} most recently changed saved searches.
        </p>
      ) : null}
    </div>
  )
}
