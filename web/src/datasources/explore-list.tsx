import { useId, useState } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"

import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { SavedSearchTable } from "@/saved-searches/saved-search-table"
import { loadProblemText } from "@/service-accounts/queries"

import { statusOf } from "./format"
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

const emptyTitles = {
  logs: "No log sources",
  traces: "No trace sources",
  metrics: "No metric sources",
} as const

const emptyText = {
  logs: "No logs datasource is configured. An Admin adds an Elasticsearch, OpenSearch, Loki or ClickHouse datasource on the Datasources page.",
  traces:
    "No traces datasource is configured. An Admin adds a Tempo or ClickHouse datasource on the Datasources page.",
  metrics:
    "No metrics datasource is configured. An Admin adds a Prometheus datasource on the Datasources page.",
} as const

const tableWidth = 480
const servicesTableWidth = 720
const skeletonRows = 2

function SourceColumns({ withServices }: { withServices: boolean }) {
  return (
    <thead>
      <tr>
        <HeaderCell>Name</HeaderCell>
        <HeaderCell>Language</HeaderCell>
        <HeaderCell>Status</HeaderCell>
        {withServices ? <HeaderCell>Services</HeaderCell> : null}
      </tr>
    </thead>
  )
}

function SourceSkeleton({ headingId }: { headingId: string }) {
  return (
    <TableCard labelledBy={headingId} minWidth={tableWidth} stackOnNarrow>
      <SourceColumns withServices={false} />
      <tbody aria-busy="true">
        {Array.from({ length: skeletonRows }, (_, index) => (
          <tr key={index}>
            <Cell>
              <Skeleton className="h-4 w-40 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-16 motion-safe:animate-none" />
            </Cell>
            <Cell>
              <Skeleton className="h-4 w-20 motion-safe:animate-none" />
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
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
          <SourceSkeleton headingId={sourcesId} />
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
          <TableCard
            labelledBy={sourcesId}
            minWidth={signal === null ? tableWidth : servicesTableWidth}
            stackOnNarrow
          >
            <SourceColumns withServices={signal !== null} />
            <tbody>
              {shown.length === 0 ? (
                <tr>
                  <Cell colSpan={4} className="text-muted-foreground">
                    No source or service matches the filter.
                  </Cell>
                </tr>
              ) : null}
              {shown.map((source) => {
                const { tone, text } = statusOf(source)
                return (
                  <tr key={source.uid}>
                    <Cell className="wrap-anywhere">
                      <Link
                        {...exploreLink(kind, source.uid, {})}
                        className="rounded-sm font-medium underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        {source.name}
                      </Link>
                    </Cell>
                    <Cell className="text-muted-foreground">
                      {sourceLanguage(kind, source.type)}
                    </Cell>
                    <Cell>
                      <div className="flex">
                        <StatusLabel tone={tone} title={text}>
                          <span className="max-w-[360px] truncate">{text}</span>
                        </StatusLabel>
                      </div>
                    </Cell>
                    {signal !== null ? (
                      <Cell data-label="Services" className="max-w-[420px]">
                        <SourceServices
                          kind={signal}
                          uid={source.uid}
                          state={services.get(source.uid)}
                        />
                      </Cell>
                    ) : null}
                  </tr>
                )
              })}
            </tbody>
          </TableCard>
        )}
      </section>
      <SavedSearchTable kind={kind} sources={sources} />
    </>
  )
}
