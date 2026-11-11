import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { folderFindingsPage, folderFindingsQuery } from "@/findings/queries"
import { findingLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

export function FolderFindings({ folderId }: { folderId: string }) {
  const headingId = useId()
  const query = useQuery(folderFindingsQuery(folderId))

  if (query.isError) {
    return (
      <LoadProblem
        title="Could not load this folder's findings"
        detail={loadProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  const findings = query.data?.findings ?? []
  if (findings.length === 0) return null

  return (
    <div className="mt-6">
      <h2 id={headingId} className="mb-2 text-sm font-semibold">
        Findings
      </h2>
      <TableCard labelledBy={headingId} minWidth={560} stackOnNarrow>
        <thead>
          <tr>
            <HeaderCell>Title</HeaderCell>
            <HeaderCell>Saved by</HeaderCell>
            <HeaderCell>Created</HeaderCell>
          </tr>
        </thead>
        <tbody>
          {findings.map((finding) => (
            <tr key={finding.id}>
              <Cell className="wrap-anywhere">
                <Link
                  {...findingLink(finding.id)}
                  className="rounded-sm font-medium underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {finding.title}
                </Link>
              </Cell>
              <Cell className="text-muted-foreground">
                {finding.actor.label}
              </Cell>
              <Cell className="text-muted-foreground">
                <RelativeTime iso={finding.created_at} />
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
      {query.data?.next_cursor != null ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Showing the newest {folderFindingsPage} findings.
        </p>
      ) : null}
    </div>
  )
}
