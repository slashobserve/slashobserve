import { useId } from "react"
import { Link, useRouteContext } from "@tanstack/react-router"

import { EmptyState } from "@/components/empty-state"
import { LoadProblem } from "@/components/load-problem"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { logsLink, tracesLink } from "@/lib/links"
import { roleAtLeast } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { SavedSearchTable } from "@/saved-searches/saved-search-table"
import { loadProblemText } from "@/service-accounts/queries"

import { statusOf } from "./format"
import { sourceLanguage, type ExploreKind } from "./sources"
import { useExploreSources } from "./use-sources"

const pageNames = { logs: "Logs", traces: "Traces" } as const

const emptyTitles = {
  logs: "No log sources",
  traces: "No trace sources",
} as const

const emptyText = {
  logs: "No logs datasource is configured. An Admin adds an Elasticsearch, OpenSearch, Loki or ClickHouse datasource in the datasources file.",
  traces:
    "No traces datasource is configured. An Admin adds a Tempo or ClickHouse datasource in the datasources file.",
} as const

const tableWidth = 480
const skeletonRows = 2

function SourceColumns() {
  return (
    <thead>
      <tr>
        <HeaderCell>Name</HeaderCell>
        <HeaderCell>Language</HeaderCell>
        <HeaderCell>Status</HeaderCell>
      </tr>
    </thead>
  )
}

function SourceSkeleton({ headingId }: { headingId: string }) {
  return (
    <TableCard labelledBy={headingId} minWidth={tableWidth} stackOnNarrow>
      <SourceColumns />
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
  const name = pageNames[kind]
  useDocumentTitle(`${name} - SlashObserve`)
  const sourcesId = useId()
  const { principal } = useRouteContext({ from: "/_authed" })
  const { sources, isPending, isError, error, refetch } =
    useExploreSources(kind)

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
        <h2 id={sourcesId} className="mb-2 text-sm font-semibold">
          Sources
        </h2>
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
          <TableCard labelledBy={sourcesId} minWidth={tableWidth} stackOnNarrow>
            <SourceColumns />
            <tbody>
              {sources.map((source) => {
                const { tone, text } = statusOf(source)
                return (
                  <tr key={source.uid}>
                    <Cell className="wrap-anywhere">
                      <Link
                        {...(kind === "logs"
                          ? logsLink(source.uid)
                          : tracesLink(source.uid))}
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
