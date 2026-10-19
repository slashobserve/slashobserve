import { useQuery } from "@tanstack/react-query"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Skeleton } from "@/components/ui/skeleton"
import { loadProblemText } from "@/service-accounts/queries"

import { statusOf, typeLabel, usageLabel } from "./format"
import { datasourcesQuery } from "./queries"

function Header() {
  return (
    <thead>
      <tr>
        <HeaderCell>Name</HeaderCell>
        <HeaderCell>Type</HeaderCell>
        <HeaderCell>Status</HeaderCell>
        <HeaderCell>Used by</HeaderCell>
        <HeaderCell className="text-right">Checked</HeaderCell>
      </tr>
    </thead>
  )
}

export function DatasourceTable() {
  const datasources = useQuery({
    ...datasourcesQuery(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })

  if (datasources.status === "pending") {
    return (
      <TableCard label="Datasources" minWidth={680}>
        <Header />
        <tbody aria-busy="true">
          {[0, 1, 2].map((index) => (
            <tr key={index}>
              <Cell colSpan={5}>
                <Skeleton className="h-5 w-full" />
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
    )
  }

  if (datasources.data === undefined) {
    return (
      <LoadProblem
        title="Could not load datasources"
        detail={loadProblemText(datasources.error)}
        retrying={datasources.isFetching}
        onRetry={() => void datasources.refetch()}
      />
    )
  }

  if (datasources.data.length === 0) {
    return (
      <div className="rounded-card border px-4 py-10 text-center">
        <h2 className="text-sm font-semibold">No datasources configured</h2>
        <p className="text-[13px] text-muted-foreground">
          Add them to the datasources file and restart the server.
        </p>
      </div>
    )
  }

  return (
    <TableCard label="Datasources" minWidth={680}>
      <Header />
      <tbody>
        {datasources.data.map((datasource) => {
          const { tone, text } = statusOf(datasource)
          return (
            <tr key={datasource.uid} data-uid={datasource.uid}>
              <Cell>
                <span className="font-mono text-xs font-medium whitespace-nowrap">
                  {datasource.uid}
                </span>
              </Cell>
              <Cell>{typeLabel(datasource.type)}</Cell>
              <Cell>
                <div className="flex">
                  <StatusLabel tone={tone} title={text}>
                    <span className="max-w-[360px] truncate">{text}</span>
                  </StatusLabel>
                </div>
              </Cell>
              <Cell className="text-muted-foreground">
                {usageLabel(datasource.dashboard_count)}
              </Cell>
              <Cell className="text-right text-muted-foreground">
                <RelativeTime iso={datasource.checked_at} />
              </Cell>
            </tr>
          )
        })}
      </tbody>
    </TableCard>
  )
}
