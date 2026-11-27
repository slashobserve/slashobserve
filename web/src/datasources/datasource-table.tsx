import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { logsLink, metricsLink, tracesLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { AudienceBadge } from "./audience-badge"
import { RemoveDatasourceButton } from "./remove-datasource"

import {
  exploreTargets,
  rowLabel,
  sourceLabel,
  statusOf,
  typeLabel,
  usageLabel,
} from "./format"
import {
  datasourcesQuery,
  useRecheckDatasource,
  type DatasourceStatus,
} from "./queries"

function datasourceId(datasource: DatasourceStatus): string {
  return datasource.uid
}

type Props = {
  onEdit: (uid: string, trigger: HTMLElement) => void
}

export function DatasourceTable({ onEdit }: Props) {
  const datasources = useQuery({
    ...datasourcesQuery(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  })
  const recheck = useRecheckDatasource()

  const listed = datasources.data ?? []

  function runRecheck(datasource: DatasourceStatus) {
    const label = rowLabel(datasource, listed)
    recheck.mutate(datasource.uid, {
      onSuccess: (status) => {
        const { text } = statusOf(status)
        if (status.healthy) toast.success(`${label}: ${text}`)
        else toast.error(`${label}: ${text}`)
      },
      onError: (error) =>
        toast.error(`Could not recheck ${label}: ${error.message}`),
    })
  }

  if (datasources.status === "pending") {
    return (
      <div className="flex flex-col gap-3" aria-busy="true">
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} className="h-5 w-full" />
        ))}
      </div>
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
        <h2 className="text-sm font-semibold">No datasources yet</h2>
        <p className="text-[13px] text-muted-foreground">
          Add one to let dashboards and the agent query your data.
        </p>
      </div>
    )
  }

  const columns: DataGridColumn<DatasourceStatus>[] = [
    {
      id: "name",
      header: "Name",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: (datasource) => (
        <div className="min-w-0 py-2">
          <DatasourceName datasource={datasource} />
        </div>
      ),
    },
    {
      id: "type",
      header: "Type",
      minWidth: 110,
      autoHeight: true,
      cell: (datasource) => (
        <div className="flex flex-col py-2">
          <span>{typeLabel(datasource.type)}</span>
          <span className="text-xs text-muted-foreground">
            {sourceLabel(datasource.source)}
          </span>
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      minWidth: 180,
      autoHeight: true,
      cell: (datasource) => {
        const { tone, text, advice } = statusOf(datasource)
        return (
          <div className="flex flex-col gap-0.5 py-2">
            <div className="flex">
              <StatusLabel tone={tone} title={text}>
                <span className="max-w-[360px] wrap-anywhere">{text}</span>
              </StatusLabel>
            </div>
            {datasource.detail ? (
              <span className="pl-[19px] text-xs text-muted-foreground">
                {datasource.detail}
              </span>
            ) : null}
            {advice === undefined ? null : (
              <span className="pl-[19px] text-xs text-muted-foreground">
                {advice}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: "usedBy",
      header: "Used by",
      minWidth: 140,
      cell: (datasource) => (
        <span className="text-muted-foreground">
          {usageLabel(datasource.dashboard_count)}
        </span>
      ),
    },
    {
      id: "checked",
      header: "Checked",
      align: "right",
      minWidth: 120,
      cell: (datasource) => (
        <span className="text-muted-foreground">
          <RelativeTime iso={datasource.checked_at} />
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      minWidth: 260,
      flex: 2,
      autoHeight: true,
      cell: (datasource) => (
        <div className="flex flex-wrap justify-end gap-1.5 py-1.5">
          <RecheckButton
            label={rowLabel(datasource, listed)}
            pending={recheck.isPending && recheck.variables === datasource.uid}
            onRecheck={() => runRecheck(datasource)}
          />
          <ExploreLinks
            datasource={datasource}
            label={rowLabel(datasource, listed)}
          />
          {datasource.source === "app" ? (
            <>
              <Button
                variant="outline"
                size="xs"
                aria-label={`Edit ${rowLabel(datasource, listed)}`}
                onClick={(event) => onEdit(datasource.uid, event.currentTarget)}
              >
                Edit
              </Button>
              <RemoveDatasourceButton
                datasource={datasource}
                label={rowLabel(datasource, listed)}
              />
            </>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <DataGrid
      label="Datasources"
      rows={datasources.data}
      columns={columns}
      rowId={datasourceId}
      rowAttributes={(datasource) => ({ "data-uid": datasource.uid })}
    />
  )
}

function DatasourceName({ datasource }: { datasource: DatasourceStatus }) {
  if (datasource.name === datasource.uid) {
    return (
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <span className="font-mono text-xs font-medium wrap-anywhere">
          {datasource.uid}
        </span>
        <AudienceBadge datasource={datasource} />
      </div>
    )
  }
  return (
    <div className="flex min-w-0 flex-col">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="min-w-0 font-medium wrap-break-word">
          {datasource.name}
        </span>
        <AudienceBadge datasource={datasource} />
      </div>
      <span className="font-mono text-xs font-normal wrap-anywhere text-muted-foreground">
        {datasource.uid}
      </span>
    </div>
  )
}

function RecheckButton({
  label,
  pending,
  onRecheck,
}: {
  label: string
  pending: boolean
  onRecheck: () => void
}) {
  return (
    <Button
      variant="outline"
      size="xs"
      disabled={pending}
      aria-busy={pending}
      aria-label={`Recheck ${label}`}
      onClick={onRecheck}
    >
      {pending ? (
        <LoaderCircleIcon
          aria-hidden="true"
          className="motion-safe:animate-spin"
        />
      ) : null}
      Recheck
    </Button>
  )
}

const exploreLinkClass = cn(buttonVariants({ variant: "outline", size: "xs" }))

function ExploreLinks({
  datasource,
  label,
}: {
  datasource: DatasourceStatus
  label: string
}) {
  const targets = exploreTargets(datasource.type)
  if (targets.length === 0) return null
  return (
    <>
      {targets.includes("logs") ? (
        <Link
          {...logsLink(datasource.uid)}
          aria-label={`Explore logs in ${label}`}
          className={exploreLinkClass}
        >
          Explore logs
        </Link>
      ) : null}
      {targets.includes("traces") ? (
        <Link
          {...tracesLink(datasource.uid)}
          aria-label={`Search traces in ${label}`}
          className={exploreLinkClass}
        >
          Search traces
        </Link>
      ) : null}
      {targets.includes("metrics") ? (
        <Link
          {...metricsLink(datasource.uid)}
          aria-label={`Explore metrics in ${label}`}
          className={exploreLinkClass}
        >
          Explore metrics
        </Link>
      ) : null}
    </>
  )
}
