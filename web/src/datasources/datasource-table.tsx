import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { logsLink, tracesLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { exploreTargets, statusOf, typeLabel, usageLabel } from "./format"
import { datasourcesQuery, type DatasourceStatus } from "./queries"

const columnCount = 6
const tableMinWidth = 760

function Header() {
  return (
    <thead>
      <tr>
        <HeaderCell>Name</HeaderCell>
        <HeaderCell>Type</HeaderCell>
        <HeaderCell>Status</HeaderCell>
        <HeaderCell>Used by</HeaderCell>
        <HeaderCell className="text-right">Checked</HeaderCell>
        <HeaderCell>
          <span className="sr-only">Actions</span>
        </HeaderCell>
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
      <TableCard label="Datasources" minWidth={tableMinWidth} stackOnNarrow>
        <Header />
        <tbody aria-busy="true">
          {[0, 1, 2].map((index) => (
            <tr key={index}>
              <Cell colSpan={columnCount}>
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
    <TableCard label="Datasources" minWidth={tableMinWidth} stackOnNarrow>
      <Header />
      <tbody>
        {datasources.data.map((datasource) => {
          const { tone, text } = statusOf(datasource)
          return (
            <tr key={datasource.uid} data-uid={datasource.uid}>
              <Cell>
                <DatasourceName datasource={datasource} />
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
              <Cell data-cell="action">
                <ExploreLinks datasource={datasource} />
              </Cell>
            </tr>
          )
        })}
      </tbody>
    </TableCard>
  )
}

function DatasourceName({ datasource }: { datasource: DatasourceStatus }) {
  if (datasource.name === datasource.uid) {
    return (
      <span className="font-mono text-xs font-medium whitespace-nowrap">
        {datasource.uid}
      </span>
    )
  }
  return (
    <div className="flex flex-col">
      <span className="font-medium">{datasource.name}</span>
      <span className="font-mono text-xs font-normal whitespace-nowrap text-muted-foreground">
        {datasource.uid}
      </span>
    </div>
  )
}

const exploreLinkClass = cn(buttonVariants({ variant: "outline", size: "xs" }))

function ExploreLinks({ datasource }: { datasource: DatasourceStatus }) {
  const targets = exploreTargets(datasource.type)
  if (targets.length === 0) return null
  return (
    <div className="flex justify-end gap-1.5">
      {targets.includes("logs") ? (
        <Link
          {...logsLink(datasource.uid)}
          aria-label={`Explore logs in ${datasource.uid}`}
          className={exploreLinkClass}
        >
          Explore logs
        </Link>
      ) : null}
      {targets.includes("traces") ? (
        <Link
          {...tracesLink(datasource.uid)}
          aria-label={`Search traces in ${datasource.uid}`}
          className={exploreLinkClass}
        >
          Search traces
        </Link>
      ) : null}
    </div>
  )
}
