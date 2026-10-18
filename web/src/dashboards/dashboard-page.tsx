import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"

import type { Selections } from "@/api/keys"
import { Skeleton } from "@/components/ui/skeleton"
import { refreshToMs } from "@/lib/time"
import { panelRenderers } from "@/panels"
import { hasData } from "@/panels/frames"
import type { Panel } from "@/spec"

import { DashboardGrid, gridMargin, gridRowHeight } from "./dashboard-grid"
import { DashboardToolbar } from "./dashboard-toolbar"
import { LiveIndicator } from "./live/live-indicator"
import {
  useDashboardEvents,
  type LiveStatus,
} from "./live/use-dashboard-events"
import { panelBodySize, PanelChrome, type PanelState } from "./panel-chrome"
import { PanelMenu } from "./panel-menu"
import {
  asDashboardSpec,
  dashboardVersionQuery,
  latestDashboardQuery,
  type DashboardEnvelope,
} from "./queries"
import { RefreshPicker } from "./refresh-picker"
import { useDashboardSearch, useSetDashboardSearch } from "./search"
import { TimeRangePicker } from "./time-range-picker"
import { maxDataPointsFor, usePanelData } from "./use-panel-data"
import { useSeen } from "./use-seen"
import { useVariables } from "./use-variables"
import { VariablesBar } from "./variables-bar"
import { VersionBanner } from "./version-banner"
import { VariableLoadError } from "./variables"

type PanelContainerProps = {
  dashboardId: string
  version: number
  panel: Panel
  width: number
  height: number
  from: string | undefined
  to: string | undefined
  selections: Selections
  selectionsReady: boolean
  blockedBy: string | undefined
  onRetryBlocked: () => void
  refreshMs: number | null
}

const PanelContainer = memo(function PanelContainer({
  dashboardId,
  version,
  panel,
  width,
  height,
  from,
  to,
  selections,
  selectionsReady,
  blockedBy,
  onRetryBlocked,
  refreshMs,
}: PanelContainerProps) {
  const ref = useRef<HTMLDivElement>(null)
  const seen = useSeen(ref)
  const [tableView, setTableView] = useState(false)
  const setSearch = useSetDashboardSearch()
  const body = panelBodySize(Math.floor(width), Math.floor(height))

  const query = usePanelData({
    dashboardId,
    version,
    panel,
    from,
    to,
    vars: selections,
    maxDataPoints: maxDataPointsFor(panel, body.width),
    refreshMs,
    enabled: seen && selectionsReady,
  })

  const onTimeRangeSelect = useCallback(
    (fromMs: number, toMs: number) =>
      setSearch((state) => ({
        ...state,
        from: String(Math.round(fromMs)),
        to: String(Math.round(toMs)),
      })),
    [setSearch]
  )

  let state: PanelState = "loading"
  if (query.data) state = hasData(query.data.frames) ? "ready" : "empty"
  else if (blockedBy !== undefined) state = "error"
  else if (query.isError) state = "error"

  const blockedError = useMemo(
    () =>
      blockedBy === undefined ? undefined : new VariableLoadError(blockedBy),
    [blockedBy]
  )

  const Renderer = panelRenderers[tableView ? "table" : panel.type]

  return (
    <div ref={ref} className="size-full">
      <PanelChrome
        panel={panel}
        state={state}
        error={query.data ? query.error : (blockedError ?? query.error)}
        refreshing={query.isFetching && query.data !== undefined}
        staleError={query.isError && query.data ? query.error : undefined}
        onRetry={() => (blockedError ? onRetryBlocked() : void query.refetch())}
        menu={
          <PanelMenu
            panel={panel}
            tableView={tableView}
            onToggleTableView={() => setTableView((current) => !current)}
          />
        }
      >
        {query.data ? (
          <Renderer
            panel={panel}
            frames={query.data.frames}
            meta={query.data.meta}
            width={body.width}
            height={body.height}
            syncKey={dashboardId}
            onTimeRangeSelect={onTimeRangeSelect}
          />
        ) : null}
      </PanelChrome>
    </div>
  )
})

function DashboardView({
  dashboardId,
  envelope,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  envelope: DashboardEnvelope
  latestVersion: number
  liveStatus: LiveStatus
}) {
  const search = useDashboardSearch()
  const version = envelope.version.version
  const spec = useMemo(
    () => asDashboardSpec(envelope.version.spec),
    [envelope.version.spec]
  )
  const variables = useMemo(() => spec.variables ?? [], [spec])

  const { states, selections, ready, blocking } = useVariables({
    dashboardId,
    version,
    variables,
    from: search.from,
    to: search.to,
    urlVars: search.vars,
  })

  const retryBlocking = useRef<(() => void) | undefined>(undefined)
  useEffect(() => {
    retryBlocking.current = blocking?.retry
  })
  const onRetryBlocked = useCallback(() => retryBlocking.current?.(), [])
  const blockedBy = blocking
    ? (blocking.variable.label ?? blocking.variable.name)
    : undefined

  useEffect(() => {
    const previous = document.title
    document.title = `${spec.title} - SlashObserve`
    return () => {
      document.title = previous
    }
  }, [spec.title])

  const effectiveFrom = search.from ?? spec.timeRange.from
  const effectiveTo = search.to ?? spec.timeRange.to
  const refreshValue = search.refresh ?? spec.refresh ?? "off"
  const refreshMs = refreshToMs(refreshValue)

  const controls = [
    <TimeRangePicker
      key="time-range"
      dashboardId={dashboardId}
      version={version}
      from={effectiveFrom}
      to={effectiveTo}
    />,
    <RefreshPicker key="refresh" value={refreshValue} />,
  ]
  const actions: ReactNode[] = [
    <LiveIndicator
      key="live"
      status={liveStatus}
      pinned={search.version !== undefined}
    />,
  ]

  return (
    <main className="flex flex-1 flex-col">
      <DashboardToolbar
        title={spec.title}
        versionBadge={`v${version}`}
        controls={controls}
        actions={actions}
      />
      {search.version !== undefined ? (
        <VersionBanner version={version} latestVersion={latestVersion} />
      ) : null}
      <VariablesBar states={states} />
      <div className="flex-1 p-4">
        <DashboardGrid
          spec={spec}
          renderPanel={(panel, size) => (
            <PanelContainer
              dashboardId={dashboardId}
              version={version}
              panel={panel}
              width={size.width}
              height={size.height}
              from={search.from}
              to={search.to}
              selections={selections}
              selectionsReady={ready}
              blockedBy={blockedBy}
              onRetryBlocked={onRetryBlocked}
              refreshMs={refreshMs}
            />
          )}
        />
      </div>
    </main>
  )
}

function PinnedDashboardPage({
  dashboardId,
  version,
  latestVersion,
  liveStatus,
}: {
  dashboardId: string
  version: number
  latestVersion: number | undefined
  liveStatus: LiveStatus
}) {
  const pinned = useQuery(dashboardVersionQuery(dashboardId, version))
  if (!pinned.data) return <DashboardPageSkeleton />
  return (
    <DashboardView
      dashboardId={dashboardId}
      envelope={pinned.data}
      latestVersion={latestVersion ?? pinned.data.version.version}
      liveStatus={liveStatus}
    />
  )
}

export function DashboardPage({ dashboardId }: { dashboardId: string }) {
  const search = useDashboardSearch()
  const queryClient = useQueryClient()
  const latest = useQuery(latestDashboardQuery(queryClient, dashboardId))
  const latestVersion = latest.data?.dashboard.latest_version
  const { status: liveStatus } = useDashboardEvents(dashboardId, {
    pinnedVersion: search.version,
  })
  if (search.version !== undefined) {
    return (
      <PinnedDashboardPage
        dashboardId={dashboardId}
        version={search.version}
        latestVersion={latestVersion}
        liveStatus={liveStatus}
      />
    )
  }
  if (!latest.data) return <DashboardPageSkeleton />
  return (
    <DashboardView
      dashboardId={dashboardId}
      envelope={latest.data}
      latestVersion={latestVersion ?? latest.data.version.version}
      liveStatus={liveStatus}
    />
  )
}

const skeletonPanels = [
  "min-[800px]:col-span-12 row-span-8",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-6 row-span-4",
  "min-[800px]:col-span-24 row-span-8",
]

export function DashboardPageSkeleton() {
  return (
    <main className="flex flex-1 flex-col" aria-busy="true">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b px-4 py-2">
        <Skeleton className="h-7 w-56 animate-none motion-safe:animate-pulse" />
        <Skeleton className="h-8 w-48 animate-none motion-safe:animate-pulse" />
      </div>
      <div className="flex-1 p-4">
        <div
          className="grid grid-cols-1 min-[800px]:grid-cols-24"
          style={{ gridAutoRows: gridRowHeight, gap: gridMargin }}
        >
          {skeletonPanels.map((span, index) => (
            <Skeleton
              key={index}
              className={`${span} animate-none motion-safe:animate-pulse`}
            />
          ))}
        </div>
      </div>
    </main>
  )
}
