import { useId, useState } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"

import { loadProblemText } from "@/api/errors"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { SavedSearchTable } from "@/saved-searches/saved-search-table"

import { statusOf } from "./format"
import type { DatasourceStatus } from "./queries"
import { useSourceServices } from "./service-queries"
import { filterSources, serviceSignal } from "./services"
import { SourceServices } from "./source-services"
import {
  exploreLink,
  exploreText,
  sourceLanguage,
  type ExploreKind,
} from "./sources"
import { useBrowsableSources } from "./use-sources"

function sourceId(source: DatasourceStatus): string {
  return source.uid
}

const emptyTitles = {
  logs: "No log sources",
  traces: "No trace sources",
  metrics: "No metric sources",
} as const

const emptyText = {
  logs: "No logs datasource is configured. An Admin adds an Elasticsearch, OpenSearch, Loki or ClickHouse datasource on the Datasources page.",
  traces:
    "No traces datasource is configured. An Admin adds a Tempo, Jaeger or ClickHouse datasource on the Datasources page.",
  metrics:
    "No metrics datasource is configured. An Admin adds a Prometheus datasource on the Datasources page.",
} as const

const skeletonRows = 2

function SourceSkeleton() {
  return (
    <div className="flex flex-col gap-3" aria-busy="true">
      {Array.from({ length: skeletonRows }, (_, index) => (
        <Skeleton key={index} className="h-4 w-full motion-safe:animate-none" />
      ))}
    </div>
  )
}

function sourceColumns(
  kind: ExploreKind,
  signal: ReturnType<typeof serviceSignal>,
  services: ReturnType<typeof useSourceServices>
): DataGridColumn<DatasourceStatus>[] {
  return [
    {
      id: "name",
      header: "Name",
      minWidth: 140,
      autoHeight: true,
      cell: (source) => (
        <Link
          {...exploreLink(kind, source.uid, {})}
          className="rounded-sm py-2 font-medium wrap-anywhere underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {source.name}
        </Link>
      ),
    },
    {
      id: "language",
      header: "Language",
      minWidth: 100,
      cell: (source) => (
        <span className="text-muted-foreground">
          {sourceLanguage(kind, source.type)}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      minWidth: 140,
      cell: (source) => {
        const { tone, text } = statusOf(source)
        return (
          <StatusLabel tone={tone} title={text}>
            <span className="max-w-[360px] truncate">{text}</span>
          </StatusLabel>
        )
      },
    },
    ...(signal === null
      ? []
      : [
          {
            id: "services",
            header: "Services",
            minWidth: 240,
            flex: 2,
            autoHeight: true,
            cell: (source: DatasourceStatus) => (
              <div className="max-w-[420px] py-2">
                <SourceServices
                  kind={signal}
                  uid={source.uid}
                  state={services.get(source.uid)}
                />
              </div>
            ),
          },
        ]),
  ]
}

export function ExploreList({ kind }: { kind: ExploreKind }) {
  const name = exploreText[kind].page
  useDocumentTitle(`${name} - SlashObserve`)
  const sourcesId = useId()
  const { principal } = useRouteContext({ from: "/_authed" })
  const { sources, isPending, isError, error, refetch } =
    useBrowsableSources(kind)
  const signal = serviceSignal(kind)
  const services = useSourceServices(sources, signal)
  const [filter, setFilter] = useState("")
  const shown = filterSources(sources, services, filter)

  const adminLink = roleAtLeast(principal.role, "admin") ? (
    <Link
      to="/datasources"
      className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
    >
      Open datasources
    </Link>
  ) : undefined

  return (
    <>
      <PageHeader title={name} />
      <section aria-labelledby={sourcesId}>
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 id={sourcesId} className="text-sm font-semibold">
            Sources
          </h2>
          {signal !== null && sources.length > 0 ? (
            <div className="relative max-w-xs min-w-[200px] flex-1">
              <SearchIcon
                aria-hidden="true"
                className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                type="search"
                aria-label="Filter sources and services"
                placeholder="Filter sources and services"
                autoComplete="off"
                maxLength={200}
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                className="pl-8"
              />
            </div>
          ) : null}
        </div>
        {isPending ? (
          <SourceSkeleton />
        ) : isError ? (
          <LoadProblem
            title="Could not load the datasources"
            detail={loadProblemText(error)}
            retrying={false}
            onRetry={() => void refetch()}
          />
        ) : sources.length === 0 ? (
          <div className="rounded-card border bg-background">
            <EmptyState title={emptyTitles[kind]} action={adminLink}>
              {emptyText[kind]}
            </EmptyState>
          </div>
        ) : (
          <DataGrid
            label="Sources"
            rows={shown}
            columns={sourceColumns(kind, signal, services)}
            rowId={sourceId}
            empty={
              <p className="rounded-card border px-3.5 py-3 text-[13px] text-muted-foreground">
                No source or service matches the filter.
              </p>
            }
          />
        )}
      </section>
      <SavedSearchTable kind={kind} sources={sources} />
    </>
  )
}
