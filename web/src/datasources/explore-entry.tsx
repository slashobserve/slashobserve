import { Link, Navigate, useRouteContext } from "@tanstack/react-router"

import { LoadProblem } from "@/components/load-problem"
import { useDocumentTitle } from "@/lib/document-title"
import { logsLink, tracesLink } from "@/lib/links"
import { roleAtLeast } from "@/lib/roles"
import { ResultSkeleton } from "@/logs/results-section"
import { loadProblemText } from "@/service-accounts/queries"

import { recentSources } from "./datasource-memory"
import { defaultSource, type ExploreKind } from "./sources"
import { useExploreSources } from "./use-sources"

const pageNames = { logs: "Logs", traces: "Traces" } as const

const emptyText = {
  logs: "No logs datasource is configured. An Admin adds an Elasticsearch, OpenSearch, Loki or ClickHouse datasource in the datasources file.",
  traces:
    "No traces datasource is configured. An Admin adds a Tempo or ClickHouse datasource in the datasources file.",
} as const

export function ExploreEntry({ kind }: { kind: ExploreKind }) {
  const name = pageNames[kind]
  useDocumentTitle(`${name} - SlashObserve`)
  const { principal } = useRouteContext({ from: "/_authed" })
  const { sources, isPending, isError, error, refetch } =
    useExploreSources(kind)
  const target = defaultSource(kind, sources, recentSources(kind))

  if (target !== undefined) {
    return (
      <Navigate
        {...(kind === "logs" ? logsLink(target.uid) : tracesLink(target.uid))}
        replace
      />
    )
  }
  return (
    <>
      <h1 className="sr-only">{name}</h1>
      {isPending ? (
        <ResultSkeleton />
      ) : isError ? (
        <LoadProblem
          title="Could not load the datasources"
          detail={loadProblemText(error)}
          retrying={false}
          onRetry={() => void refetch()}
        />
      ) : (
        <>
          <p className="text-[13px] text-muted-foreground">{emptyText[kind]}</p>
          {roleAtLeast(principal.role, "admin") ? (
            <Link
              to="/datasources"
              className="mt-2 inline-block text-[13px] underline underline-offset-3"
            >
              Open datasources
            </Link>
          ) : null}
        </>
      )}
    </>
  )
}
