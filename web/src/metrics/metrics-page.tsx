import { useCallback, useEffect, useRef, useState } from "react"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"

import { loadProblemText } from "@/api/errors"
import { useInputMode } from "@/ask/input-mode"
import { LoadProblem } from "@/components/load-problem"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { SourceTitle } from "@/datasources/source-title"
import { UnknownSource } from "@/datasources/unknown-source"
import { useExploreSources } from "@/datasources/use-sources"
import { exploreKeys } from "@/api/keys"
import { useDocumentTitle } from "@/lib/document-title"
import { useAgentInfo } from "@/observe/queries"
import { useLogsShortcuts } from "@/logs/use-logs-shortcuts"
import { roleAtLeast } from "@/lib/roles"
import { metricsDraft } from "@/saved-searches/draft"
import { OpenSavedSearch } from "@/saved-searches/open-saved-search"
import { ExploreActions } from "@/saved-searches/explore-actions"
import { useLeaveAskOnSavedOpen } from "@/saved-searches/saved-open"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { exploreRange } from "@/lib/time"

import { QueryBar } from "./query-bar"
import { ResultsSection } from "./results-section"
import { MetricsResultActions } from "./result-actions"
import { modeOf, searchAfter, type MetricsSearch } from "./search"
import { MetricsWorkspaceContext, type MetricsWorkspace } from "./workspace"

const routeId = "/_authed/_shell/explore/metrics/$datasourceUid"

type Update = MetricsWorkspace["update"]

function useMetricsSearch() {
  const search = useSearch({ from: routeId })
  const navigate = useNavigate({ from: "/explore/metrics/$datasourceUid" })
  const update = useCallback<Update>(
    (next, options) =>
      void navigate({
        search: (state) => searchAfter(state, next),
        ...(options?.replace === undefined ? {} : { replace: options.replace }),
      }),
    [navigate]
  )
  return { search, update }
}

export function MetricsPage({ uid }: { uid: string }) {
  const { search, update } = useMetricsSearch()
  const { sources, isPending, isError, error, refetch } =
    useExploreSources("metrics")
  const { principal } = useRouteContext({ from: "/_authed" })
  const datasource = sources.find((candidate) => candidate.uid === uid)
  const rememberedUid = datasource?.uid
  useDocumentTitle(
    datasource
      ? `Metrics / ${datasource.name} - SlashObserve`
      : "Metrics - SlashObserve"
  )
  useEffect(() => {
    if (rememberedUid !== undefined) {
      useSourceMemory.getState().remember("metrics", rememberedUid)
    }
  }, [rememberedUid])

  if (isPending) {
    return (
      <>
        <h1 className="sr-only">Metrics</h1>
        <p role="status" className="text-[13px] text-muted-foreground">
          Loading the datasources.
        </p>
      </>
    )
  }
  if (isError) {
    return (
      <>
        <h1 className="sr-only">Metrics</h1>
        <LoadProblem
          title="Could not load the datasources"
          detail={loadProblemText(error)}
          retrying={false}
          onRetry={() => void refetch()}
        />
      </>
    )
  }
  if (sources.length === 0) {
    return (
      <>
        <h1 className="sr-only">Metrics</h1>
        <p className="text-[13px] text-muted-foreground">
          No metrics datasource is configured. An Admin adds a Prometheus
          datasource in the datasources file.
        </p>
        {roleAtLeast(principal.role, "admin") ? (
          <Link
            to="/datasources"
            className="mt-2 inline-block text-[13px] underline underline-offset-3"
          >
            Open datasources
          </Link>
        ) : null}
      </>
    )
  }
  if (datasource === undefined) {
    return <UnknownSource kind="metrics" uid={uid} />
  }
  return (
    <MetricsWorkspaceView
      key={datasource.uid}
      datasource={datasource}
      search={search}
      update={update}
    />
  )
}

function MetricsWorkspaceView({
  datasource,
  search,
  update,
}: {
  datasource: DatasourceStatus
  search: MetricsSearch
  update: Update
}) {
  const queryClient = useQueryClient()
  const uid = datasource.uid
  const range = exploreRange(search)
  const mode = modeOf(search)
  const q = search.q
  const [draft, setDraft] = useState(q ?? "")
  const [seenQuery, setSeenQuery] = useState(q)
  if (seenQuery !== q) {
    setSeenQuery(q)
    if (q !== undefined) setDraft(q)
  }
  const agentAvailable = useAgentInfo().data?.available === true
  const [inputMode, setInputMode] = useInputMode({
    agentAvailable,
    hasQuery: q !== undefined,
    hasAsk: search.ask !== undefined,
  })
  useLeaveAskOnSavedOpen(inputMode, setInputMode)
  const saveTrigger = useRef<HTMLButtonElement>(null)
  const [saveOpen, setSaveOpen] = useState(false)
  const savable = metricsDraft({ uid, search, range })
  const queryFocus = useRef<() => void>(() => {})
  const registerQueryFocus = useCallback((focus: () => void) => {
    queryFocus.current = focus
  }, [])
  const focusQuery = useCallback(() => queryFocus.current(), [])
  useLogsShortcuts({ focusQuery, openHelp: () => {} })

  function run(raw?: string, asked?: string) {
    const text = (raw ?? draft).trim()
    if (text === "") return
    if (text === q) {
      void queryClient.invalidateQueries({
        queryKey: exploreKeys.all,
        predicate: (query) => query.queryKey[1] === uid,
      })
    }
    if (text !== q || (asked !== undefined && asked !== search.ask)) {
      update(asked === undefined ? { q: text } : { q: text, ask: asked })
    }
  }

  const workspace: MetricsWorkspace = {
    datasource,
    search,
    mode,
    inputMode,
    setInputMode,
    effectiveQuery: q,
    update,
    draft,
    setDraft,
    run,
    range,
    focusQuery,
    registerQueryFocus,
  }

  return (
    <MetricsWorkspaceContext value={workspace}>
      <ExploreActions
        kind="metrics"
        range={range}
        onRangeChange={update}
        saveBlocked={"draft" in savable ? undefined : "Run a query first."}
        saveTrigger={saveTrigger}
        onSave={() => setSaveOpen(true)}
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <h1 className="sr-only">Metrics in {datasource.name}</h1>
        <SourceTitle kind="metrics" source={datasource} />
        <OpenSavedSearch
          kind="metrics"
          uid={uid}
          savedId={search.saved}
          draft={"draft" in savable ? savable.draft : undefined}
          onClose={() => update({ saved: undefined }, { replace: true })}
        />
        <section
          aria-label="Query"
          className="flex flex-col rounded-card border bg-card"
        >
          <QueryBar />
        </section>
        <div className="flex min-h-[34rem] min-w-0 flex-1 overflow-hidden rounded-card border bg-card narrow:min-h-[48rem]">
          <ResultsSection
            uid={uid}
            query={q}
            range={range}
            mode={mode}
            onMode={(next) =>
              update({ mode: next === "table" ? "table" : undefined })
            }
            onSelectRange={(fromMs, toMs) =>
              update({
                from: String(Math.round(fromMs)),
                to: String(Math.round(toMs)),
              })
            }
            actions={
              <MetricsResultActions
                uid={uid}
                query={q}
                mode={mode}
                range={range}
                observation={search.observation}
              />
            }
          />
        </div>
      </div>
      {"draft" in savable ? (
        <SaveSearchDialog
          draft={savable.draft}
          open={saveOpen}
          onOpenChange={setSaveOpen}
          finalFocus={() => saveTrigger.current ?? true}
        />
      ) : null}
    </MetricsWorkspaceContext>
  )
}
