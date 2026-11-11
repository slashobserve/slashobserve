import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { savedSearchLocation } from "@/saved-searches/links"
import { savedSearchesPage, savedSearchesQuery } from "@/saved-searches/queries"
import { SavedSearchRowMenu } from "@/saved-searches/saved-search-row-menu"
import { loadProblemText } from "@/service-accounts/queries"

const kindLabels = { logs: "Logs", traces: "Traces" } as const

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
      <TableCard labelledBy={headingId} minWidth={640} stackOnNarrow>
        <thead>
          <tr>
            <HeaderCell>Title</HeaderCell>
            <HeaderCell>Kind</HeaderCell>
            <HeaderCell>Datasource</HeaderCell>
            <HeaderCell>Updated</HeaderCell>
            <HeaderCell>
              <span className="sr-only">Actions</span>
            </HeaderCell>
          </tr>
        </thead>
        <tbody>
          {searches.map((saved) => (
            <tr key={saved.id}>
              <Cell className="wrap-anywhere">
                <Link
                  {...savedSearchLocation(saved)}
                  className="rounded-sm font-medium underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {saved.title}
                </Link>
              </Cell>
              <Cell className="text-muted-foreground">
                {kindLabels[saved.kind]}
              </Cell>
              <Cell className="font-mono text-xs text-muted-foreground">
                {saved.datasource_uid}
              </Cell>
              <Cell className="text-muted-foreground">
                <RelativeTime iso={saved.updated_at} />
              </Cell>
              <Cell data-cell="action" className="w-10 text-right">
                <SavedSearchRowMenu saved={saved} />
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
      {query.data?.next_cursor != null ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Showing the {savedSearchesPage} most recently changed saved searches.
        </p>
      ) : null}
    </div>
  )
}
