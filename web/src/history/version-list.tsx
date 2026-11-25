import { useMemo } from "react"
import { Link } from "@tanstack/react-router"

import { AgentMark } from "@/components/agent-mark"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import {
  dataCell,
  skeletonRows,
  type SkeletonRow,
} from "@/components/data-grid/skeleton-cell"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { dashboardSearchFromBack } from "@/dashboards/search"
import { formatDateTime, formatRelative } from "@/lib/time"
import { cn } from "@/lib/utils"
import { dashboardLink } from "@/lib/links"

import { RestoreVersionButton } from "./restore-version-button"
import { useVersionHistory, type VersionMeta } from "./use-version-history"

type Props = {
  dashboardId: string
  latestVersion: number
  back: string | undefined
  onCompare: (version: number) => void
  onRestored: () => void
}

function Author({ meta }: { meta: VersionMeta }) {
  if (meta.agent_request_id === null) return meta.actor.label
  return (
    <span className="inline-flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
      <AgentMark className="size-3.5" />
      <span>{meta.actor.label}</span>
      <Link
        to="/c/$requestId"
        params={{ requestId: meta.agent_request_id }}
        aria-label={`From chat for version ${meta.version}`}
        className="rounded-sm font-medium underline-offset-2 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        From chat
      </Link>
    </span>
  )
}

type Row = { kind: "version"; id: string; meta: VersionMeta } | SkeletonRow

function versionColumns({
  dashboardId,
  latestVersion,
  back,
  onCompare,
  onRestored,
}: Props): DataGridColumn<Row>[] {
  return [
    {
      id: "version",
      header: "Version",
      width: 190,
      autoHeight: true,
      cell: dataCell<Row, "version">("version", "h-5 w-10", ({ meta }) => (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium tabular-nums">v{meta.version}</span>
          {meta.version === latestVersion ? (
            <Badge variant="secondary">Latest</Badge>
          ) : null}
          {meta.restored_from !== null ? (
            <Badge variant="outline">Restored from v{meta.restored_from}</Badge>
          ) : null}
        </div>
      )),
    },
    {
      id: "summary",
      header: "Summary",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: dataCell<Row, "version">(
        "version",
        "h-5 w-40 max-w-full",
        ({ meta }) => <span className="wrap-anywhere">{meta.summary}</span>
      ),
    },
    {
      id: "author",
      header: "Author",
      minWidth: 160,
      flex: 1,
      autoHeight: true,
      cell: dataCell<Row, "version">("version", "h-5 w-20", ({ meta }) => (
        <span className="wrap-anywhere">
          <Author meta={meta} />
        </span>
      )),
    },
    {
      id: "saved",
      header: "Saved",
      width: 110,
      cell: dataCell<Row, "version">("version", "h-5 w-16", ({ meta }) => (
        <span className="text-muted-foreground">
          <time
            dateTime={meta.created_at}
            title={formatDateTime(meta.created_at)}
          >
            {formatRelative(meta.created_at)}
          </time>
        </span>
      )),
    },
    {
      id: "actions",
      header: "Actions",
      width: 250,
      action: true,
      cell: dataCell<Row, "version">("version", "h-7 w-32", ({ meta }) => {
        const latest = meta.version === latestVersion
        return (
          <div className="flex flex-wrap items-center gap-1.5 sm:flex-nowrap">
            <Link
              {...dashboardLink(dashboardId)}
              search={
                latest
                  ? dashboardSearchFromBack(back, { dropVersion: true })
                  : {
                      ...dashboardSearchFromBack(back, { dropVersion: true }),
                      version: meta.version,
                    }
              }
              aria-label={`View version ${meta.version}`}
              className={cn(
                buttonVariants({ variant: "ghost", size: "sm" }),
                "-ml-[11px]"
              )}
            >
              View
            </Link>
            <Button
              variant="outline"
              size="sm"
              aria-label={`Compare version ${meta.version} with ${
                meta.version === 1
                  ? "an empty dashboard"
                  : `version ${meta.version - 1}`
              }`}
              onClick={() => onCompare(meta.version)}
            >
              Compare
            </Button>
            {latest ? null : (
              <RestoreVersionButton
                dashboardId={dashboardId}
                version={meta.version}
                latestVersion={latestVersion}
                onRestored={onRestored}
              />
            )}
          </div>
        )
      }),
    },
  ]
}

const rowId = (row: Row) => row.id
const skeleton = skeletonRows(5)

export function VersionList({
  dashboardId,
  latestVersion,
  back,
  onCompare,
  onRestored,
}: Props) {
  const query = useVersionHistory(dashboardId)
  const rows = useMemo<Row[]>(
    () =>
      query.data?.pages
        .flatMap((page) => page.versions)
        .map((meta) => ({
          kind: "version",
          id: String(meta.version),
          meta,
        })) ?? [],
    [query.data]
  )
  const columns = useMemo(
    () =>
      versionColumns({
        dashboardId,
        latestVersion,
        back,
        onCompare,
        onRestored,
      }),
    [dashboardId, latestVersion, back, onCompare, onRestored]
  )
  const loadMoreFailed = query.isFetchNextPageError

  return (
    <section aria-label="Versions">
      {query.isError && !loadMoreFailed ? (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Could not load versions</AlertTitle>
          <AlertDescription>
            <p>{query.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className="text-foreground"
              disabled={query.isFetching}
              focusableWhenDisabled
              aria-busy={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {query.isError && !query.data ? null : (
        <DataGrid
          label="Versions"
          busy={query.isPending}
          rows={query.isPending ? skeleton : rows}
          columns={columns}
          rowId={rowId}
        />
      )}
      {query.hasNextPage ? (
        <div className="mt-4 flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
          {loadMoreFailed ? (
            <p role="alert" className="text-sm text-destructive">
              Could not load more versions: {query.error?.message}
            </p>
          ) : null}
          <Button
            variant="outline"
            className="text-foreground"
            disabled={query.isFetchingNextPage}
            focusableWhenDisabled
            aria-busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </section>
  )
}
